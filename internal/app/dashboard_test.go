// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"testing"
	"time"

	"github.com/kolapsis/shm/internal/app/ports"
	"github.com/kolapsis/shm/internal/domain"
)

// mockDashboardReader is a test double for ports.DashboardReader.
type mockDashboardReader struct {
	stats      ports.DashboardStats
	instances  []ports.InstanceSummary
	timeSeries ports.MetricsTimeSeries
	statsErr   error
	listErr    error
	tsErr      error
	// Badge-specific fields
	instanceCount int
	version       string
	metricValue   float64
	combinedCount int
	badgeErr      error
}

func (m *mockDashboardReader) GetStats(ctx context.Context) (ports.DashboardStats, error) {
	if m.statsErr != nil {
		return ports.DashboardStats{}, m.statsErr
	}
	return m.stats, nil
}

func (m *mockDashboardReader) ListInstances(ctx context.Context, offset, limit int, appName, search string) ([]ports.InstanceSummary, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	// Apply offset and limit
	start := offset
	if start >= len(m.instances) {
		return []ports.InstanceSummary{}, nil
	}
	end := start + limit
	if limit <= 0 || end > len(m.instances) {
		end = len(m.instances)
	}
	return m.instances[start:end], nil
}

func (m *mockDashboardReader) GetMetricsTimeSeries(ctx context.Context, appName string, since time.Time) (ports.MetricsTimeSeries, error) {
	if m.tsErr != nil {
		return ports.MetricsTimeSeries{}, m.tsErr
	}
	return m.timeSeries, nil
}

func (m *mockDashboardReader) GetActiveInstancesCount(ctx context.Context, appSlug string) (int, error) {
	if m.badgeErr != nil {
		return 0, m.badgeErr
	}
	return m.instanceCount, nil
}

func (m *mockDashboardReader) GetMostUsedVersion(ctx context.Context, appSlug string) (string, error) {
	if m.badgeErr != nil {
		return "", m.badgeErr
	}
	return m.version, nil
}

func (m *mockDashboardReader) GetAggregatedMetric(ctx context.Context, appSlug, metricName string) (float64, error) {
	if m.badgeErr != nil {
		return 0, m.badgeErr
	}
	return m.metricValue, nil
}

func (m *mockDashboardReader) GetCombinedStats(ctx context.Context, appSlug, metricName string) (float64, int, error) {
	if m.badgeErr != nil {
		return 0, 0, m.badgeErr
	}
	return m.metricValue, m.combinedCount, nil
}

func TestDashboardService_GetStats(t *testing.T) {
	ctx := context.Background()

	t.Run("returns stats", func(t *testing.T) {
		reader := &mockDashboardReader{
			stats: ports.DashboardStats{
				TotalInstances:  100,
				ActiveInstances: 75,
				GlobalMetrics:   map[string]int64{"cpu": 500, "memory": 10240},
			},
		}
		svc := NewDashboardService(reader)

		stats, err := svc.GetStats(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if stats.TotalInstances != 100 {
			t.Errorf("expected 100 total instances, got %d", stats.TotalInstances)
		}
		if stats.ActiveInstances != 75 {
			t.Errorf("expected 75 active instances, got %d", stats.ActiveInstances)
		}
	})
}

func TestDashboardService_ListInstances(t *testing.T) {
	ctx := context.Background()

	t.Run("returns instances with default limit", func(t *testing.T) {
		id, _ := domain.NewInstanceID(validUUID)
		reader := &mockDashboardReader{
			instances: []ports.InstanceSummary{
				{
					ID:         id,
					AppName:    "myapp",
					AppVersion: "1.0.0",
					Status:     domain.StatusActive,
				},
			},
		}
		svc := NewDashboardService(reader)

		instances, err := svc.ListInstances(ctx, 0, 0, "", "") // offset=0, limit=0 (default), no filters
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(instances) != 1 {
			t.Errorf("expected 1 instance, got %d", len(instances))
		}
	})
}

func TestDashboardService_GetMetricsTimeSeries(t *testing.T) {
	ctx := context.Background()

	t.Run("returns time series", func(t *testing.T) {
		now := time.Now().UTC()
		reader := &mockDashboardReader{
			timeSeries: ports.MetricsTimeSeries{
				Timestamps: []time.Time{now.Add(-1 * time.Hour), now},
				Metrics:    map[string][]float64{"cpu": {0.3, 0.5}},
			},
		}
		svc := NewDashboardService(reader)

		ts, err := svc.GetMetricsTimeSeries(ctx, "myapp", Period24h)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ts.Timestamps) != 2 {
			t.Errorf("expected 2 timestamps, got %d", len(ts.Timestamps))
		}
	})

	t.Run("rejects empty app name", func(t *testing.T) {
		reader := &mockDashboardReader{}
		svc := NewDashboardService(reader)

		_, err := svc.GetMetricsTimeSeries(ctx, "", Period24h)
		if err == nil {
			t.Error("expected error for empty app name")
		}
	})
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		input    string
		expected Period
	}{
		{"24h", Period24h},
		{"7d", Period7d},
		{"30d", Period30d},
		{"3m", Period3m},
		{"1y", Period1y},
		{"all", PeriodAll},
		{"unknown", Period24h}, // default
		{"", Period24h},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParsePeriod(tt.input)
			if got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestPeriod_Duration(t *testing.T) {
	tests := []struct {
		period   Period
		expected time.Duration
	}{
		{Period24h, 24 * time.Hour},
		{Period7d, 7 * 24 * time.Hour},
		{Period30d, 30 * 24 * time.Hour},
		{Period3m, 90 * 24 * time.Hour},
		{Period1y, 365 * 24 * time.Hour},
		{PeriodAll, 10 * 365 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			got := tt.period.Duration()
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
