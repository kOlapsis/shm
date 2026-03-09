// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kolapsis/shm/internal/app/ports"
	"github.com/kolapsis/shm/internal/domain"
)

// SaveSnapshotInput holds the data needed to save a snapshot.
type SaveSnapshotInput struct {
	InstanceID string
	Timestamp  time.Time
	Metrics    json.RawMessage
}

// SnapshotService handles snapshot-related use cases.
type SnapshotService struct {
	snapshotRepo ports.SnapshotRepository
	instanceRepo ports.InstanceRepository
}

// NewSnapshotService creates a new SnapshotService.
func NewSnapshotService(snapshotRepo ports.SnapshotRepository, instanceRepo ports.InstanceRepository) *SnapshotService {
	return &SnapshotService{
		snapshotRepo: snapshotRepo,
		instanceRepo: instanceRepo,
	}
}

// Save validates and persists a snapshot from an instance.
// The instance must exist and not be revoked (verified by signature middleware).
func (s *SnapshotService) Save(ctx context.Context, input SaveSnapshotInput) error {
	// Create and validate the domain entity
	snapshot, err := domain.NewSnapshot(input.InstanceID, input.Timestamp, input.Metrics)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	// Verify instance exists and is not revoked
	_, err = s.instanceRepo.GetPublicKey(ctx, snapshot.InstanceID)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	// Persist the snapshot
	if err := s.snapshotRepo.Save(ctx, snapshot); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return nil
}

// GetLatest retrieves the most recent snapshot for an instance.
func (s *SnapshotService) GetLatest(ctx context.Context, instanceID string) (*domain.Snapshot, error) {
	id, err := domain.NewInstanceID(instanceID)
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}

	snapshot, err := s.snapshotRepo.GetLatestByInstanceID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}

	return snapshot, nil
}

// GetHistory retrieves recent snapshots for an instance.
func (s *SnapshotService) GetHistory(ctx context.Context, instanceID string, limit int) ([]*domain.Snapshot, error) {
	id, err := domain.NewInstanceID(instanceID)
	if err != nil {
		return nil, fmt.Errorf("get snapshot history: %w", err)
	}

	if limit <= 0 {
		limit = 100
	}

	snapshots, err := s.snapshotRepo.FindByInstanceID(ctx, id, limit)
	if err != nil {
		return nil, fmt.Errorf("get snapshot history: %w", err)
	}

	return snapshots, nil
}
