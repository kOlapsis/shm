// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewApplicationID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "valid UUID",
			id:      "550e8400-e29b-41d4-a716-446655440000",
			wantErr: nil,
		},
		{
			name:    "empty string",
			id:      "",
			wantErr: ErrInvalidApplicationID,
		},
		{
			name:    "invalid format",
			id:      "not-a-uuid",
			wantErr: ErrInvalidApplicationID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewApplicationID(tt.id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if id.String() != tt.id {
				t.Errorf("expected %s, got %s", tt.id, id.String())
			}
		})
	}
}

func TestNewAppSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr error
	}{
		{
			name:    "valid slug",
			slug:    "my-app",
			wantErr: nil,
		},
		{
			name:    "single word",
			slug:    "app",
			wantErr: nil,
		},
		{
			name:    "with numbers",
			slug:    "app-v2",
			wantErr: nil,
		},
		{
			name:    "empty string",
			slug:    "",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "uppercase not allowed",
			slug:    "My-App",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "underscores not allowed",
			slug:    "my_app",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "spaces not allowed",
			slug:    "my app",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "double hyphens not allowed",
			slug:    "my--app",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "leading hyphen not allowed",
			slug:    "-myapp",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "trailing hyphen not allowed",
			slug:    "myapp-",
			wantErr: ErrInvalidAppSlug,
		},
		{
			name:    "too long",
			slug:    string(make([]byte, 101)),
			wantErr: ErrInvalidAppSlug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, err := NewAppSlug(tt.slug)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if slug.String() != tt.slug {
				t.Errorf("expected %s, got %s", tt.slug, slug.String())
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Cool App", "my-cool-app"},
		{"MyApp", "myapp"},
		{"my_app", "my-app"},
		{"my  app", "my-app"},
		{"My-App-v2.0", "my-app-v2-0"},
		{"Café", "cafe"},
		{"naïve", "naive"},
		{"São Paulo", "sao-paulo"},
		{"---test---", "test"},
		{"", "app"},
		{"123", "123"},
		{"àáâãäå", "aaaaaa"},
		{"Test@#$%App", "test-app"},
		{"hello    world", "hello-world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Slugify(tt.input)
			if result.String() != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewGitHubURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr error
	}{
		{
			name:    "valid URL",
			url:     "https://github.com/kolapsis/shm",
			want:    "https://github.com/kolapsis/shm",
			wantErr: nil,
		},
		{
			name:    "valid URL with trailing slash",
			url:     "https://github.com/kolapsis/shm/",
			want:    "https://github.com/kolapsis/shm",
			wantErr: nil,
		},
		{
			name:    "empty string allowed",
			url:     "",
			want:    "",
			wantErr: nil,
		},
		{
			name:    "invalid domain",
			url:     "https://gitlab.com/owner/repo",
			wantErr: ErrInvalidGitHubURL,
		},
		{
			name:    "missing owner",
			url:     "https://github.com/repo",
			wantErr: ErrInvalidGitHubURL,
		},
		{
			name:    "too many path segments",
			url:     "https://github.com/owner/repo/issues",
			wantErr: ErrInvalidGitHubURL,
		},
		{
			name:    "not a URL",
			url:     "not-a-url",
			wantErr: ErrInvalidGitHubURL,
		},
		{
			name:    "http not allowed",
			url:     "http://github.com/owner/repo",
			wantErr: ErrInvalidGitHubURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGitHubURL(tt.url)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got.String())
			}
		})
	}
}

func TestGitHubURL_OwnerAndRepo(t *testing.T) {
	tests := []struct {
		name      string
		url       GitHubURL
		wantOwner string
		wantRepo  string
		wantErr   error
	}{
		{
			name:      "valid URL",
			url:       "https://github.com/kolapsis/shm",
			wantOwner: "btouchard",
			wantRepo:  "shm",
			wantErr:   nil,
		},
		{
			name:      "with hyphen and underscore",
			url:       "https://github.com/my-org/my_repo",
			wantOwner: "my-org",
			wantRepo:  "my_repo",
			wantErr:   nil,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: ErrInvalidGitHubURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := tt.url.OwnerAndRepo()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner: expected %s, got %s", tt.wantOwner, owner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo: expected %s, got %s", tt.wantRepo, repo)
			}
		})
	}
}

func TestNewApplication(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		appName string
		wantErr error
	}{
		{
			name:    "valid application",
			slug:    "my-app",
			appName: "My App",
			wantErr: nil,
		},
		{
			name:    "empty name",
			slug:    "my-app",
			appName: "",
			wantErr: ErrInvalidApplication,
		},
		{
			name:    "invalid slug",
			slug:    "My App",
			appName: "My App",
			wantErr: ErrInvalidAppSlug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := NewApplication(tt.slug, tt.appName)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if app.Slug.String() != tt.slug {
				t.Errorf("slug: expected %s, got %s", tt.slug, app.Slug)
			}
			if app.Name != tt.appName {
				t.Errorf("name: expected %s, got %s", tt.appName, app.Name)
			}
			if app.Stars != 0 {
				t.Errorf("stars should be 0, got %d", app.Stars)
			}
		})
	}
}

func TestApplication_SetGitHubURL(t *testing.T) {
	app, _ := NewApplication("my-app", "My App")
	oldUpdatedAt := app.UpdatedAt

	time.Sleep(10 * time.Millisecond) // Ensure time difference

	err := app.SetGitHubURL("https://github.com/owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app.GitHubURL.String() != "https://github.com/owner/repo" {
		t.Errorf("expected URL to be set")
	}

	if !app.UpdatedAt.After(oldUpdatedAt) {
		t.Errorf("UpdatedAt should be updated")
	}

	// Test invalid URL
	err = app.SetGitHubURL("invalid-url")
	if !errors.Is(err, ErrInvalidGitHubURL) {
		t.Errorf("expected ErrInvalidGitHubURL, got %v", err)
	}
}

func TestApplication_SetLogoURL(t *testing.T) {
	app, _ := NewApplication("my-app", "My App")
	oldUpdatedAt := app.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	app.SetLogoURL("https://example.com/logo.png")

	if app.LogoURL != "https://example.com/logo.png" {
		t.Errorf("expected logo URL to be set")
	}

	if !app.UpdatedAt.After(oldUpdatedAt) {
		t.Errorf("UpdatedAt should be updated")
	}
}

func TestApplication_UpdateStars(t *testing.T) {
	app, _ := NewApplication("my-app", "My App")

	if app.StarsUpdatedAt != nil {
		t.Errorf("StarsUpdatedAt should be nil initially")
	}

	app.UpdateStars(42)

	if app.Stars != 42 {
		t.Errorf("expected stars to be 42, got %d", app.Stars)
	}

	if app.StarsUpdatedAt == nil {
		t.Errorf("StarsUpdatedAt should be set")
	}

	// Test negative stars are clamped to 0
	app.UpdateStars(-10)
	if app.Stars != 0 {
		t.Errorf("expected stars to be 0 for negative input, got %d", app.Stars)
	}
}

func TestApplication_NeedsStarsRefresh(t *testing.T) {
	app, _ := NewApplication("my-app", "My App")

	// No GitHub URL
	if app.NeedsStarsRefresh() {
		t.Errorf("should not need refresh without GitHub URL")
	}

	// With GitHub URL but never fetched
	_ = app.SetGitHubURL("https://github.com/owner/repo")
	if !app.NeedsStarsRefresh() {
		t.Errorf("should need refresh when never fetched")
	}

	// Fresh data
	app.UpdateStars(10)
	if app.NeedsStarsRefresh() {
		t.Errorf("should not need refresh when data is fresh")
	}

	// Stale data (simulate old timestamp)
	oldTime := time.Now().Add(-2 * time.Hour)
	app.StarsUpdatedAt = &oldTime
	if !app.NeedsStarsRefresh() {
		t.Errorf("should need refresh when data is stale")
	}
}
