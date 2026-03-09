// SPDX-License-Identifier: AGPL-3.0-or-later

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kolapsis/shm/internal/domain"
)

// StarsService implements ports.GitHubService for fetching GitHub repository stars.
type StarsService struct {
	httpClient *http.Client
	token      string // Optional GitHub token for higher rate limits
	cache      *starsCache
}

// starsCache provides in-memory caching with TTL.
type starsCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	stars     int
	expiresAt time.Time
}

// NewStarsService creates a new StarsService.
// token is optional - if empty, uses unauthenticated API (60 req/h limit).
func NewStarsService(token string) *StarsService {
	return &StarsService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		token:      token,
		cache: &starsCache{
			entries: make(map[string]cacheEntry),
		},
	}
}

// githubRepoResponse represents the GitHub API response for a repository.
type githubRepoResponse struct {
	StargazersCount int    `json:"stargazers_count"`
	Message         string `json:"message"` // Error message if any
}

// GetStars fetches the current star count for a GitHub repository.
// Uses cache if available and not expired (1 hour TTL).
// Returns 0 if the repository doesn't exist or on error (fail-safe).
func (s *StarsService) GetStars(ctx context.Context, repoURL domain.GitHubURL) (int, error) {
	if repoURL == "" {
		return 0, fmt.Errorf("empty repository URL")
	}

	// Check cache first
	if stars, ok := s.cache.get(repoURL.String()); ok {
		return stars, nil
	}

	// Fetch from GitHub API
	owner, repo, err := repoURL.OwnerAndRepo()
	if err != nil {
		return 0, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch GitHub API: %w", err)
	}
	defer resp.Body.Close()

	// Handle HTTP errors
	if resp.StatusCode == http.StatusNotFound {
		// Repository doesn't exist - cache 0 stars
		s.cache.set(repoURL.String(), 0, 1*time.Hour)
		return 0, nil
	}

	if resp.StatusCode == http.StatusForbidden {
		// Rate limit exceeded - return cached value or 0
		return 0, fmt.Errorf("GitHub API rate limit exceeded (status 403)")
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Parse response
	var apiResp githubRepoResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Message != "" {
		return 0, fmt.Errorf("GitHub API error: %s", apiResp.Message)
	}

	// Cache result
	s.cache.set(repoURL.String(), apiResp.StargazersCount, 1*time.Hour)

	return apiResp.StargazersCount, nil
}

// get retrieves a cached value if it exists and hasn't expired.
func (c *starsCache) get(key string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return 0, false
	}

	if time.Now().After(entry.expiresAt) {
		return 0, false
	}

	return entry.stars, true
}

// set stores a value in the cache with the given TTL.
func (c *starsCache) set(key string, stars int, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		stars:     stars,
		expiresAt: time.Now().Add(ttl),
	}
}

// cleanup removes expired entries from the cache.
// Should be called periodically by a background goroutine.
func (c *starsCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// StartCleanup starts a background goroutine to periodically clean up expired cache entries.
func (s *StarsService) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cache.cleanup()
			}
		}
	}()
}
