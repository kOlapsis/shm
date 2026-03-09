// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kolapsis/shm/internal/app/ports"
)

// DashboardService handles dashboard-related use cases.
// This is a read-only service (CQRS-lite pattern).
type DashboardService struct {
	reader ports.DashboardReader
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(reader ports.DashboardReader) *DashboardService {
	return &DashboardService{reader: reader}
}

// GetStats returns aggregated dashboard statistics.
func (s *DashboardService) GetStats(ctx context.Context) (ports.DashboardStats, error) {
	stats, err := s.reader.GetStats(ctx)
	if err != nil {
		return ports.DashboardStats{}, fmt.Errorf("get dashboard stats: %w", err)
	}
	return stats, nil
}

// ListInstances returns instances with their latest metrics.
// offset and limit are used for pagination.
// appName filters by app name (empty = all apps).
// search filters by instance_id, version, environment, or deployment_mode.
func (s *DashboardService) ListInstances(ctx context.Context, offset, limit int, appName, search string) ([]ports.InstanceSummary, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}

	instances, err := s.reader.ListInstances(ctx, offset, limit, appName, search)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}

	return instances, nil
}

// Period represents a time period for metrics queries.
type Period string

const (
	Period24h Period = "24h"
	Period7d  Period = "7d"
	Period30d Period = "30d"
	Period3m  Period = "3m"
	Period1y  Period = "1y"
	PeriodAll Period = "all"
)

// Duration returns the time.Duration for the period.
func (p Period) Duration() time.Duration {
	switch p {
	case Period7d:
		return 7 * 24 * time.Hour
	case Period30d:
		return 30 * 24 * time.Hour
	case Period3m:
		return 90 * 24 * time.Hour
	case Period1y:
		return 365 * 24 * time.Hour
	case PeriodAll:
		return 10 * 365 * 24 * time.Hour // 10 years
	default:
		return 24 * time.Hour
	}
}

// ParsePeriod parses a period string.
func ParsePeriod(s string) Period {
	switch s {
	case "7d":
		return Period7d
	case "30d":
		return Period30d
	case "3m":
		return Period3m
	case "1y":
		return Period1y
	case "all":
		return PeriodAll
	default:
		return Period24h
	}
}

// GetMetricsTimeSeries returns time-series metrics for an app.
func (s *DashboardService) GetMetricsTimeSeries(ctx context.Context, appName string, period Period) (ports.MetricsTimeSeries, error) {
	if appName == "" {
		return ports.MetricsTimeSeries{}, fmt.Errorf("get metrics time series: app name is required")
	}

	since := time.Now().UTC().Add(-period.Duration())

	data, err := s.reader.GetMetricsTimeSeries(ctx, appName, since)
	if err != nil {
		return ports.MetricsTimeSeries{}, fmt.Errorf("get metrics time series: %w", err)
	}

	return data, nil
}

// Badge-specific methods

// GetActiveInstancesCount returns the count of active instances for an app.
func (s *DashboardService) GetActiveInstancesCount(ctx context.Context, appSlug string) (int, error) {
	count, err := s.reader.GetActiveInstancesCount(ctx, appSlug)
	if err != nil {
		return 0, fmt.Errorf("get active instances count: %w", err)
	}
	return count, nil
}

// GetMostUsedVersion returns the most commonly used version for an app.
func (s *DashboardService) GetMostUsedVersion(ctx context.Context, appSlug string) (string, error) {
	version, err := s.reader.GetMostUsedVersion(ctx, appSlug)
	if err != nil {
		return "", fmt.Errorf("get most used version: %w", err)
	}
	return version, nil
}

// GetAggregatedMetric sums a specific metric across all active instances of an app.
func (s *DashboardService) GetAggregatedMetric(ctx context.Context, appSlug, metricName string) (float64, error) {
	value, err := s.reader.GetAggregatedMetric(ctx, appSlug, metricName)
	if err != nil {
		return 0, fmt.Errorf("get aggregated metric: %w", err)
	}
	return value, nil
}

// GetCombinedStats returns both an aggregated metric and instance count.
func (s *DashboardService) GetCombinedStats(ctx context.Context, appSlug, metricName string) (float64, int, error) {
	metricValue, instanceCount, err := s.reader.GetCombinedStats(ctx, appSlug, metricName)
	if err != nil {
		return 0, 0, fmt.Errorf("get combined stats: %w", err)
	}
	return metricValue, instanceCount, nil
}
