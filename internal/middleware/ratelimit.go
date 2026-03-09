// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/kolapsis/shm/internal/config"
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // Unix nano timestamp for thread-safe access
}

type bruteForceEntry struct {
	failures  int
	bannedAt  time.Time
	banExpiry time.Time
}

type RateLimiter struct {
	config config.RateLimitConfig

	ipLimiters       sync.Map // IP -> limiterEntry (for register/activate)
	instanceLimiters sync.Map // Instance ID -> limiterEntry (for snapshot)
	adminLimiters    sync.Map // IP -> limiterEntry (for admin)
	bruteForce       sync.Map // IP -> bruteForceEntry

	stopCleanup chan struct{}
}

func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:      cfg,
		stopCleanup: make(chan struct{}),
	}

	if cfg.Enabled && cfg.CleanupInterval > 0 {
		go rl.cleanupLoop()
	}

	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

func (rl *RateLimiter) cleanup() {
	threshold := time.Now().Add(-rl.config.CleanupInterval * 2).UnixNano()

	cleanupMap := func(m *sync.Map) int {
		count := 0
		m.Range(func(key, value interface{}) bool {
			if entry, ok := value.(*limiterEntry); ok {
				if entry.lastSeen.Load() < threshold {
					m.Delete(key)
					count++
				}
			}
			return true
		})
		return count
	}

	ipCount := cleanupMap(&rl.ipLimiters)
	instanceCount := cleanupMap(&rl.instanceLimiters)
	adminCount := cleanupMap(&rl.adminLimiters)

	bruteForceCount := 0
	now := time.Now()
	rl.bruteForce.Range(func(key, value interface{}) bool {
		if entry, ok := value.(*bruteForceEntry); ok {
			if !entry.banExpiry.IsZero() && entry.banExpiry.Before(now) {
				rl.bruteForce.Delete(key)
				bruteForceCount++
			}
		}
		return true
	})

	total := ipCount + instanceCount + adminCount + bruteForceCount
	if total > 0 {
		slog.Debug("ratelimit cleanup",
			"total", total,
			"ip", ipCount,
			"instance", instanceCount,
			"admin", adminCount,
			"bruteforce", bruteForceCount,
		)
	}
}

func (rl *RateLimiter) getLimiter(store *sync.Map, key string, cfg config.RateLimitRouteConfig) *rate.Limiter {
	nowNano := time.Now().UnixNano()
	rateLimit := rate.Limit(float64(cfg.Requests) / cfg.Period.Seconds())

	if existing, ok := store.Load(key); ok {
		entry := existing.(*limiterEntry)
		entry.lastSeen.Store(nowNano)
		return entry.limiter
	}

	limiter := rate.NewLimiter(rateLimit, cfg.Burst)
	entry := &limiterEntry{
		limiter: limiter,
	}
	entry.lastSeen.Store(nowNano)

	actual, _ := store.LoadOrStore(key, entry)
	return actual.(*limiterEntry).limiter
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = xff[:idx]
		}
		xff = strings.TrimSpace(xff)
		if xff != "" {
			return xff
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func writeRateLimitHeaders(w http.ResponseWriter, limiter *rate.Limiter, cfg config.RateLimitRouteConfig) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(cfg.Requests))

	tokens := int(limiter.Tokens())
	if tokens < 0 {
		tokens = 0
	}
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(tokens))

	resetTime := time.Now().Add(cfg.Period).Unix()
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
}

func writeTooManyRequests(w http.ResponseWriter, limiter *rate.Limiter, cfg config.RateLimitRouteConfig) {
	writeRateLimitHeaders(w, limiter, cfg)

	reservation := limiter.Reserve()
	delay := reservation.Delay()
	reservation.Cancel()

	retryAfter := int(delay.Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))

	http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
}

func (rl *RateLimiter) RegisterMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.config.Enabled {
			next(w, r)
			return
		}

		ip := getClientIP(r)
		limiter := rl.getLimiter(&rl.ipLimiters, ip, rl.config.Register)

		if !limiter.Allow() {
			slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			writeTooManyRequests(w, limiter, rl.config.Register)
			return
		}

		writeRateLimitHeaders(w, limiter, rl.config.Register)
		next(w, r)
	}
}

func (rl *RateLimiter) SnapshotMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.config.Enabled {
			next(w, r)
			return
		}

		instanceID := r.Header.Get("X-Instance-ID")
		if instanceID == "" {
			next(w, r)
			return
		}

		limiter := rl.getLimiter(&rl.instanceLimiters, instanceID, rl.config.Snapshot)

		if !limiter.Allow() {
			slog.Warn("rate limit exceeded", "instance_id", instanceID, "path", "/v1/snapshot")
			writeTooManyRequests(w, limiter, rl.config.Snapshot)
			return
		}

		writeRateLimitHeaders(w, limiter, rl.config.Snapshot)
		next(w, r)
	}
}

func (rl *RateLimiter) AdminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.config.Enabled {
			next(w, r)
			return
		}

		ip := getClientIP(r)

		if rl.isBanned(ip) {
			slog.Warn("banned IP attempted access", "ip", ip)
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.config.BruteForceBan.Seconds())))
			http.Error(w, "Too Many Requests - Temporarily Banned", http.StatusTooManyRequests)
			return
		}

		limiter := rl.getLimiter(&rl.adminLimiters, ip, rl.config.Admin)

		if !limiter.Allow() {
			slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			writeTooManyRequests(w, limiter, rl.config.Admin)
			return
		}

		writeRateLimitHeaders(w, limiter, rl.config.Admin)

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next(wrapped, r)

		if wrapped.statusCode == http.StatusUnauthorized || wrapped.statusCode == http.StatusForbidden {
			rl.recordAuthFailure(ip)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rl *RateLimiter) isBanned(ip string) bool {
	if entry, ok := rl.bruteForce.Load(ip); ok {
		bf := entry.(*bruteForceEntry)
		if !bf.banExpiry.IsZero() && time.Now().Before(bf.banExpiry) {
			return true
		}
	}
	return false
}

func (rl *RateLimiter) recordAuthFailure(ip string) {
	now := time.Now()

	entry, _ := rl.bruteForce.LoadOrStore(ip, &bruteForceEntry{})
	bf := entry.(*bruteForceEntry)

	bf.failures++
	slog.Debug("auth failure recorded", "failures", bf.failures, "threshold", rl.config.BruteForceThreshold, "ip", ip)

	if bf.failures >= rl.config.BruteForceThreshold {
		bf.bannedAt = now
		bf.banExpiry = now.Add(rl.config.BruteForceBan)
		slog.Warn("IP banned for brute-force", "ip", ip, "duration", rl.config.BruteForceBan)
	}
}
