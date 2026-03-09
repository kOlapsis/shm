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
	testAppUUID = "650e8400-e29b-41d4-a716-446655440001"
	testSlug    = "my-app"
)

func TestApplicationRepository_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("saves new application", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		app, _ := domain.NewApplication(testSlug, "My App")
		app.ID = domain.ApplicationID(testAppUUID)

		mock.ExpectQuery("INSERT INTO applications").
			WithArgs(
				testAppUUID, testSlug, "My App",
				nil, 0, nil, nil,
				sqlmock.AnyArg(), sqlmock.AnyArg(),
			).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(testAppUUID))

		err = repo.Save(ctx, app)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("saves application with GitHub URL", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		app, _ := domain.NewApplication(testSlug, "My App")
		app.ID = domain.ApplicationID(testAppUUID)
		_ = app.SetGitHubURL("https://github.com/owner/repo")

		githubURL := "https://github.com/owner/repo"
		mock.ExpectQuery("INSERT INTO applications").
			WithArgs(
				testAppUUID, testSlug, "My App",
				&githubURL, 0, nil, nil,
				sqlmock.AnyArg(), sqlmock.AnyArg(),
			).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(testAppUUID))

		err = repo.Save(ctx, app)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("returns error on DB failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		app, _ := domain.NewApplication(testSlug, "My App")
		app.ID = domain.ApplicationID(testAppUUID)

		mock.ExpectQuery("INSERT INTO applications").
			WillReturnError(errors.New("db error"))

		err = repo.Save(ctx, app)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestApplicationRepository_FindByID(t *testing.T) {
	ctx := context.Background()

	t.Run("finds existing application", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		id, _ := domain.NewApplicationID(testAppUUID)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{
			"id", "app_slug", "app_name", "github_url",
			"github_stars", "github_stars_updated_at", "logo_url",
			"created_at", "updated_at",
		}).AddRow(
			testAppUUID, testSlug, "My App", "https://github.com/owner/repo",
			42, now, nil,
			now, now,
		)

		mock.ExpectQuery("SELECT .+ FROM applications").
			WithArgs(testAppUUID).
			WillReturnRows(rows)

		app, err := repo.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if app.Name != "My App" {
			t.Errorf("expected name=My App, got %s", app.Name)
		}
		if app.Slug.String() != testSlug {
			t.Errorf("expected slug=%s, got %s", testSlug, app.Slug)
		}
		if app.Stars != 42 {
			t.Errorf("expected stars=42, got %d", app.Stars)
		}
	})

	t.Run("returns ErrApplicationNotFound", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		id, _ := domain.NewApplicationID(testAppUUID)

		mock.ExpectQuery("SELECT .+ FROM applications").
			WithArgs(testAppUUID).
			WillReturnError(sql.ErrNoRows)

		_, err = repo.FindByID(ctx, id)
		if !errors.Is(err, domain.ErrApplicationNotFound) {
			t.Errorf("expected ErrApplicationNotFound, got %v", err)
		}
	})
}

func TestApplicationRepository_FindBySlug(t *testing.T) {
	ctx := context.Background()

	t.Run("finds existing application by slug", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		slug, _ := domain.NewAppSlug(testSlug)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{
			"id", "app_slug", "app_name", "github_url",
			"github_stars", "github_stars_updated_at", "logo_url",
			"created_at", "updated_at",
		}).AddRow(
			testAppUUID, testSlug, "My App", nil,
			0, nil, nil,
			now, now,
		)

		mock.ExpectQuery("SELECT .+ FROM applications").
			WithArgs(testSlug).
			WillReturnRows(rows)

		app, err := repo.FindBySlug(ctx, slug)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if app.Slug.String() != testSlug {
			t.Errorf("expected slug=%s, got %s", testSlug, app.Slug)
		}
	})

	t.Run("returns ErrApplicationNotFound", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		slug, _ := domain.NewAppSlug(testSlug)

		mock.ExpectQuery("SELECT .+ FROM applications").
			WithArgs(testSlug).
			WillReturnError(sql.ErrNoRows)

		_, err = repo.FindBySlug(ctx, slug)
		if !errors.Is(err, domain.ErrApplicationNotFound) {
			t.Errorf("expected ErrApplicationNotFound, got %v", err)
		}
	})
}

func TestApplicationRepository_List(t *testing.T) {
	ctx := context.Background()

	t.Run("lists applications", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		now := time.Now().UTC()

		rows := sqlmock.NewRows([]string{
			"id", "app_slug", "app_name", "github_url",
			"github_stars", "github_stars_updated_at", "logo_url",
			"created_at", "updated_at",
		}).
			AddRow(testAppUUID, "app1", "App 1", nil, 0, nil, nil, now, now).
			AddRow(testAppUUID, "app2", "App 2", "https://github.com/owner/repo", 10, now, nil, now, now)

		mock.ExpectQuery("SELECT .+ FROM applications").
			WithArgs(50).
			WillReturnRows(rows)

		apps, err := repo.List(ctx, 50)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(apps) != 2 {
			t.Errorf("expected 2 apps, got %d", len(apps))
		}

		if apps[0].Name != "App 1" {
			t.Errorf("expected first app name=App 1, got %s", apps[0].Name)
		}
		if apps[1].Stars != 10 {
			t.Errorf("expected second app stars=10, got %d", apps[1].Stars)
		}
	})

	t.Run("returns empty list", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)

		rows := sqlmock.NewRows([]string{
			"id", "app_slug", "app_name", "github_url",
			"github_stars", "github_stars_updated_at", "logo_url",
			"created_at", "updated_at",
		})

		mock.ExpectQuery("SELECT .+ FROM applications").
			WithArgs(100). // Default limit
			WillReturnRows(rows)

		apps, err := repo.List(ctx, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if apps == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(apps) != 0 {
			t.Errorf("expected 0 apps, got %d", len(apps))
		}
	})
}

func TestApplicationRepository_UpdateStars(t *testing.T) {
	ctx := context.Background()

	t.Run("updates stars", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		id, _ := domain.NewApplicationID(testAppUUID)

		mock.ExpectExec("UPDATE applications SET github_stars").
			WithArgs(100, testAppUUID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.UpdateStars(ctx, id, 100)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("returns ErrApplicationNotFound when no rows affected", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		id, _ := domain.NewApplicationID(testAppUUID)

		mock.ExpectExec("UPDATE applications SET github_stars").
			WithArgs(50, testAppUUID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.UpdateStars(ctx, id, 50)
		if !errors.Is(err, domain.ErrApplicationNotFound) {
			t.Errorf("expected ErrApplicationNotFound, got %v", err)
		}
	})

	t.Run("returns error on DB failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock: %v", err)
		}
		defer db.Close()

		repo := NewApplicationRepository(db)
		id, _ := domain.NewApplicationID(testAppUUID)

		mock.ExpectExec("UPDATE applications SET github_stars").
			WillReturnError(errors.New("db error"))

		err = repo.UpdateStars(ctx, id, 50)
		if err == nil {
			t.Error("expected error")
		}
	})
}
