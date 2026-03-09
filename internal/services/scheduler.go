// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/shm/internal/app"
)

// Scheduler handles background periodic tasks.
type Scheduler struct {
	appService *app.ApplicationService
	logger     *slog.Logger
}

// NewScheduler creates a new Scheduler.
func NewScheduler(appService *app.ApplicationService, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		appService: appService,
		logger:     logger,
	}
}

// Start begins running scheduled tasks in the background.
// This function blocks until the context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	// Refresh GitHub stars every hour
	starsRefreshTicker := time.NewTicker(1 * time.Hour)
	defer starsRefreshTicker.Stop()

	s.logger.Info("scheduler started", "stars_refresh_interval", "1h")

	// Initial refresh on startup (after a small delay)
	time.AfterFunc(30*time.Second, func() {
		s.refreshStars(ctx)
	})

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-starsRefreshTicker.C:
			s.refreshStars(ctx)
		}
	}
}

// refreshStars refreshes GitHub stars for all applications.
func (s *Scheduler) refreshStars(ctx context.Context) {
	s.logger.Debug("starting GitHub stars refresh")

	if err := s.appService.RefreshAllStars(ctx); err != nil {
		s.logger.Error("failed to refresh GitHub stars", "error", err)
	} else {
		s.logger.Debug("GitHub stars refresh completed")
	}
}
