// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewInstanceID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "valid UUID",
			input:   "550e8400-e29b-41d4-a716-446655440000",
			wantErr: nil,
		},
		{
			name:    "valid UUID uppercase",
			input:   "550E8400-E29B-41D4-A716-446655440000",
			wantErr: nil,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: ErrInvalidInstanceID,
		},
		{
			name:    "invalid format - no dashes",
			input:   "550e8400e29b41d4a716446655440000",
			wantErr: ErrInvalidInstanceID,
		},
		{
			name:    "invalid format - too short",
			input:   "550e8400-e29b-41d4",
			wantErr: ErrInvalidInstanceID,
		},
		{
			name:    "invalid format - not hex",
			input:   "550e8400-e29b-41d4-a716-44665544000g",
			wantErr: ErrInvalidInstanceID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewInstanceID(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if id.String() != tt.input {
					t.Errorf("expected %s, got %s", tt.input, id.String())
				}
			}
		})
	}
}

func TestNewPublicKey(t *testing.T) {
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "valid 64 hex chars",
			input:   validKey,
			wantErr: nil,
		},
		{
			name:    "valid uppercase",
			input:   "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
			wantErr: nil,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: ErrInvalidPublicKey,
		},
		{
			name:    "too short",
			input:   "0123456789abcdef",
			wantErr: ErrInvalidPublicKey,
		},
		{
			name:    "too long",
			input:   validKey + "00",
			wantErr: ErrInvalidPublicKey,
		},
		{
			name:    "invalid hex char",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg",
			wantErr: ErrInvalidPublicKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pk, err := NewPublicKey(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if pk.String() != tt.input {
					t.Errorf("expected %s, got %s", tt.input, pk.String())
				}
			}
		})
	}
}

func TestInstanceStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from   InstanceStatus
		to     InstanceStatus
		expect bool
	}{
		{StatusPending, StatusActive, true},
		{StatusPending, StatusRevoked, true},
		{StatusPending, StatusPending, false},
		{StatusActive, StatusRevoked, true},
		{StatusActive, StatusPending, false},
		{StatusActive, StatusActive, false},
		{StatusRevoked, StatusPending, false},
		{StatusRevoked, StatusActive, false},
		{StatusRevoked, StatusRevoked, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestNewInstance(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name       string
		instanceID string
		publicKey  string
		appName    string
		wantErr    error
	}{
		{
			name:       "valid instance",
			instanceID: validUUID,
			publicKey:  validKey,
			appName:    "myapp",
			wantErr:    nil,
		},
		{
			name:       "invalid instance ID",
			instanceID: "invalid",
			publicKey:  validKey,
			appName:    "myapp",
			wantErr:    ErrInvalidInstanceID,
		},
		{
			name:       "invalid public key",
			instanceID: validUUID,
			publicKey:  "invalid",
			appName:    "myapp",
			wantErr:    ErrInvalidPublicKey,
		},
		{
			name:       "missing app name",
			instanceID: validUUID,
			publicKey:  validKey,
			appName:    "",
			wantErr:    ErrInvalidInstance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := NewInstance(tt.instanceID, tt.publicKey, tt.appName, "1.0", "docker", "prod", "linux/amd64")
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if inst.Status != StatusPending {
					t.Errorf("expected status %s, got %s", StatusPending, inst.Status)
				}
			}
		})
	}
}

func TestInstance_Activate(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	inst, _ := NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")

	// Activate from pending
	if err := inst.Activate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if inst.Status != StatusActive {
		t.Errorf("expected status %s, got %s", StatusActive, inst.Status)
	}

	// Cannot activate again
	if err := inst.Activate(); err == nil {
		t.Error("expected error when activating already active instance")
	}
}

func TestInstance_Revoke(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// Revoke from pending
	inst1, _ := NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
	if err := inst1.Revoke(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if inst1.Status != StatusRevoked {
		t.Errorf("expected status %s, got %s", StatusRevoked, inst1.Status)
	}

	// Cannot revoke again
	if err := inst1.Revoke(); err == nil {
		t.Error("expected error when revoking already revoked instance")
	}

	// Revoke from active
	inst2, _ := NewInstance(validUUID, validKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
	_ = inst2.Activate()
	if err := inst2.Revoke(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if inst2.Status != StatusRevoked {
		t.Errorf("expected status %s, got %s", StatusRevoked, inst2.Status)
	}
}

func TestComputeHealth(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		ago  time.Duration
		want InstanceHealth
	}{
		{"fresh", time.Minute, HealthOK},
		{"just-under-late", LateAfter - time.Second, HealthOK},
		{"late-boundary", LateAfter, HealthLate},
		{"mid-late", 12 * time.Hour, HealthLate},
		{"silent-boundary", SilentAfter, HealthSilent},
		{"mid-silent", 3 * 24 * time.Hour, HealthSilent},
		{"inactive-boundary", InactiveAfter, HealthInactive},
		{"mid-inactive", 14 * 24 * time.Hour, HealthInactive},
		{"abandoned-boundary", AbandonedAfter, HealthAbandoned},
		{"long-abandoned", 100 * 24 * time.Hour, HealthAbandoned},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeHealth(now.Add(-tc.ago), now)
			if got != tc.want {
				t.Errorf("age=%s: got %s, want %s", tc.ago, got, tc.want)
			}
		})
	}
}
