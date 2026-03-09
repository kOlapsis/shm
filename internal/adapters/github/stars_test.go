// SPDX-License-Identifier: AGPL-3.0-or-later

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/kolapsis/shm/internal/domain"
)

func TestStarsService_GetStars(t *testing.T) {
	ctx := context.Background()

	t.Run("fetches stars successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/owner/repo" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"stargazers_count": 42}`))
		}))
		defer server.Close()

		service := NewStarsService("")
		service.httpClient.Transport = &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return http.ProxyURL(nil)(req)
			},
		}

		// Override API URL for testing
		repoURL, _ := domain.NewGitHubURL("https://github.com/owner/repo")

		// Replace the default client with one that redirects to test server
		oldClient := service.httpClient
		service.httpClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: &mockTransport{server: server},
		}
		defer func() { service.httpClient = oldClient }()

		stars, err := service.GetStars(ctx, repoURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if stars != 42 {
			t.Errorf("expected 42 stars, got %d", stars)
		}
	})

	t.Run("handles 404 not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message": "Not Found"}`))
		}))
		defer server.Close()

		service := NewStarsService("")
		service.httpClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: &mockTransport{server: server},
		}

		repoURL, _ := domain.NewGitHubURL("https://github.com/owner/nonexistent")

		stars, err := service.GetStars(ctx, repoURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should return 0 for non-existent repos
		if stars != 0 {
			t.Errorf("expected 0 stars for 404, got %d", stars)
		}
	})

	t.Run("handles rate limit (403)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
		}))
		defer server.Close()

		service := NewStarsService("")
		service.httpClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: &mockTransport{server: server},
		}

		repoURL, _ := domain.NewGitHubURL("https://github.com/owner/repo")

		_, err := service.GetStars(ctx, repoURL)
		if err == nil {
			t.Error("expected error for rate limit")
		}
	})

	t.Run("uses authorization header with token", func(t *testing.T) {
		tokenReceived := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer test-token" {
				tokenReceived = true
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"stargazers_count": 100}`))
		}))
		defer server.Close()

		service := NewStarsService("test-token")
		service.httpClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: &mockTransport{server: server},
		}

		repoURL, _ := domain.NewGitHubURL("https://github.com/owner/repo")

		_, err := service.GetStars(ctx, repoURL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !tokenReceived {
			t.Error("token was not sent in Authorization header")
		}
	})

	t.Run("returns error for empty URL", func(t *testing.T) {
		service := NewStarsService("")
		_, err := service.GetStars(ctx, "")
		if err == nil {
			t.Error("expected error for empty URL")
		}
	})
}

func TestStarsCache(t *testing.T) {
	t.Run("cache miss", func(t *testing.T) {
		cache := &starsCache{
			entries: make(map[string]cacheEntry),
		}
		_, ok := cache.get("key1")
		if ok {
			t.Error("expected cache miss")
		}
	})

	t.Run("cache hit", func(t *testing.T) {
		cache := &starsCache{
			entries: make(map[string]cacheEntry),
		}
		cache.set("key1", 42, 1*time.Hour)
		stars, ok := cache.get("key1")
		if !ok {
			t.Error("expected cache hit")
		}
		if stars != 42 {
			t.Errorf("expected 42, got %d", stars)
		}
	})

	t.Run("cache expiration", func(t *testing.T) {
		cache := &starsCache{
			entries: make(map[string]cacheEntry),
		}
		cache.set("key2", 100, 10*time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		_, ok := cache.get("key2")
		if ok {
			t.Error("expected cache miss after expiration")
		}
	})

	t.Run("cleanup removes expired entries", func(t *testing.T) {
		cache := &starsCache{
			entries: make(map[string]cacheEntry),
		}
		cache.set("expired", 1, 1*time.Millisecond)
		cache.set("valid", 2, 1*time.Hour)

		time.Sleep(5 * time.Millisecond)
		cache.cleanup()

		if len(cache.entries) != 1 {
			t.Errorf("expected 1 entry after cleanup, got %d", len(cache.entries))
		}

		_, ok := cache.get("valid")
		if !ok {
			t.Error("valid entry should still exist")
		}
	})
}

func TestStarsService_Caching(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stargazers_count": 42}`))
	}))
	defer server.Close()

	service := NewStarsService("")
	service.httpClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: &mockTransport{server: server},
	}

	repoURL, _ := domain.NewGitHubURL("https://github.com/owner/repo")

	// First call - should hit API
	stars1, err := service.GetStars(ctx, repoURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call - should use cache
	stars2, err := service.GetStars(ctx, repoURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stars1 != stars2 {
		t.Errorf("expected same stars value, got %d and %d", stars1, stars2)
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call due to caching, got %d", callCount)
	}
}

// mockTransport redirects all requests to the test server
type mockTransport struct {
	server *httptest.Server
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite URL to point to test server
	req.URL.Scheme = "http"
	req.URL.Host = m.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}
