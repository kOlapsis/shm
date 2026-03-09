// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/shm/internal/config"
)

// okHandler is a simple handler that returns 200 OK
func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expectedIP string
	}{
		{
			name:       "no proxy headers",
			remoteAddr: "192.168.1.1:12345",
			headers:    map[string]string{},
			expectedIP: "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			expectedIP: "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, 70.41.3.18, 150.172.238.178"},
			expectedIP: "203.0.113.50",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "198.51.100.178"},
			expectedIP: "198.51.100.178",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.50",
				"X-Real-IP":       "198.51.100.178",
			},
			expectedIP: "203.0.113.50",
		},
		{
			name:       "empty X-Forwarded-For falls back to X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "",
				"X-Real-IP":       "198.51.100.178",
			},
			expectedIP: "198.51.100.178",
		},
		{
			name:       "remoteAddr without port",
			remoteAddr: "192.168.1.1",
			headers:    map[string]string{},
			expectedIP: "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			ip := getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("getClientIP() = %q, want %q", ip, tt.expectedIP)
			}
		})
	}
}

func TestRegisterMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		numRequests    int
		expectedStatus int
	}{
		{
			name:           "within rate limit",
			numRequests:    1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "at burst limit",
			numRequests:    2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "exceeds rate limit",
			numRequests:    10,
			expectedStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.RateLimitConfig{
				Enabled:         true,
				CleanupInterval: 0,
				Register: config.RateLimitRouteConfig{
					Requests: 5,
					Period:   time.Minute,
					Burst:    2,
				},
			}
			rl := NewRateLimiter(cfg)
			defer rl.Stop()

			handler := rl.RegisterMiddleware(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			var lastStatus int
			for i := 0; i < tt.numRequests; i++ {
				req := httptest.NewRequest("POST", "/v1/register", nil)
				req.RemoteAddr = "192.168.1.100:12345"
				rec := httptest.NewRecorder()
				handler(rec, req)
				lastStatus = rec.Code
			}

			if lastStatus != tt.expectedStatus {
				t.Errorf("after %d requests, status = %d, want %d", tt.numRequests, lastStatus, tt.expectedStatus)
			}
		})
	}
}

func TestSnapshotMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		instanceID     string
		numRequests    int
		expectedStatus int
	}{
		{
			name:           "first request allowed",
			instanceID:     "instance-1",
			numRequests:    1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "second request blocked (no burst)",
			instanceID:     "instance-2",
			numRequests:    2,
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "no instance ID - fail open",
			instanceID:     "",
			numRequests:    5,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.RateLimitConfig{
				Enabled:         true,
				CleanupInterval: 0,
				Snapshot: config.RateLimitRouteConfig{
					Requests: 1,
					Period:   time.Minute,
					Burst:    1,
				},
			}
			rl := NewRateLimiter(cfg)
			defer rl.Stop()

			handler := rl.SnapshotMiddleware(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			var lastStatus int
			for i := 0; i < tt.numRequests; i++ {
				req := httptest.NewRequest("POST", "/v1/snapshot", nil)
				if tt.instanceID != "" {
					req.Header.Set("X-Instance-ID", tt.instanceID)
				}
				rec := httptest.NewRecorder()
				handler(rec, req)
				lastStatus = rec.Code
			}

			if lastStatus != tt.expectedStatus {
				t.Errorf("after %d requests, status = %d, want %d", tt.numRequests, lastStatus, tt.expectedStatus)
			}
		})
	}
}

func TestAdminMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		numRequests    int
		expectedStatus int
	}{
		{
			name:           "within rate limit",
			numRequests:    5,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "at burst limit",
			numRequests:    10,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "exceeds rate limit",
			numRequests:    15,
			expectedStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.RateLimitConfig{
				Enabled:             true,
				CleanupInterval:     0,
				Admin:               config.RateLimitRouteConfig{Requests: 30, Period: time.Minute, Burst: 10},
				BruteForceThreshold: 5,
				BruteForceBan:       15 * time.Minute,
			}
			rl := NewRateLimiter(cfg)
			defer rl.Stop()

			handler := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			var lastStatus int
			for i := 0; i < tt.numRequests; i++ {
				req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
				req.RemoteAddr = "192.168.1.200:12345"
				rec := httptest.NewRecorder()
				handler(rec, req)
				lastStatus = rec.Code
			}

			if lastStatus != tt.expectedStatus {
				t.Errorf("after %d requests, status = %d, want %d", tt.numRequests, lastStatus, tt.expectedStatus)
			}
		})
	}
}

func TestBruteForceProtection(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 3,
		BruteForceBan:       100 * time.Millisecond,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	clientIP := "192.168.1.50:12345"

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
		req.RemoteAddr = clientIP
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("request %d: got status %d, want %d", i+1, rec.Code, http.StatusUnauthorized)
		}
	}

	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("after ban: got status %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	time.Sleep(150 * time.Millisecond)

	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after ban expired: got status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRateLimitHeaders(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Register:        config.RateLimitRouteConfig{Requests: 5, Period: time.Minute, Burst: 2},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/v1/register", nil)
	req.RemoteAddr = "192.168.1.150:12345"
	rec := httptest.NewRecorder()
	handler(rec, req)

	if limit := rec.Header().Get("X-RateLimit-Limit"); limit != "5" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", limit, "5")
	}
	if remaining := rec.Header().Get("X-RateLimit-Remaining"); remaining == "" {
		t.Error("X-RateLimit-Remaining header missing")
	}
	if reset := rec.Header().Get("X-RateLimit-Reset"); reset == "" {
		t.Error("X-RateLimit-Reset header missing")
	}
}

func TestRetryAfterHeader(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Register:        config.RateLimitRouteConfig{Requests: 1, Period: time.Minute, Burst: 1},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/v1/register", nil)
	req.RemoteAddr = "192.168.1.175:12345"
	rec := httptest.NewRecorder()
	handler(rec, req)

	req = httptest.NewRequest("POST", "/v1/register", nil)
	req.RemoteAddr = "192.168.1.175:12345"
	rec = httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
		t.Error("Retry-After header missing on 429 response")
	}
}

func TestDisabledRateLimiting(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         false,
		CleanupInterval: 0,
		Register:        config.RateLimitRouteConfig{Requests: 1, Period: time.Minute, Burst: 1},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("POST", "/v1/register", nil)
		req.RemoteAddr = "192.168.1.200:12345"
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: got status %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Register:        config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 50},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var wg sync.WaitGroup
	numGoroutines := 50
	requestsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("POST", "/v1/register", nil)
				req.RemoteAddr = "192.168.1.100:12345"
				rec := httptest.NewRecorder()
				handler(rec, req)
			}
		}()
	}

	wg.Wait()
}

func TestDifferentKeysIndependent(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Snapshot:        config.RateLimitRouteConfig{Requests: 1, Period: time.Minute, Burst: 1},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.SnapshotMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req1 := httptest.NewRequest("POST", "/v1/snapshot", nil)
	req1.Header.Set("X-Instance-ID", "instance-A")
	rec1 := httptest.NewRecorder()
	handler(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("instance-A first request: got %d, want %d", rec1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest("POST", "/v1/snapshot", nil)
	req2.Header.Set("X-Instance-ID", "instance-B")
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("instance-B first request: got %d, want %d", rec2.Code, http.StatusOK)
	}

	req3 := httptest.NewRequest("POST", "/v1/snapshot", nil)
	req3.Header.Set("X-Instance-ID", "instance-A")
	rec3 := httptest.NewRecorder()
	handler(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("instance-A second request: got %d, want %d", rec3.Code, http.StatusTooManyRequests)
	}
}

// =============================================================================
// BUSINESS LOGIC TESTS - Token Recovery & Rate Limit Behavior
// =============================================================================

func TestTokenRecoveryAfterRateLimit(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Register: config.RateLimitRouteConfig{
			Requests: 1,
			Period:   50 * time.Millisecond,
			Burst:    1,
		},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(okHandler)
	clientIP := "10.0.0.1:1234"

	// First request - should succeed
	req := httptest.NewRequest("POST", "/v1/register", nil)
	req.RemoteAddr = clientIP
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec.Code)
	}

	// Second request immediately - should be rate limited
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec.Code)
	}

	// Wait for token regeneration
	time.Sleep(60 * time.Millisecond)

	// Third request after recovery - should succeed
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("after recovery: expected 200, got %d", rec.Code)
	}
}

// =============================================================================
// BUSINESS LOGIC TESTS - Brute Force Protection
// =============================================================================

func TestBruteForceIsolationMultipleIPs(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 2,
		BruteForceBan:       time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	// Ban IP1 by triggering brute force threshold
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
		req.RemoteAddr = "1.1.1.1:1234"
		handler(httptest.NewRecorder(), req)
	}

	// Verify IP1 is banned
	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = "1.1.1.1:1234"
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 should be banned, got status %d", rec.Code)
	}

	// IP2 must remain independent and not be banned
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = "2.2.2.2:1234"
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Error("IP2 was banned but should be independent from IP1")
	}

	// IP3 must also be independent
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Error("IP3 was banned but should be independent")
	}
}

func TestBruteForceNoResetAfterSuccess(t *testing.T) {
	// This test documents current behavior: failure counter does NOT reset after success
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 3,
		BruteForceBan:       time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	failCount := 0
	handler := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if failCount < 2 {
			failCount++
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusOK) // Success!
		}
	})

	clientIP := "192.168.50.1:1234"

	// 2 failures
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
		req.RemoteAddr = clientIP
		handler(httptest.NewRecorder(), req)
	}

	// 1 success (failCount = 2, so returns 200)
	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected success, got %d", rec.Code)
	}

	// Reset handler to always fail
	handler = rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	// 1 more failure should trigger ban (2 + 1 = 3 >= threshold)
	// Current behavior: counter is NOT reset, so this triggers ban
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	rec = httptest.NewRecorder()
	handler(rec, req)

	// Next request should be banned
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected ban after cumulative failures, got %d (counter not reset after success)", rec.Code)
	}
}

func TestBruteForceBanExpiry(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 1,
		BruteForceBan:       50 * time.Millisecond,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	clientIP := "192.168.100.1:1234"

	// Trigger ban
	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = clientIP
	handler(httptest.NewRecorder(), req)

	// Verify banned
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("should be banned, got %d", rec.Code)
	}

	// Wait for ban to expire
	time.Sleep(60 * time.Millisecond)

	// Should be able to make requests again
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Error("ban should have expired")
	}
}

// =============================================================================
// BUSINESS LOGIC TESTS - Cleanup
// =============================================================================

func TestCleanupRemovesExpiredEntries(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 50 * time.Millisecond,
		Register:        config.RateLimitRouteConfig{Requests: 10, Period: time.Minute, Burst: 5},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(okHandler)

	// Create entries for multiple IPs
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/v1/register", nil)
		req.RemoteAddr = "10.0.0." + string(rune('0'+i)) + ":1234"
		handler(httptest.NewRecorder(), req)
	}

	// Verify entries exist
	count := 0
	rl.ipLimiters.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	if count == 0 {
		t.Fatal("no entries created")
	}

	// Wait for cleanup (CleanupInterval * 2 + buffer)
	time.Sleep(150 * time.Millisecond)

	// Entries should be cleaned up (lastSeen older than threshold)
	countAfter := 0
	rl.ipLimiters.Range(func(_, _ interface{}) bool {
		countAfter++
		return true
	})
	if countAfter >= count {
		t.Errorf("cleanup did not remove entries: before=%d, after=%d", count, countAfter)
	}
}

// =============================================================================
// SECURITY TESTS - IP Handling
// =============================================================================

func TestMalformedXForwardedFor(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		expectedIP string
	}{
		{
			name:       "SQL injection attempt",
			xff:        "127.0.0.1; DROP TABLE users;--",
			expectedIP: "127.0.0.1; DROP TABLE users;--", // Stored as-is, SQL layer must sanitize
		},
		{
			name:       "XSS attempt",
			xff:        "<script>alert('xss')</script>",
			expectedIP: "<script>alert('xss')</script>",
		},
		{
			name:       "empty first IP in chain",
			xff:        ", 192.168.1.1",
			expectedIP: "", // First part is empty after trim
		},
		{
			name:       "whitespace only",
			xff:        "   ",
			expectedIP: "", // Empty after trim, will fallback
		},
		{
			name:       "very long IP (potential DoS)",
			xff:        string(make([]byte, 1000)),
			expectedIP: string(make([]byte, 1000)),
		},
		{
			name:       "null bytes",
			xff:        "192.168.1.1\x00malicious",
			expectedIP: "192.168.1.1\x00malicious",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			req.Header.Set("X-Forwarded-For", tt.xff)

			ip := getClientIP(req)

			// For empty results, should fallback to X-Real-IP or RemoteAddr
			if tt.expectedIP == "" {
				if ip == "" {
					t.Error("should fallback to RemoteAddr when XFF is empty")
				}
			} else if ip != tt.expectedIP {
				t.Errorf("getClientIP() = %q, want %q", ip, tt.expectedIP)
			}
		})
	}
}

func TestXForwardedForWithMultipleProxies(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")

	ip := getClientIP(req)

	// Should return the first (leftmost) IP - the original client
	if ip != "203.0.113.50" {
		t.Errorf("expected first IP in chain, got %q", ip)
	}
}

// =============================================================================
// SECURITY TESTS - Admin Auth Detection
// =============================================================================

func TestAdminMiddlewareDetects401And403(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 2,
		BruteForceBan:       time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	// Test 401 Unauthorized
	handler401 := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
		req.RemoteAddr = "192.168.1.10:1234"
		handler401(httptest.NewRecorder(), req)
	}

	// Should be banned after 401s
	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = "192.168.1.10:1234"
	rec := httptest.NewRecorder()
	handler401(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("401 should trigger brute force, got %d", rec.Code)
	}

	// Test 403 Forbidden with different IP
	handler403 := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
		req.RemoteAddr = "192.168.1.20:1234"
		handler403(httptest.NewRecorder(), req)
	}

	// Should be banned after 403s
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = "192.168.1.20:1234"
	rec = httptest.NewRecorder()
	handler403(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("403 should trigger brute force, got %d", rec.Code)
	}
}

func TestAdminMiddlewareIgnoresOtherStatusCodes(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 2,
		BruteForceBan:       time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	// 400 Bad Request should NOT trigger brute force
	handler400 := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
		req.RemoteAddr = "192.168.1.30:1234"
		handler400(httptest.NewRecorder(), req)
	}

	// Should NOT be banned
	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = "192.168.1.30:1234"
	rec := httptest.NewRecorder()
	handler400(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Error("400 should not trigger brute force ban")
	}
}

// =============================================================================
// CONCURRENCY TESTS
// =============================================================================

func TestConcurrentLimiterCreation(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Register:        config.RateLimitRouteConfig{Requests: 1000, Period: time.Minute, Burst: 100},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(okHandler)

	// Same IP, many goroutines - tests LoadOrStore race
	var wg sync.WaitGroup
	sameIP := "192.168.1.1:1234"

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/v1/register", nil)
			req.RemoteAddr = sameIP
			rec := httptest.NewRecorder()
			handler(rec, req)
		}()
	}

	wg.Wait()

	// Verify only one limiter was created for this IP
	count := 0
	rl.ipLimiters.Range(func(key, _ interface{}) bool {
		if key == "192.168.1.1" {
			count++
		}
		return true
	})
	if count != 1 {
		t.Errorf("expected 1 limiter for IP, got %d", count)
	}
}

func TestConcurrentAccessWithAssertions(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:         true,
		CleanupInterval: 0,
		Register:        config.RateLimitRouteConfig{Requests: 10, Period: time.Minute, Burst: 10},
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	handler := rl.RegisterMiddleware(okHandler)

	var wg sync.WaitGroup
	var successCount, rateLimitedCount atomic.Int32

	numGoroutines := 50
	requestsPerGoroutine := 2

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("POST", "/v1/register", nil)
				req.RemoteAddr = "192.168.1.100:12345"
				rec := httptest.NewRecorder()
				handler(rec, req)

				if rec.Code == http.StatusOK {
					successCount.Add(1)
				} else if rec.Code == http.StatusTooManyRequests {
					rateLimitedCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	total := successCount.Load() + rateLimitedCount.Load()
	expectedTotal := int32(numGoroutines * requestsPerGoroutine)

	if total != expectedTotal {
		t.Errorf("total requests = %d, want %d", total, expectedTotal)
	}

	// With burst=10, we expect ~10 successes and ~90 rate limited
	if successCount.Load() > 15 {
		t.Errorf("too many successes: %d (expected ~10 with burst=10)", successCount.Load())
	}
	if rateLimitedCount.Load() < 80 {
		t.Errorf("not enough rate limiting: %d rate limited (expected ~90)", rateLimitedCount.Load())
	}
}

// =============================================================================
// RESPONSE WRITER TESTS
// =============================================================================

func TestResponseWriterMultipleWriteHeader(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:             true,
		CleanupInterval:     0,
		Admin:               config.RateLimitRouteConfig{Requests: 100, Period: time.Minute, Burst: 100},
		BruteForceThreshold: 5,
		BruteForceBan:       time.Hour,
	}
	rl := NewRateLimiter(cfg)
	defer rl.Stop()

	// Handler that calls WriteHeader multiple times
	handler := rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.WriteHeader(http.StatusUnauthorized) // Second call should be ignored by http
	})

	req := httptest.NewRequest("POST", "/api/v1/admin/action", nil)
	req.RemoteAddr = "192.168.1.40:1234"
	rec := httptest.NewRecorder()
	handler(rec, req)

	// First WriteHeader wins (200 OK)
	if rec.Code != http.StatusOK {
		t.Errorf("expected first WriteHeader to win, got %d", rec.Code)
	}
}
