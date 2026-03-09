// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kolapsis/shm/internal/domain"
)

const (
	testUUID = "550e8400-e29b-41d4-a716-446655440000"
	testKey  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestInstanceRepository_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("saves new instance", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		inst, _ := domain.NewInstance(testUUID, testKey, "myapp", "1.0", "docker", "prod", "linux/amd64")

		mock.ExpectExec("INSERT INTO instances").
			WithArgs(
				testUUID, testKey, sqlmock.AnyArg(), "myapp", "1.0", "docker", "prod", "linux/amd64",
				string(domain.StatusPending), sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.Save(ctx, inst)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("returns error on DB failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		inst, _ := domain.NewInstance(testUUID, testKey, "myapp", "1.0", "docker", "prod", "linux/amd64")

		mock.ExpectExec("INSERT INTO instances").
			WillReturnError(errors.New("db error"))

		err = repo.Save(ctx, inst)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestInstanceRepository_FindByID(t *testing.T) {
	ctx := context.Background()

	t.Run("finds existing instance", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		id, _ := domain.NewInstanceID(testUUID)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{
			"instance_id", "public_key", "application_id", "app_name", "app_version",
			"deployment_mode", "environment", "os_arch", "status",
			"last_seen_at", "created_at",
		}).AddRow(
			testUUID, testKey, nil, "myapp", "1.0",
			"docker", "prod", "linux/amd64", "active",
			now, now,
		)

		mock.ExpectQuery("SELECT .+ FROM instances").
			WithArgs(testUUID).
			WillReturnRows(rows)

		inst, err := repo.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if inst.AppName != "myapp" {
			t.Errorf("expected app_name=myapp, got %s", inst.AppName)
		}
		if inst.Status != domain.StatusActive {
			t.Errorf("expected status=active, got %s", inst.Status)
		}
	})

	t.Run("returns ErrInstanceNotFound", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		id, _ := domain.NewInstanceID(testUUID)

		mock.ExpectQuery("SELECT .+ FROM instances").
			WithArgs(testUUID).
			WillReturnError(sql.ErrNoRows)

		_, err = repo.FindByID(ctx, id)
		if !errors.Is(err, domain.ErrInstanceNotFound) {
			t.Errorf("expected ErrInstanceNotFound, got %v", err)
		}
	})
}

func TestInstanceRepository_GetPublicKey(t *testing.T) {
	ctx := context.Background()

	t.Run("returns public key", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		id, _ := domain.NewInstanceID(testUUID)

		rows := sqlmock.NewRows([]string{"public_key", "status"}).
			AddRow(testKey, "active")

		mock.ExpectQuery("SELECT public_key, status FROM instances").
			WithArgs(testUUID).
			WillReturnRows(rows)

		pk, err := repo.GetPublicKey(ctx, id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pk.String() != testKey {
			t.Errorf("expected key %s, got %s", testKey, pk)
		}
	})

	t.Run("returns ErrInstanceRevoked", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		id, _ := domain.NewInstanceID(testUUID)

		rows := sqlmock.NewRows([]string{"public_key", "status"}).
			AddRow(testKey, "revoked")

		mock.ExpectQuery("SELECT public_key, status FROM instances").
			WithArgs(testUUID).
			WillReturnRows(rows)

		_, err = repo.GetPublicKey(ctx, id)
		if !errors.Is(err, domain.ErrInstanceRevoked) {
			t.Errorf("expected ErrInstanceRevoked, got %v", err)
		}
	})
}

func TestInstanceRepository_UpdateStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("updates status", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		id, _ := domain.NewInstanceID(testUUID)

		mock.ExpectExec("UPDATE instances SET status").
			WithArgs("active", testUUID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.UpdateStatus(ctx, id, domain.StatusActive)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("returns ErrInstanceNotFound when no rows affected", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewInstanceRepository(db)
		id, _ := domain.NewInstanceID(testUUID)

		mock.ExpectExec("UPDATE instances SET status").
			WithArgs("active", testUUID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.UpdateStatus(ctx, id, domain.StatusActive)
		if !errors.Is(err, domain.ErrInstanceNotFound) {
			t.Errorf("expected ErrInstanceNotFound, got %v", err)
		}
	})
}
