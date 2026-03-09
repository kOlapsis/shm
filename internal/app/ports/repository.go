// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ports defines the interfaces (ports) used by the application layer.
// These interfaces are implemented by adapters (repositories, external services).
// Following hexagonal architecture: interfaces are declared where they are consumed.
package ports

import (
	"context"
	"time"

	"github.com/kolapsis/shm/internal/domain"
)

// InstanceRepository defines persistence operations for instances.
type InstanceRepository interface {
	// Save persists an instance (insert or update).
	Save(ctx context.Context, instance *domain.Instance) error

	// FindByID retrieves an instance by its ID.
	// Returns domain.ErrInstanceNotFound if not found.
	FindByID(ctx context.Context, id domain.InstanceID) (*domain.Instance, error)

	// GetPublicKey retrieves the public key for an instance.
	// Returns domain.ErrInstanceNotFound if not found.
	// Returns domain.ErrInstanceRevoked if the instance is revoked.
	GetPublicKey(ctx context.Context, id domain.InstanceID) (domain.PublicKey, error)

	// UpdateStatus updates the status and last_seen_at timestamp.
	UpdateStatus(ctx context.Context, id domain.InstanceID, status domain.InstanceStatus) error

	// Delete removes an instance and all its associated snapshots.
	// Returns domain.ErrInstanceNotFound if the instance doesn't exist.
	Delete(ctx context.Context, id domain.InstanceID) error
}

// SnapshotRepository defines persistence operations for snapshots.
type SnapshotRepository interface {
	// Save persists a snapshot and updates the instance heartbeat.
	Save(ctx context.Context, snapshot *domain.Snapshot) error

	// FindByInstanceID retrieves snapshots for an instance.
	FindByInstanceID(ctx context.Context, id domain.InstanceID, limit int) ([]*domain.Snapshot, error)

	// GetLatestByInstanceID retrieves the most recent snapshot for an instance.
	GetLatestByInstanceID(ctx context.Context, id domain.InstanceID) (*domain.Snapshot, error)
}

// DashboardStats holds aggregated statistics for the dashboard.
type DashboardStats struct {
	TotalInstances  int
	ActiveInstances int
	GlobalMetrics   map[string]int64
	PerAppCounts    map[string]int // Instance count per app_name
}

// InstanceSummary holds instance data with latest metrics for listing.
type InstanceSummary struct {
	ID             domain.InstanceID
	AppName        string
	AppSlug        string // From applications table
	AppVersion     string
	Environment    string
	Status         domain.InstanceStatus
	DeploymentMode string
	LastSeenAt     time.Time
	Metrics        domain.Metrics
	// Application metadata
	GitHubURL   string
	GitHubStars int
	LogoURL     string
}

// MetricsTimeSeries holds time-series data for charting.
type MetricsTimeSeries struct {
	Timestamps []time.Time
	Metrics    map[string][]float64
}

// ApplicationRepository defines persistence operations for applications.
type ApplicationRepository interface {
	// Save persists an application (insert or update).
	Save(ctx context.Context, app *domain.Application) error

	// FindByID retrieves an application by its ID.
	// Returns domain.ErrApplicationNotFound if not found.
	FindByID(ctx context.Context, id domain.ApplicationID) (*domain.Application, error)

	// FindBySlug retrieves an application by its slug.
	// Returns domain.ErrApplicationNotFound if not found.
	FindBySlug(ctx context.Context, slug domain.AppSlug) (*domain.Application, error)

	// List retrieves all applications, limited by the specified count.
	List(ctx context.Context, limit int) ([]*domain.Application, error)

	// UpdateStars updates only the GitHub stars count and timestamp.
	UpdateStars(ctx context.Context, id domain.ApplicationID, stars int) error
}

// GitHubService defines external GitHub API operations.
type GitHubService interface {
	// GetStars fetches the current star count for a GitHub repository.
	GetStars(ctx context.Context, repoURL domain.GitHubURL) (int, error)
}

// DashboardReader defines read operations for the dashboard.
// Separated from write repositories for CQRS-lite pattern.
type DashboardReader interface {
	// GetStats returns aggregated dashboard statistics.
	GetStats(ctx context.Context) (DashboardStats, error)

	// ListInstances returns instances with their latest metrics.
	// offset and limit are used for pagination.
	// appName filters by app name (empty = all apps).
	// search filters by instance_id, version, environment, or deployment_mode.
	ListInstances(ctx context.Context, offset, limit int, appName, search string) ([]InstanceSummary, error)

	// GetMetricsTimeSeries returns time-series metrics for an app.
	GetMetricsTimeSeries(ctx context.Context, appName string, since time.Time) (MetricsTimeSeries, error)

	// Badge-specific queries

	// GetActiveInstancesCount returns the count of active instances for an app.
	// Active = last_seen_at within the last 30 days.
	GetActiveInstancesCount(ctx context.Context, appSlug string) (int, error)

	// GetMostUsedVersion returns the most commonly used version for an app.
	// Returns empty string if no instances found.
	GetMostUsedVersion(ctx context.Context, appSlug string) (string, error)

	// GetAggregatedMetric sums a specific metric across all active instances of an app.
	// Returns 0 if metric not found or no active instances.
	GetAggregatedMetric(ctx context.Context, appSlug, metricName string) (float64, error)

	// GetCombinedStats returns both an aggregated metric and instance count.
	// Used for the combined badge (e.g., "1.2k users / 42 inst").
	GetCombinedStats(ctx context.Context, appSlug, metricName string) (metricValue float64, instanceCount int, err error)
}
