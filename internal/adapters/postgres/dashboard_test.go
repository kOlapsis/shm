// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kolapsis/shm/internal/domain"
)

func TestDashboardReader_GetStats(t *testing.T) {
	ctx := context.Background()

	t.Run("returns stats with metrics", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		reader := NewDashboardReader(db)

		// Mock counts query
		countRows := sqlmock.NewRows([]string{"total", "active"}).
			AddRow(100, 75)
		mock.ExpectQuery("SELECT.+COUNT").WillReturnRows(countRows)

		// Mock per-app counts query
		perAppRows := sqlmock.NewRows([]string{"app_name", "count"}).
			AddRow("myapp", 60).
			AddRow("otherapp", 40)
		mock.ExpectQuery("SELECT app_name, COUNT").WillReturnRows(perAppRows)

		// Mock metrics query
		metricsRows := sqlmock.NewRows([]string{"data"}).
			AddRow(`{"cpu": 50, "memory": 1024}`).
			AddRow(`{"cpu": 30, "memory": 512}`)
		mock.ExpectQuery("SELECT data FROM").WillReturnRows(metricsRows)

		stats, err := reader.GetStats(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if stats.TotalInstances != 100 {
			t.Errorf("expected 100 total, got %d", stats.TotalInstances)
		}
		if stats.ActiveInstances != 75 {
			t.Errorf("expected 75 active, got %d", stats.ActiveInstances)
		}
		if stats.GlobalMetrics["cpu"] != 80 {
			t.Errorf("expected cpu=80, got %d", stats.GlobalMetrics["cpu"])
		}
		if stats.GlobalMetrics["memory"] != 1536 {
			t.Errorf("expected memory=1536, got %d", stats.GlobalMetrics["memory"])
		}
	})
}

func TestDashboardReader_ListInstances(t *testing.T) {
	ctx := context.Background()

	t.Run("returns instances with metrics", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		reader := NewDashboardReader(db)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{
			"instance_id", "app_name", "app_version", "environment",
			"status", "last_seen_at", "deployment_mode", "data", "app_slug",
		}).
			AddRow(testUUID, "myapp", "1.0", "prod", "active", now, "docker", `{"cpu": 0.5}`, "myapp")

		mock.ExpectQuery("SELECT.+FROM instances").
			WithArgs(50, 0).
			WillReturnRows(rows)

		list, err := reader.ListInstances(ctx, 0, 50, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(list) != 1 {
			t.Fatalf("expected 1 instance, got %d", len(list))
		}

		inst := list[0]
		if inst.AppName != "myapp" {
			t.Errorf("expected app_name=myapp, got %s", inst.AppName)
		}
		if inst.Status != domain.StatusActive {
			t.Errorf("expected status=active, got %s", inst.Status)
		}

		cpu, ok := inst.Metrics.GetFloat64("cpu")
		if !ok || cpu != 0.5 {
			t.Errorf("expected cpu=0.5, got %v", cpu)
		}
	})
}

func TestDashboardReader_GetMetricsTimeSeries(t *testing.T) {
	ctx := context.Background()

	t.Run("returns time series", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		reader := NewDashboardReader(db)
		now := time.Now().UTC()
		since := now.Add(-24 * time.Hour)

		rows := sqlmock.NewRows([]string{"snapshot_at", "data"}).
			AddRow(now.Add(-1*time.Hour), `{"cpu": 0.3}`).
			AddRow(now, `{"cpu": 0.5}`)

		mock.ExpectQuery("SELECT.+FROM snapshots").
			WithArgs("myapp", since).
			WillReturnRows(rows)

		ts, err := reader.GetMetricsTimeSeries(ctx, "myapp", since)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ts.Timestamps) != 2 {
			t.Errorf("expected 2 timestamps, got %d", len(ts.Timestamps))
		}

		cpuData, ok := ts.Metrics["cpu"]
		if !ok {
			t.Fatal("expected cpu metric")
		}
		if len(cpuData) != 2 {
			t.Errorf("expected 2 data points, got %d", len(cpuData))
		}
	})
}
