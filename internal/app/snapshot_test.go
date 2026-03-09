// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kolapsis/shm/internal/domain"
)

// mockSnapshotRepo is a test double for ports.SnapshotRepository.
type mockSnapshotRepo struct {
	snapshots map[string][]*domain.Snapshot
	saveErr   error
}

func newMockSnapshotRepo() *mockSnapshotRepo {
	return &mockSnapshotRepo{
		snapshots: make(map[string][]*domain.Snapshot),
	}
}

func (m *mockSnapshotRepo) Save(ctx context.Context, snapshot *domain.Snapshot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	id := snapshot.InstanceID.String()
	m.snapshots[id] = append(m.snapshots[id], snapshot)
	return nil
}

func (m *mockSnapshotRepo) FindByInstanceID(ctx context.Context, id domain.InstanceID, limit int) ([]*domain.Snapshot, error) {
	snaps := m.snapshots[id.String()]
	if limit > 0 && len(snaps) > limit {
		return snaps[:limit], nil
	}
	return snaps, nil
}

func (m *mockSnapshotRepo) GetLatestByInstanceID(ctx context.Context, id domain.InstanceID) (*domain.Snapshot, error) {
	snaps := m.snapshots[id.String()]
	if len(snaps) == 0 {
		return nil, errors.New("no snapshots found")
	}
	return snaps[len(snaps)-1], nil
}

func TestSnapshotService_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("saves valid snapshot", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		// Create active instance
		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		_ = inst.Activate()
		instanceRepo.instances[validUUID] = inst

		err := svc.Save(ctx, SaveSnapshotInput{
			InstanceID: validUUID,
			Timestamp:  time.Now().UTC(),
			Metrics:    json.RawMessage(`{"cpu": 0.5, "memory": 1024}`),
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(snapshotRepo.snapshots[validUUID]) != 1 {
			t.Error("snapshot not saved")
		}
	})

	t.Run("rejects invalid instance ID", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		err := svc.Save(ctx, SaveSnapshotInput{
			InstanceID: "invalid",
			Timestamp:  time.Now().UTC(),
			Metrics:    json.RawMessage(`{}`),
		})

		if !errors.Is(err, domain.ErrInvalidInstanceID) {
			t.Errorf("expected ErrInvalidInstanceID, got %v", err)
		}
	})

	t.Run("rejects non-existent instance", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		err := svc.Save(ctx, SaveSnapshotInput{
			InstanceID: validUUID,
			Timestamp:  time.Now().UTC(),
			Metrics:    json.RawMessage(`{}`),
		})

		if !errors.Is(err, domain.ErrInstanceNotFound) {
			t.Errorf("expected ErrInstanceNotFound, got %v", err)
		}
	})

	t.Run("rejects revoked instance", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		// Create revoked instance
		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		_ = inst.Revoke()
		instanceRepo.instances[validUUID] = inst

		err := svc.Save(ctx, SaveSnapshotInput{
			InstanceID: validUUID,
			Timestamp:  time.Now().UTC(),
			Metrics:    json.RawMessage(`{}`),
		})

		if !errors.Is(err, domain.ErrInstanceRevoked) {
			t.Errorf("expected ErrInstanceRevoked, got %v", err)
		}
	})

	t.Run("rejects future timestamp", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		instanceRepo.instances[validUUID] = inst

		err := svc.Save(ctx, SaveSnapshotInput{
			InstanceID: validUUID,
			Timestamp:  time.Now().UTC().Add(1 * time.Hour),
			Metrics:    json.RawMessage(`{}`),
		})

		if !errors.Is(err, domain.ErrInvalidSnapshot) {
			t.Errorf("expected ErrInvalidSnapshot, got %v", err)
		}
	})

	t.Run("rejects invalid JSON metrics", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		instanceRepo.instances[validUUID] = inst

		err := svc.Save(ctx, SaveSnapshotInput{
			InstanceID: validUUID,
			Timestamp:  time.Now().UTC(),
			Metrics:    json.RawMessage(`{invalid`),
		})

		if !errors.Is(err, domain.ErrInvalidMetrics) {
			t.Errorf("expected ErrInvalidMetrics, got %v", err)
		}
	})
}

func TestSnapshotService_GetLatest(t *testing.T) {
	ctx := context.Background()

	t.Run("returns latest snapshot", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		snapshotRepo := newMockSnapshotRepo()
		svc := NewSnapshotService(snapshotRepo, instanceRepo)

		// Create instance
		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		instanceRepo.instances[validUUID] = inst

		// Save two snapshots
		snap1, _ := domain.NewSnapshot(validUUID, time.Now().UTC().Add(-1*time.Hour), json.RawMessage(`{"cpu": 0.3}`))
		snap2, _ := domain.NewSnapshot(validUUID, time.Now().UTC(), json.RawMessage(`{"cpu": 0.5}`))
		snapshotRepo.snapshots[validUUID] = []*domain.Snapshot{snap1, snap2}

		latest, err := svc.GetLatest(ctx, validUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cpu, _ := latest.Metrics.GetFloat64("cpu")
		if cpu != 0.5 {
			t.Errorf("expected latest snapshot with cpu=0.5, got %v", cpu)
		}
	})
}
