// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"fmt"
	"regexp"
	"time"
)

// InstanceStatus represents the lifecycle state of an instance.
type InstanceStatus string

const (
	StatusPending InstanceStatus = "pending"
	StatusActive  InstanceStatus = "active"
	StatusRevoked InstanceStatus = "revoked"
)

// InstanceHealth is the freshness derived from LastSeenAt — independent
// from the registration lifecycle (InstanceStatus).
type InstanceHealth string

const (
	HealthOK        InstanceHealth = "ok"        // last seen < LateAfter
	HealthLate      InstanceHealth = "late"      // LateAfter <= last seen < SilentAfter
	HealthSilent    InstanceHealth = "silent"    // SilentAfter <= last seen < InactiveAfter
	HealthInactive  InstanceHealth = "inactive"  // InactiveAfter <= last seen < AbandonedAfter (hidden from dashboard)
	HealthAbandoned InstanceHealth = "abandoned" // last seen >= AbandonedAfter (purged from DB)
)

// Freshness thresholds. Default push interval is 1h (see sdk/golang/client.go).
const (
	LateAfter      = 3 * time.Hour
	SilentAfter    = 24 * time.Hour
	InactiveAfter  = 7 * 24 * time.Hour
	AbandonedAfter = 30 * 24 * time.Hour
)

// ComputeHealth returns the InstanceHealth for a given last-seen time.
func ComputeHealth(lastSeenAt, now time.Time) InstanceHealth {
	age := now.Sub(lastSeenAt)
	switch {
	case age >= AbandonedAfter:
		return HealthAbandoned
	case age >= InactiveAfter:
		return HealthInactive
	case age >= SilentAfter:
		return HealthSilent
	case age >= LateAfter:
		return HealthLate
	default:
		return HealthOK
	}
}

// Valid returns true if the status is a known value.
func (s InstanceStatus) Valid() bool {
	switch s {
	case StatusPending, StatusActive, StatusRevoked:
		return true
	default:
		return false
	}
}

// CanTransitionTo checks if a status transition is allowed.
func (s InstanceStatus) CanTransitionTo(target InstanceStatus) bool {
	switch s {
	case StatusPending:
		return target == StatusActive || target == StatusRevoked
	case StatusActive:
		return target == StatusRevoked
	case StatusRevoked:
		return false // Terminal state
	default:
		return false
	}
}

// InstanceID is a validated instance identifier (UUID format).
type InstanceID string

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NewInstanceID creates and validates an InstanceID.
func NewInstanceID(id string) (InstanceID, error) {
	if id == "" {
		return "", ErrInvalidInstanceID
	}
	if !uuidRegex.MatchString(id) {
		return "", fmt.Errorf("%w: invalid UUID format", ErrInvalidInstanceID)
	}
	return InstanceID(id), nil
}

// String returns the string representation.
func (id InstanceID) String() string {
	return string(id)
}

// PublicKey is a hex-encoded Ed25519 public key.
type PublicKey string

// NewPublicKey creates and validates a PublicKey.
func NewPublicKey(key string) (PublicKey, error) {
	if key == "" {
		return "", ErrInvalidPublicKey
	}
	// Ed25519 public key = 32 bytes = 64 hex chars
	if len(key) != 64 {
		return "", fmt.Errorf("%w: expected 64 hex chars, got %d", ErrInvalidPublicKey, len(key))
	}
	for _, c := range key {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", fmt.Errorf("%w: invalid hex character", ErrInvalidPublicKey)
		}
	}
	return PublicKey(key), nil
}

// String returns the string representation.
func (pk PublicKey) String() string {
	return string(pk)
}

// Instance represents a registered telemetry client instance.
type Instance struct {
	ID             InstanceID
	PublicKey      PublicKey
	ApplicationID  ApplicationID // Foreign key to applications table
	AppName        string        // Denormalized for compatibility
	AppVersion     string
	DeploymentMode string
	Environment    string
	OSArch         string
	Status         InstanceStatus
	LastSeenAt     time.Time
	CreatedAt      time.Time
}

// NewInstance creates a new Instance with validation.
func NewInstance(
	instanceID string,
	publicKey string,
	appName string,
	appVersion string,
	deploymentMode string,
	environment string,
	osArch string,
) (*Instance, error) {
	id, err := NewInstanceID(instanceID)
	if err != nil {
		return nil, err
	}

	pk, err := NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	if appName == "" {
		return nil, fmt.Errorf("%w: app_name is required", ErrInvalidInstance)
	}

	now := time.Now().UTC()
	return &Instance{
		ID:             id,
		PublicKey:      pk,
		AppName:        appName,
		AppVersion:     appVersion,
		DeploymentMode: deploymentMode,
		Environment:    environment,
		OSArch:         osArch,
		Status:         StatusPending,
		LastSeenAt:     now,
		CreatedAt:      now,
	}, nil
}

// Activate transitions the instance to active status.
func (i *Instance) Activate() error {
	if !i.Status.CanTransitionTo(StatusActive) {
		return fmt.Errorf("%w: cannot activate from status %s", ErrInvalidStatusTransition, i.Status)
	}
	i.Status = StatusActive
	i.LastSeenAt = time.Now().UTC()
	return nil
}

// Revoke transitions the instance to revoked status.
func (i *Instance) Revoke() error {
	if !i.Status.CanTransitionTo(StatusRevoked) {
		return fmt.Errorf("%w: cannot revoke from status %s", ErrInvalidStatusTransition, i.Status)
	}
	i.Status = StatusRevoked
	return nil
}

// UpdateHeartbeat updates the last seen timestamp.
func (i *Instance) UpdateHeartbeat() {
	i.LastSeenAt = time.Now().UTC()
}

// IsActive returns true if the instance is in active status.
func (i *Instance) IsActive() bool {
	return i.Status == StatusActive
}

// IsRevoked returns true if the instance is revoked.
func (i *Instance) IsRevoked() bool {
	return i.Status == StatusRevoked
}
