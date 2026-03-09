// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kolapsis/shm/internal/domain"
)

// mockInstanceRepo is a test double for ports.InstanceRepository.
type mockInstanceRepo struct {
	instances map[string]*domain.Instance
	saveErr   error
	findErr   error
}

func newMockInstanceRepo() *mockInstanceRepo {
	return &mockInstanceRepo{
		instances: make(map[string]*domain.Instance),
	}
}

// Helper to create a test ApplicationService (reuses mocks from application_test.go)
func newTestApplicationService() *ApplicationService {
	return NewApplicationService(newMockApplicationRepository(), &mockGitHubService{}, nil)
}

func (m *mockInstanceRepo) Save(ctx context.Context, instance *domain.Instance) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.instances[instance.ID.String()] = instance
	return nil
}

func (m *mockInstanceRepo) FindByID(ctx context.Context, id domain.InstanceID) (*domain.Instance, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	inst, ok := m.instances[id.String()]
	if !ok {
		return nil, domain.ErrInstanceNotFound
	}
	return inst, nil
}

func (m *mockInstanceRepo) GetPublicKey(ctx context.Context, id domain.InstanceID) (domain.PublicKey, error) {
	inst, ok := m.instances[id.String()]
	if !ok {
		return "", domain.ErrInstanceNotFound
	}
	if inst.IsRevoked() {
		return "", domain.ErrInstanceRevoked
	}
	return inst.PublicKey, nil
}

func (m *mockInstanceRepo) UpdateStatus(ctx context.Context, id domain.InstanceID, status domain.InstanceStatus) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	inst, ok := m.instances[id.String()]
	if !ok {
		return domain.ErrInstanceNotFound
	}
	inst.Status = status
	inst.LastSeenAt = time.Now().UTC()
	return nil
}

func (m *mockInstanceRepo) Delete(ctx context.Context, id domain.InstanceID) error {
	if _, ok := m.instances[id.String()]; !ok {
		return domain.ErrInstanceNotFound
	}
	delete(m.instances, id.String())
	return nil
}

const (
	validUUID = "550e8400-e29b-41d4-a716-446655440000"
	validKey  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestInstanceService_Register(t *testing.T) {
	ctx := context.Background()

	t.Run("registers new instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		err := svc.Register(ctx, RegisterInstanceInput{
			InstanceID:     validUUID,
			PublicKey:      validKey,
			AppName:        "myapp",
			AppVersion:     "1.0.0",
			DeploymentMode: "docker",
			Environment:    "prod",
			OSArch:         "linux/amd64",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		inst, ok := repo.instances[validUUID]
		if !ok {
			t.Fatal("instance not saved")
		}
		if inst.Status != domain.StatusPending {
			t.Errorf("expected status %s, got %s", domain.StatusPending, inst.Status)
		}
	})

	t.Run("updates existing instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		// Register first time
		_ = svc.Register(ctx, RegisterInstanceInput{
			InstanceID: validUUID,
			PublicKey:  validKey,
			AppName:    "myapp",
			AppVersion: "1.0.0",
		})

		// Activate
		repo.instances[validUUID].Status = domain.StatusActive

		// Register again with new version
		err := svc.Register(ctx, RegisterInstanceInput{
			InstanceID: validUUID,
			PublicKey:  validKey,
			AppName:    "myapp",
			AppVersion: "2.0.0",
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		inst := repo.instances[validUUID]
		if inst.Status != domain.StatusActive {
			t.Error("status should be preserved")
		}
		if inst.AppVersion != "2.0.0" {
			t.Errorf("version should be updated, got %s", inst.AppVersion)
		}
	})

	t.Run("rejects invalid instance ID", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		err := svc.Register(ctx, RegisterInstanceInput{
			InstanceID: "invalid",
			PublicKey:  validKey,
			AppName:    "myapp",
		})

		if !errors.Is(err, domain.ErrInvalidInstanceID) {
			t.Errorf("expected ErrInvalidInstanceID, got %v", err)
		}
	})

	t.Run("rejects invalid public key", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		err := svc.Register(ctx, RegisterInstanceInput{
			InstanceID: validUUID,
			PublicKey:  "invalid",
			AppName:    "myapp",
		})

		if !errors.Is(err, domain.ErrInvalidPublicKey) {
			t.Errorf("expected ErrInvalidPublicKey, got %v", err)
		}
	})
}

func TestInstanceService_Activate(t *testing.T) {
	ctx := context.Background()

	t.Run("activates pending instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		// Create pending instance
		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		repo.instances[validUUID] = inst

		err := svc.Activate(ctx, validUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if repo.instances[validUUID].Status != domain.StatusActive {
			t.Error("instance should be active")
		}
	})

	t.Run("fails for non-existent instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		err := svc.Activate(ctx, validUUID)

		if !errors.Is(err, domain.ErrInstanceNotFound) {
			t.Errorf("expected ErrInstanceNotFound, got %v", err)
		}
	})

	t.Run("fails for already active instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		// Create active instance
		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		_ = inst.Activate()
		repo.instances[validUUID] = inst

		err := svc.Activate(ctx, validUUID)

		if !errors.Is(err, domain.ErrInvalidStatusTransition) {
			t.Errorf("expected ErrInvalidStatusTransition, got %v", err)
		}
	})
}

func TestInstanceService_GetPublicKey(t *testing.T) {
	ctx := context.Background()

	t.Run("returns public key", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		repo.instances[validUUID] = inst

		pk, err := svc.GetPublicKey(ctx, validUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pk != validKey {
			t.Errorf("expected %s, got %s", validKey, pk)
		}
	})

	t.Run("fails for revoked instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		_ = inst.Revoke()
		repo.instances[validUUID] = inst

		_, err := svc.GetPublicKey(ctx, validUUID)
		if !errors.Is(err, domain.ErrInstanceRevoked) {
			t.Errorf("expected ErrInstanceRevoked, got %v", err)
		}
	})
}

func TestInstanceService_Revoke(t *testing.T) {
	ctx := context.Background()

	t.Run("revokes active instance", func(t *testing.T) {
		repo := newMockInstanceRepo()
		svc := NewInstanceService(repo, newTestApplicationService())

		inst, _ := domain.NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		_ = inst.Activate()
		repo.instances[validUUID] = inst

		err := svc.Revoke(ctx, validUUID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if repo.instances[validUUID].Status != domain.StatusRevoked {
			t.Error("instance should be revoked")
		}
	})
}
