// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"fmt"

	"github.com/kolapsis/shm/internal/app/ports"
	"github.com/kolapsis/shm/internal/domain"
)

// RegisterInstanceInput holds the data needed to register an instance.
type RegisterInstanceInput struct {
	InstanceID     string
	PublicKey      string
	AppName        string
	AppVersion     string
	DeploymentMode string
	Environment    string
	OSArch         string
}

// InstanceService handles instance-related use cases.
type InstanceService struct {
	repo   ports.InstanceRepository
	appSvc *ApplicationService
}

// NewInstanceService creates a new InstanceService.
func NewInstanceService(repo ports.InstanceRepository, appSvc *ApplicationService) *InstanceService {
	return &InstanceService{
		repo:   repo,
		appSvc: appSvc,
	}
}

// Register registers a new instance or updates an existing one.
// This is an unauthenticated endpoint - instances self-register with their public key.
func (s *InstanceService) Register(ctx context.Context, input RegisterInstanceInput) error {
	// Auto-create or get the application
	app, err := s.appSvc.CreateOrGet(ctx, input.AppName)
	if err != nil {
		return fmt.Errorf("register instance: %w", err)
	}

	// Create and validate the domain entity
	instance, err := domain.NewInstance(
		input.InstanceID,
		input.PublicKey,
		input.AppName,
		input.AppVersion,
		input.DeploymentMode,
		input.Environment,
		input.OSArch,
	)
	if err != nil {
		return fmt.Errorf("register instance: %w", err)
	}

	// Link instance to application
	instance.ApplicationID = app.ID

	// Check if instance already exists
	existing, err := s.repo.FindByID(ctx, instance.ID)
	if err == nil && existing != nil {
		// Instance exists - update metadata but preserve status
		existing.ApplicationID = app.ID
		existing.AppName = instance.AppName
		existing.AppVersion = instance.AppVersion
		existing.DeploymentMode = instance.DeploymentMode
		existing.Environment = instance.Environment
		existing.OSArch = instance.OSArch
		existing.UpdateHeartbeat()
		instance = existing
	}

	// Persist
	if err := s.repo.Save(ctx, instance); err != nil {
		return fmt.Errorf("register instance: %w", err)
	}

	return nil
}

// Activate transitions an instance from pending to active status.
// This requires a valid signature (verified by middleware before calling this).
func (s *InstanceService) Activate(ctx context.Context, instanceID string) error {
	id, err := domain.NewInstanceID(instanceID)
	if err != nil {
		return fmt.Errorf("activate instance: %w", err)
	}

	instance, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("activate instance: %w", err)
	}

	if err := instance.Activate(); err != nil {
		return fmt.Errorf("activate instance: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, id, instance.Status); err != nil {
		return fmt.Errorf("activate instance: %w", err)
	}

	return nil
}

// GetPublicKey retrieves the public key for signature verification.
// Returns an error if the instance is not found or is revoked.
func (s *InstanceService) GetPublicKey(ctx context.Context, instanceID string) (string, error) {
	id, err := domain.NewInstanceID(instanceID)
	if err != nil {
		return "", fmt.Errorf("get public key: %w", err)
	}

	pk, err := s.repo.GetPublicKey(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get public key: %w", err)
	}

	return pk.String(), nil
}

// Revoke revokes an instance, preventing further snapshots.
func (s *InstanceService) Revoke(ctx context.Context, instanceID string) error {
	id, err := domain.NewInstanceID(instanceID)
	if err != nil {
		return fmt.Errorf("revoke instance: %w", err)
	}

	instance, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("revoke instance: %w", err)
	}

	if err := instance.Revoke(); err != nil {
		return fmt.Errorf("revoke instance: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, id, instance.Status); err != nil {
		return fmt.Errorf("revoke instance: %w", err)
	}

	return nil
}

// Delete permanently removes an instance and all its associated snapshots.
func (s *InstanceService) Delete(ctx context.Context, instanceID string) error {
	id, err := domain.NewInstanceID(instanceID)
	if err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}

	return nil
}
