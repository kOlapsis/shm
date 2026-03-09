// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kolapsis/shm/internal/domain"
)

// InstanceRepository implements ports.InstanceRepository for PostgreSQL.
type InstanceRepository struct {
	db *sql.DB
}

// NewInstanceRepository creates a new InstanceRepository.
func NewInstanceRepository(db *sql.DB) *InstanceRepository {
	return &InstanceRepository{db: db}
}

// Save persists an instance (insert or update).
func (r *InstanceRepository) Save(ctx context.Context, instance *domain.Instance) error {
	query := `
		INSERT INTO instances (instance_id, public_key, application_id, app_name, app_version, deployment_mode, environment, os_arch, status, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (instance_id) DO UPDATE
		SET application_id = EXCLUDED.application_id,
			app_name = EXCLUDED.app_name,
			app_version = EXCLUDED.app_version,
			deployment_mode = EXCLUDED.deployment_mode,
			environment = EXCLUDED.environment,
			os_arch = EXCLUDED.os_arch,
			status = EXCLUDED.status,
			last_seen_at = EXCLUDED.last_seen_at
	`

	var applicationID *string
	if instance.ApplicationID != "" {
		appID := instance.ApplicationID.String()
		applicationID = &appID
	}

	_, err := r.db.ExecContext(ctx, query,
		instance.ID.String(),
		instance.PublicKey.String(),
		applicationID,
		instance.AppName,
		instance.AppVersion,
		instance.DeploymentMode,
		instance.Environment,
		instance.OSArch,
		string(instance.Status),
		instance.LastSeenAt,
	)
	if err != nil {
		return fmt.Errorf("save instance %s: %w", instance.ID, err)
	}
	return nil
}

// FindByID retrieves an instance by its ID.
func (r *InstanceRepository) FindByID(ctx context.Context, id domain.InstanceID) (*domain.Instance, error) {
	query := `
		SELECT instance_id, public_key, application_id, app_name, app_version, deployment_mode, environment, os_arch, status, last_seen_at, created_at
		FROM instances
		WHERE instance_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id.String())

	var inst domain.Instance
	var instanceID, publicKey, status string
	var applicationID sql.NullString

	err := row.Scan(
		&instanceID,
		&publicKey,
		&applicationID,
		&inst.AppName,
		&inst.AppVersion,
		&inst.DeploymentMode,
		&inst.Environment,
		&inst.OSArch,
		&status,
		&inst.LastSeenAt,
		&inst.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find instance %s: %w", id, err)
	}

	// Reconstruct value objects (already validated in DB)
	inst.ID = domain.InstanceID(instanceID)
	inst.PublicKey = domain.PublicKey(publicKey)
	inst.Status = domain.InstanceStatus(status)

	if applicationID.Valid {
		inst.ApplicationID = domain.ApplicationID(applicationID.String)
	}

	return &inst, nil
}

// GetPublicKey retrieves the public key for an instance.
func (r *InstanceRepository) GetPublicKey(ctx context.Context, id domain.InstanceID) (domain.PublicKey, error) {
	query := `SELECT public_key, status FROM instances WHERE instance_id = $1`
	row := r.db.QueryRowContext(ctx, query, id.String())

	var publicKey, status string
	err := row.Scan(&publicKey, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrInstanceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get public key for %s: %w", id, err)
	}

	if domain.InstanceStatus(status) == domain.StatusRevoked {
		return "", domain.ErrInstanceRevoked
	}

	return domain.PublicKey(publicKey), nil
}

// UpdateStatus updates the status and last_seen_at timestamp.
func (r *InstanceRepository) UpdateStatus(ctx context.Context, id domain.InstanceID, status domain.InstanceStatus) error {
	query := `UPDATE instances SET status = $1, last_seen_at = NOW() WHERE instance_id = $2`
	result, err := r.db.ExecContext(ctx, query, string(status), id.String())
	if err != nil {
		return fmt.Errorf("update status for %s: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrInstanceNotFound
	}

	return nil
}

// Delete removes an instance and all its associated snapshots.
func (r *InstanceRepository) Delete(ctx context.Context, id domain.InstanceID) error {
	// Snapshots are deleted via ON DELETE CASCADE constraint
	query := `DELETE FROM instances WHERE instance_id = $1`
	result, err := r.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("delete instance %s: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrInstanceNotFound
	}

	return nil
}
