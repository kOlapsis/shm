// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kolapsis/shm/internal/domain"
)

func TestSnapshotRepository_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("saves snapshot with transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewSnapshotRepository(db)
		now := time.Now().UTC()
		snap, _ := domain.NewSnapshot(testUUID, now, json.RawMessage(`{"cpu": 0.5}`))

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO snapshots").
			WithArgs(testUUID, now, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("UPDATE instances SET last_seen_at").
			WithArgs(testUUID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err = repo.Save(ctx, snap)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("rolls back on insert error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewSnapshotRepository(db)
		now := time.Now().UTC()
		snap, _ := domain.NewSnapshot(testUUID, now, json.RawMessage(`{}`))

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO snapshots").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err = repo.Save(ctx, snap)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestSnapshotRepository_FindByInstanceID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns snapshots", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewSnapshotRepository(db)
		id, _ := domain.NewInstanceID(testUUID)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{"id", "instance_id", "snapshot_at", "data"}).
			AddRow(1, testUUID, now, `{"cpu": 0.5}`).
			AddRow(2, testUUID, now.Add(-1*time.Hour), `{"cpu": 0.3}`)

		mock.ExpectQuery("SELECT .+ FROM snapshots").
			WithArgs(testUUID, 10).
			WillReturnRows(rows)

		snapshots, err := repo.FindByInstanceID(ctx, id, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(snapshots) != 2 {
			t.Errorf("expected 2 snapshots, got %d", len(snapshots))
		}

		cpu, ok := snapshots[0].Metrics.GetFloat64("cpu")
		if !ok || cpu != 0.5 {
			t.Errorf("expected cpu=0.5, got %v", cpu)
		}
	})
}

func TestSnapshotRepository_GetLatestByInstanceID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns latest snapshot", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewSnapshotRepository(db)
		id, _ := domain.NewInstanceID(testUUID)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{"id", "instance_id", "snapshot_at", "data"}).
			AddRow(1, testUUID, now, `{"cpu": 0.5}`)

		mock.ExpectQuery("SELECT .+ FROM snapshots").
			WithArgs(testUUID).
			WillReturnRows(rows)

		snap, err := repo.GetLatestByInstanceID(ctx, id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if snap.ID != 1 {
			t.Errorf("expected ID=1, got %d", snap.ID)
		}
	})
}
