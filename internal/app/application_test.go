// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/kolapsis/shm/internal/domain"
)

// mockApplicationRepository is a mock implementation of ports.ApplicationRepository
type mockApplicationRepository struct {
	apps          map[string]*domain.Application
	saveErr       error
	findBySlugErr error
}

func newMockApplicationRepository() *mockApplicationRepository {
	return &mockApplicationRepository{
		apps: make(map[string]*domain.Application),
	}
}

func (m *mockApplicationRepository) Save(ctx context.Context, app *domain.Application) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.apps[app.Slug.String()] = app
	return nil
}

func (m *mockApplicationRepository) FindByID(ctx context.Context, id domain.ApplicationID) (*domain.Application, error) {
	for _, app := range m.apps {
		if app.ID == id {
			return app, nil
		}
	}
	return nil, domain.ErrApplicationNotFound
}

func (m *mockApplicationRepository) FindBySlug(ctx context.Context, slug domain.AppSlug) (*domain.Application, error) {
	if m.findBySlugErr != nil {
		return nil, m.findBySlugErr
	}
	if app, ok := m.apps[slug.String()]; ok {
		return app, nil
	}
	return nil, domain.ErrApplicationNotFound
}

func (m *mockApplicationRepository) List(ctx context.Context, limit int) ([]*domain.Application, error) {
	result := make([]*domain.Application, 0, len(m.apps))
	for _, app := range m.apps {
		result = append(result, app)
	}
	return result, nil
}

func (m *mockApplicationRepository) UpdateStars(ctx context.Context, id domain.ApplicationID, stars int) error {
	for _, app := range m.apps {
		if app.ID == id {
			app.UpdateStars(stars)
			return nil
		}
	}
	return domain.ErrApplicationNotFound
}

// mockGitHubService is a mock implementation of ports.GitHubService
type mockGitHubService struct {
	stars      int
	getErr     error
	getStarsFn func(ctx context.Context, repoURL domain.GitHubURL) (int, error)
}

func (m *mockGitHubService) GetStars(ctx context.Context, repoURL domain.GitHubURL) (int, error) {
	// Allow tests to override the function
	if m.getStarsFn != nil {
		return m.getStarsFn(ctx, repoURL)
	}
	if m.getErr != nil {
		return 0, m.getErr
	}
	return m.stars, nil
}

func TestApplicationService_CreateOrGet(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new application", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		app, err := service.CreateOrGet(ctx, "My App")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if app.Name != "My App" {
			t.Errorf("expected name 'My App', got %s", app.Name)
		}

		if app.Slug.String() != "my-app" {
			t.Errorf("expected slug 'my-app', got %s", app.Slug)
		}

		if len(repo.apps) != 1 {
			t.Errorf("expected 1 app in repo, got %d", len(repo.apps))
		}
	})

	t.Run("returns existing application", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		// Create first time
		app1, _ := service.CreateOrGet(ctx, "My App")

		// Get second time
		app2, err := service.CreateOrGet(ctx, "My App")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if app1.ID != app2.ID {
			t.Error("expected same application ID")
		}

		if len(repo.apps) != 1 {
			t.Errorf("expected 1 app in repo, got %d", len(repo.apps))
		}
	})

	t.Run("normalizes app name to slug", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		// Create with different casing/spacing
		app1, _ := service.CreateOrGet(ctx, "My Cool App")
		app2, _ := service.CreateOrGet(ctx, "my cool app")
		app3, _ := service.CreateOrGet(ctx, "MY-COOL-APP")

		// All should return the same app
		if app1.ID != app2.ID || app2.ID != app3.ID {
			t.Error("expected same application for different name variations")
		}

		if len(repo.apps) != 1 {
			t.Errorf("expected 1 app in repo, got %d", len(repo.apps))
		}
	})
}

func TestApplicationService_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("updates GitHub URL", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		app, _ := service.CreateOrGet(ctx, "My App")

		err := service.Update(ctx, UpdateApplicationInput{
			Slug:      app.Slug.String(),
			GitHubURL: "https://github.com/owner/repo",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := repo.FindBySlug(ctx, app.Slug)
		if updated.GitHubURL.String() != "https://github.com/owner/repo" {
			t.Errorf("expected GitHub URL to be updated, got %s", updated.GitHubURL)
		}
	})

	t.Run("updates logo URL", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		app, _ := service.CreateOrGet(ctx, "My App")

		err := service.Update(ctx, UpdateApplicationInput{
			Slug:    app.Slug.String(),
			LogoURL: "https://example.com/logo.png",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := repo.FindBySlug(ctx, app.Slug)
		if updated.LogoURL != "https://example.com/logo.png" {
			t.Errorf("expected logo URL to be updated, got %s", updated.LogoURL)
		}
	})

	t.Run("returns error for non-existent app", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		err := service.Update(ctx, UpdateApplicationInput{
			Slug:      "nonexistent",
			GitHubURL: "https://github.com/owner/repo",
		})

		if !errors.Is(err, domain.ErrApplicationNotFound) {
			t.Errorf("expected ErrApplicationNotFound, got %v", err)
		}
	})
}

func TestApplicationService_RefreshStars(t *testing.T) {
	ctx := context.Background()

	t.Run("refreshes stars successfully", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{stars: 42}
		service := NewApplicationService(repo, github, nil)

		app, _ := service.CreateOrGet(ctx, "My App")
		_ = service.Update(ctx, UpdateApplicationInput{
			Slug:      app.Slug.String(),
			GitHubURL: "https://github.com/owner/repo",
		})

		err := service.RefreshStars(ctx, app.Slug.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		updated, _ := repo.FindBySlug(ctx, app.Slug)
		if updated.Stars != 42 {
			t.Errorf("expected 42 stars, got %d", updated.Stars)
		}

		if updated.StarsUpdatedAt == nil {
			t.Error("expected StarsUpdatedAt to be set")
		}
	})

	t.Run("returns error when no GitHub URL", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{}
		service := NewApplicationService(repo, github, nil)

		app, _ := service.CreateOrGet(ctx, "My App")

		err := service.RefreshStars(ctx, app.Slug.String())
		if err == nil {
			t.Error("expected error when no GitHub URL")
		}
	})

	t.Run("handles GitHub API error", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{getErr: errors.New("API error")}
		service := NewApplicationService(repo, github, nil)

		app, _ := service.CreateOrGet(ctx, "My App")
		_ = service.Update(ctx, UpdateApplicationInput{
			Slug:      app.Slug.String(),
			GitHubURL: "https://github.com/owner/repo",
		})

		err := service.RefreshStars(ctx, app.Slug.String())
		if err == nil {
			t.Error("expected error from GitHub API")
		}
	})
}

func TestApplicationService_RefreshAllStars(t *testing.T) {
	ctx := context.Background()

	t.Run("refreshes stars for all apps with GitHub URL", func(t *testing.T) {
		repo := newMockApplicationRepository()
		github := &mockGitHubService{stars: 100}
		service := NewApplicationService(repo, github, nil)

		// Create apps
		app1, _ := service.CreateOrGet(ctx, "App 1")
		app2, _ := service.CreateOrGet(ctx, "App 2")

		// Set GitHub URL only for app1
		_ = service.Update(ctx, UpdateApplicationInput{
			Slug:      app1.Slug.String(),
			GitHubURL: "https://github.com/owner/repo1",
		})

		// Force needs refresh
		app1, _ = repo.FindBySlug(ctx, app1.Slug)
		app1.StarsUpdatedAt = nil
		_ = repo.Save(ctx, app1)

		err := service.RefreshAllStars(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// app1 should have stars updated
		updated1, _ := repo.FindBySlug(ctx, app1.Slug)
		if updated1.Stars != 100 {
			t.Errorf("expected app1 to have 100 stars, got %d", updated1.Stars)
		}

		// app2 should not have stars (no GitHub URL)
		updated2, _ := repo.FindBySlug(ctx, app2.Slug)
		if updated2.Stars != 0 {
			t.Errorf("expected app2 to have 0 stars, got %d", updated2.Stars)
		}
	})

	t.Run("skips apps that don't need refresh", func(t *testing.T) {
		repo := newMockApplicationRepository()
		callCount := 0
		github := &mockGitHubService{stars: 50}

		// Wrap to count calls
		github.getStarsFn = func(ctx context.Context, repoURL domain.GitHubURL) (int, error) {
			callCount++
			return 50, nil
		}

		service := NewApplicationService(repo, github, nil)

		app, _ := service.CreateOrGet(ctx, "App")
		_ = service.Update(ctx, UpdateApplicationInput{
			Slug:      app.Slug.String(),
			GitHubURL: "https://github.com/owner/repo",
		})

		// First refresh
		_ = service.RefreshStars(ctx, app.Slug.String())
		callCount = 0 // Reset counter

		// Second refresh (should skip - data is fresh)
		_ = service.RefreshAllStars(ctx)

		if callCount != 0 {
			t.Errorf("expected 0 API calls for fresh data, got %d", callCount)
		}
	})
}
