// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/shm/internal/app"
)

const cleanupInterval = 6 * time.Hour

// Scheduler handles background periodic tasks.
type Scheduler struct {
	appService      *app.ApplicationService
	instanceService *app.InstanceService
	logger          *slog.Logger
}

// NewScheduler creates a new Scheduler.
func NewScheduler(appService *app.ApplicationService, instanceService *app.InstanceService, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		appService:      appService,
		instanceService: instanceService,
		logger:          logger,
	}
}

// Start begins running scheduled tasks in the background.
// This function blocks until the context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	starsRefreshTicker := time.NewTicker(1 * time.Hour)
	defer starsRefreshTicker.Stop()

	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	s.logger.Info("scheduler started",
		"stars_refresh_interval", "1h",
		"cleanup_interval", cleanupInterval.String(),
	)

	time.AfterFunc(30*time.Second, func() {
		s.refreshStars(ctx)
		s.purgeAbandoned(ctx)
	})

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-starsRefreshTicker.C:
			s.refreshStars(ctx)
		case <-cleanupTicker.C:
			s.purgeAbandoned(ctx)
		}
	}
}

func (s *Scheduler) refreshStars(ctx context.Context) {
	s.logger.Debug("starting GitHub stars refresh")

	if err := s.appService.RefreshAllStars(ctx); err != nil {
		s.logger.Error("failed to refresh GitHub stars", "error", err)
	} else {
		s.logger.Debug("GitHub stars refresh completed")
	}
}

func (s *Scheduler) purgeAbandoned(ctx context.Context) {
	if s.instanceService == nil {
		return
	}
	n, err := s.instanceService.PurgeAbandoned(ctx)
	if err != nil {
		s.logger.Error("failed to purge abandoned instances", "error", err)
		return
	}
	if n > 0 {
		s.logger.Info("purged abandoned instances", "count", n)
	}
}
