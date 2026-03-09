// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kolapsis/shm/internal/domain"
)

// SnapshotRepository implements ports.SnapshotRepository for PostgreSQL.
type SnapshotRepository struct {
	db *sql.DB
}

// NewSnapshotRepository creates a new SnapshotRepository.
func NewSnapshotRepository(db *sql.DB) *SnapshotRepository {
	return &SnapshotRepository{db: db}
}

// Save persists a snapshot and updates the instance heartbeat.
func (r *SnapshotRepository) Save(ctx context.Context, snapshot *domain.Snapshot) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Serialize metrics to JSON
	metricsJSON, err := json.Marshal(snapshot.Metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	// Insert snapshot
	insertQuery := `INSERT INTO snapshots (instance_id, snapshot_at, data) VALUES ($1, $2, $3)`
	_, err = tx.ExecContext(ctx, insertQuery, snapshot.InstanceID.String(), snapshot.SnapshotAt, metricsJSON)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	// Update instance heartbeat
	updateQuery := `UPDATE instances SET last_seen_at = NOW() WHERE instance_id = $1`
	_, err = tx.ExecContext(ctx, updateQuery, snapshot.InstanceID.String())
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// FindByInstanceID retrieves snapshots for an instance.
func (r *SnapshotRepository) FindByInstanceID(ctx context.Context, id domain.InstanceID, limit int) ([]*domain.Snapshot, error) {
	query := `
		SELECT id, instance_id, snapshot_at, data
		FROM snapshots
		WHERE instance_id = $1
		ORDER BY snapshot_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, id.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("find snapshots for %s: %w", id, err)
	}
	defer rows.Close()

	var snapshots []*domain.Snapshot
	for rows.Next() {
		snap, err := r.scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots: %w", err)
	}

	return snapshots, nil
}

// GetLatestByInstanceID retrieves the most recent snapshot for an instance.
func (r *SnapshotRepository) GetLatestByInstanceID(ctx context.Context, id domain.InstanceID) (*domain.Snapshot, error) {
	query := `
		SELECT id, instance_id, snapshot_at, data
		FROM snapshots
		WHERE instance_id = $1
		ORDER BY snapshot_at DESC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, id.String())

	var snap domain.Snapshot
	var instanceID string
	var rawMetrics []byte

	err := row.Scan(&snap.ID, &instanceID, &snap.SnapshotAt, &rawMetrics)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no snapshots found for %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot for %s: %w", id, err)
	}

	snap.InstanceID = domain.InstanceID(instanceID)
	if err := json.Unmarshal(rawMetrics, &snap.Metrics); err != nil {
		return nil, fmt.Errorf("unmarshal metrics: %w", err)
	}

	return &snap, nil
}

// scanSnapshot scans a snapshot row into a domain.Snapshot.
func (r *SnapshotRepository) scanSnapshot(rows *sql.Rows) (*domain.Snapshot, error) {
	var snap domain.Snapshot
	var instanceID string
	var rawMetrics []byte

	err := rows.Scan(&snap.ID, &instanceID, &snap.SnapshotAt, &rawMetrics)
	if err != nil {
		return nil, fmt.Errorf("scan snapshot: %w", err)
	}

	snap.InstanceID = domain.InstanceID(instanceID)
	if err := json.Unmarshal(rawMetrics, &snap.Metrics); err != nil {
		return nil, fmt.Errorf("unmarshal metrics: %w", err)
	}

	return &snap, nil
}
