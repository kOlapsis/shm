// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kolapsis/shm/internal/app/ports"
	"github.com/kolapsis/shm/internal/domain"
)

// DashboardReader implements ports.DashboardReader for PostgreSQL.
type DashboardReader struct {
	db *sql.DB
}

// NewDashboardReader creates a new DashboardReader.
func NewDashboardReader(db *sql.DB) *DashboardReader {
	return &DashboardReader{db: db}
}

// GetStats returns aggregated dashboard statistics.
func (r *DashboardReader) GetStats(ctx context.Context) (ports.DashboardStats, error) {
	var stats ports.DashboardStats
	stats.GlobalMetrics = make(map[string]int64)
	stats.PerAppCounts = make(map[string]int)

	// Get instance counts
	countsQuery := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE last_seen_at > NOW() - INTERVAL '30 days')
		FROM instances
	`
	if err := r.db.QueryRowContext(ctx, countsQuery).Scan(&stats.TotalInstances, &stats.ActiveInstances); err != nil {
		return stats, fmt.Errorf("get instance counts: %w", err)
	}

	// Get per-app instance counts
	perAppQuery := `
		SELECT app_name, COUNT(*) as count
		FROM instances
		WHERE app_name IS NOT NULL AND app_name != ''
		GROUP BY app_name
	`
	appRows, err := r.db.QueryContext(ctx, perAppQuery)
	if err != nil {
		return stats, fmt.Errorf("get per-app counts: %w", err)
	}
	defer appRows.Close()

	for appRows.Next() {
		var appName string
		var count int
		if err := appRows.Scan(&appName, &count); err != nil {
			continue
		}
		stats.PerAppCounts[appName] = count
	}

	// Get aggregated metrics from latest snapshots
	metricsQuery := `
		SELECT data
		FROM (
			SELECT DISTINCT ON (instance_id) data
			FROM snapshots
			ORDER BY instance_id, snapshot_at DESC
		) as latest
	`
	rows, err := r.db.QueryContext(ctx, metricsQuery)
	if err != nil {
		return stats, fmt.Errorf("get latest metrics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rawJSON []byte
		if err := rows.Scan(&rawJSON); err != nil {
			continue
		}

		var metrics map[string]any
		if err := json.Unmarshal(rawJSON, &metrics); err != nil {
			continue
		}

		for key, val := range metrics {
			switch v := val.(type) {
			case float64:
				stats.GlobalMetrics[key] += int64(v)
			case int:
				stats.GlobalMetrics[key] += int64(v)
			}
		}
	}

	return stats, nil
}

// ListInstances returns instances with their latest metrics.
// offset and limit are used for pagination.
// appName filters by app name (empty = all apps).
// search filters by instance_id, version, environment, or deployment_mode.
func (r *DashboardReader) ListInstances(ctx context.Context, offset, limit int, appName, search string) ([]ports.InstanceSummary, error) {
	// Build dynamic query with optional filters
	query := `
		SELECT
			i.instance_id, i.app_name, i.app_version, i.environment, i.status, i.last_seen_at, i.deployment_mode,
			COALESCE(s.data, '{}'::jsonb),
			a.app_slug
		FROM instances i
		LEFT JOIN applications a ON i.application_id = a.id
		LEFT JOIN LATERAL (
			SELECT data FROM snapshots
			WHERE instance_id = i.instance_id
			ORDER BY snapshot_at DESC
			LIMIT 1
		) s ON true
		WHERE 1=1
	`

	args := []any{}
	argIdx := 1

	// Filter by app name
	if appName != "" {
		query += fmt.Sprintf(" AND i.app_name = $%d", argIdx)
		args = append(args, appName)
		argIdx++
	}

	// Filter by search term (case-insensitive)
	if search != "" {
		searchPattern := "%" + search + "%"
		query += fmt.Sprintf(` AND (
			i.instance_id::text ILIKE $%d OR
			i.app_version ILIKE $%d OR
			i.environment ILIKE $%d OR
			i.deployment_mode ILIKE $%d
		)`, argIdx, argIdx, argIdx, argIdx)
		args = append(args, searchPattern)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY i.last_seen_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	var list []ports.InstanceSummary
	for rows.Next() {
		var instanceID, status string
		var summary ports.InstanceSummary
		var rawMetrics []byte
		var appSlug sql.NullString

		err := rows.Scan(
			&instanceID,
			&summary.AppName,
			&summary.AppVersion,
			&summary.Environment,
			&status,
			&summary.LastSeenAt,
			&summary.DeploymentMode,
			&rawMetrics,
			&appSlug,
		)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}

		summary.ID = domain.InstanceID(instanceID)
		summary.Status = domain.InstanceStatus(status)
		_ = json.Unmarshal(rawMetrics, &summary.Metrics)

		if appSlug.Valid {
			summary.AppSlug = appSlug.String
		}

		list = append(list, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}

	return list, nil
}

// GetMetricsTimeSeries returns time-series metrics for an app.
func (r *DashboardReader) GetMetricsTimeSeries(ctx context.Context, appName string, since time.Time) (ports.MetricsTimeSeries, error) {
	query := `
		SELECT s.snapshot_at, s.data
		FROM snapshots s
		JOIN instances i ON s.instance_id = i.instance_id
		WHERE i.app_name = $1
		  AND s.snapshot_at > $2
		ORDER BY s.snapshot_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, appName, since)
	if err != nil {
		return ports.MetricsTimeSeries{}, fmt.Errorf("get metrics time series: %w", err)
	}
	defer rows.Close()

	// Aggregate metrics by timestamp
	timestampMap := make(map[time.Time]map[string]float64)
	var timestamps []time.Time

	for rows.Next() {
		var snapshotAt time.Time
		var rawMetrics []byte

		if err := rows.Scan(&snapshotAt, &rawMetrics); err != nil {
			continue
		}

		var metrics map[string]any
		if err := json.Unmarshal(rawMetrics, &metrics); err != nil {
			continue
		}

		if _, exists := timestampMap[snapshotAt]; !exists {
			timestampMap[snapshotAt] = make(map[string]float64)
			timestamps = append(timestamps, snapshotAt)
		}

		for key, val := range metrics {
			if v, ok := val.(float64); ok {
				timestampMap[snapshotAt][key] += v
			}
		}
	}

	// Build result
	result := ports.MetricsTimeSeries{
		Timestamps: timestamps,
		Metrics:    make(map[string][]float64),
	}

	for _, ts := range timestamps {
		for metricKey, value := range timestampMap[ts] {
			result.Metrics[metricKey] = append(result.Metrics[metricKey], value)
		}
	}

	return result, nil
}

// GetActiveInstancesCount returns the count of active instances for an app.
func (r *DashboardReader) GetActiveInstancesCount(ctx context.Context, appSlug string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM instances i
		JOIN applications a ON i.application_id = a.id
		WHERE a.app_slug = $1
		  AND i.last_seen_at > NOW() - INTERVAL '30 days'
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, appSlug).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get active instances count: %w", err)
	}

	return count, nil
}

// GetMostUsedVersion returns the most commonly used version for an app.
func (r *DashboardReader) GetMostUsedVersion(ctx context.Context, appSlug string) (string, error) {
	query := `
		SELECT i.app_version
		FROM instances i
		JOIN applications a ON i.application_id = a.id
		WHERE a.app_slug = $1
		  AND i.last_seen_at > NOW() - INTERVAL '30 days'
		GROUP BY i.app_version
		ORDER BY COUNT(*) DESC
		LIMIT 1
	`

	var version string
	err := r.db.QueryRowContext(ctx, query, appSlug).Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil // No instances found
	}
	if err != nil {
		return "", fmt.Errorf("get most used version: %w", err)
	}

	return version, nil
}

// GetAggregatedMetric sums a specific metric across all active instances of an app.
func (r *DashboardReader) GetAggregatedMetric(ctx context.Context, appSlug, metricName string) (float64, error) {
	query := `
		SELECT COALESCE(SUM((latest.data->>$2)::numeric), 0)
		FROM (
			SELECT DISTINCT ON (i.instance_id) i.instance_id, s.data
			FROM instances i
			JOIN applications a ON i.application_id = a.id
			LEFT JOIN LATERAL (
				SELECT data
				FROM snapshots
				WHERE instance_id = i.instance_id
				  AND jsonb_exists(data, $2)
				ORDER BY snapshot_at DESC
				LIMIT 1
			) s ON true
			WHERE a.app_slug = $1
			  AND i.last_seen_at > NOW() - INTERVAL '30 days'
			  AND s.data IS NOT NULL
		) latest
	`

	var total float64
	err := r.db.QueryRowContext(ctx, query, appSlug, metricName).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("get aggregated metric: %w", err)
	}

	return total, nil
}

// GetCombinedStats returns both an aggregated metric and instance count.
func (r *DashboardReader) GetCombinedStats(ctx context.Context, appSlug, metricName string) (float64, int, error) {
	query := `
		WITH latest_metrics AS (
			SELECT DISTINCT ON (i.instance_id)
				i.instance_id,
				(s.data->>$2)::numeric as metric_value
			FROM instances i
			JOIN applications a ON i.application_id = a.id
			LEFT JOIN LATERAL (
				SELECT data
				FROM snapshots
				WHERE instance_id = i.instance_id
				  AND jsonb_exists(data, $2)
				ORDER BY snapshot_at DESC
				LIMIT 1
			) s ON true
			WHERE a.app_slug = $1
			  AND i.last_seen_at > NOW() - INTERVAL '30 days'
			  AND s.data IS NOT NULL
		)
		SELECT
			COALESCE(SUM(metric_value), 0) as total,
			COUNT(*) as instances
		FROM latest_metrics
	`

	var metricValue float64
	var instanceCount int

	err := r.db.QueryRowContext(ctx, query, appSlug, metricName).Scan(&metricValue, &instanceCount)
	if err != nil {
		return 0, 0, fmt.Errorf("get combined stats: %w", err)
	}

	return metricValue, instanceCount, nil
}
