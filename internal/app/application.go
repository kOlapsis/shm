// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kolapsis/shm/internal/app/ports"
	"github.com/kolapsis/shm/internal/domain"
)

// UpdateApplicationInput holds the data for updating an application.
type UpdateApplicationInput struct {
	Slug      string
	GitHubURL string
	LogoURL   string
}

// ApplicationService handles application-related use cases.
type ApplicationService struct {
	repo   ports.ApplicationRepository
	github ports.GitHubService
	logger *slog.Logger
}

// NewApplicationService creates a new ApplicationService.
func NewApplicationService(
	repo ports.ApplicationRepository,
	github ports.GitHubService,
	logger *slog.Logger,
) *ApplicationService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ApplicationService{
		repo:   repo,
		github: github,
		logger: logger,
	}
}

// CreateOrGet creates a new application or returns an existing one by slug.
// This is used during instance registration to auto-create applications.
func (s *ApplicationService) CreateOrGet(ctx context.Context, appName string) (*domain.Application, error) {
	slug := domain.Slugify(appName)

	// Try to find existing application
	existing, err := s.repo.FindBySlug(ctx, slug)
	if err == nil {
		return existing, nil
	}

	if !errors.Is(err, domain.ErrApplicationNotFound) {
		return nil, fmt.Errorf("create or get application: %w", err)
	}

	// Create new application
	app, err := domain.NewApplication(slug.String(), appName)
	if err != nil {
		return nil, fmt.Errorf("create or get application: %w", err)
	}

	if err := s.repo.Save(ctx, app); err != nil {
		return nil, fmt.Errorf("create or get application: %w", err)
	}

	s.logger.Info("application auto-created", "slug", slug, "name", appName)
	return app, nil
}

// GetBySlug retrieves an application by its slug.
func (s *ApplicationService) GetBySlug(ctx context.Context, slug string) (*domain.Application, error) {
	appSlug, err := domain.NewAppSlug(slug)
	if err != nil {
		return nil, err
	}

	app, err := s.repo.FindBySlug(ctx, appSlug)
	if err != nil {
		return nil, fmt.Errorf("get application by slug: %w", err)
	}

	return app, nil
}

// List retrieves all applications.
func (s *ApplicationService) List(ctx context.Context, limit int) ([]*domain.Application, error) {
	apps, err := s.repo.List(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}

	return apps, nil
}

// Update updates an application's metadata.
func (s *ApplicationService) Update(ctx context.Context, input UpdateApplicationInput) error {
	appSlug, err := domain.NewAppSlug(input.Slug)
	if err != nil {
		return err
	}

	app, err := s.repo.FindBySlug(ctx, appSlug)
	if err != nil {
		return fmt.Errorf("update application: %w", err)
	}

	// Update GitHub URL if provided
	if input.GitHubURL != "" {
		if err := app.SetGitHubURL(input.GitHubURL); err != nil {
			return fmt.Errorf("update application: %w", err)
		}
	}

	// Update logo URL if provided
	if input.LogoURL != "" {
		app.SetLogoURL(input.LogoURL)
	}

	if err := s.repo.Save(ctx, app); err != nil {
		return fmt.Errorf("update application: %w", err)
	}

	s.logger.Info("application updated", "slug", input.Slug)
	return nil
}

// RefreshStars fetches fresh star count from GitHub for a specific application.
func (s *ApplicationService) RefreshStars(ctx context.Context, slug string) error {
	appSlug, err := domain.NewAppSlug(slug)
	if err != nil {
		return err
	}

	app, err := s.repo.FindBySlug(ctx, appSlug)
	if err != nil {
		return fmt.Errorf("refresh stars: %w", err)
	}

	if app.GitHubURL == "" {
		return fmt.Errorf("refresh stars: no GitHub URL configured for %s", slug)
	}

	// Fetch stars from GitHub
	stars, err := s.github.GetStars(ctx, app.GitHubURL)
	if err != nil {
		s.logger.Warn("failed to fetch GitHub stars",
			"slug", slug,
			"github_url", app.GitHubURL,
			"error", err,
		)
		return fmt.Errorf("refresh stars: %w", err)
	}

	// Update in database
	app.UpdateStars(stars)
	if err := s.repo.Save(ctx, app); err != nil {
		return fmt.Errorf("refresh stars: %w", err)
	}

	s.logger.Info("GitHub stars refreshed", "slug", slug, "stars", stars)
	return nil
}

// RefreshAllStars refreshes GitHub stars for all applications that have a GitHub URL.
// Only refreshes if data is stale (based on Application.NeedsStarsRefresh).
func (s *ApplicationService) RefreshAllStars(ctx context.Context) error {
	apps, err := s.repo.List(ctx, 1000)
	if err != nil {
		return fmt.Errorf("refresh all stars: %w", err)
	}

	refreshed := 0
	failed := 0

	for _, app := range apps {
		if !app.NeedsStarsRefresh() {
			continue
		}

		stars, err := s.github.GetStars(ctx, app.GitHubURL)
		if err != nil {
			s.logger.Warn("failed to refresh stars",
				"slug", app.Slug,
				"error", err,
			)
			failed++
			continue
		}

		app.UpdateStars(stars)
		if err := s.repo.Save(ctx, app); err != nil {
			s.logger.Error("failed to save stars",
				"slug", app.Slug,
				"error", err,
			)
			failed++
			continue
		}

		refreshed++
		s.logger.Debug("stars refreshed", "slug", app.Slug, "stars", stars)
	}

	s.logger.Info("GitHub stars refresh completed",
		"refreshed", refreshed,
		"failed", failed,
		"total", len(apps),
	)

	return nil
}
