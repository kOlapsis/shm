// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kolapsis/shm/internal/domain"
)

// ApplicationRepository implements ports.ApplicationRepository for PostgreSQL.
type ApplicationRepository struct {
	db *sql.DB
}

// NewApplicationRepository creates a new ApplicationRepository.
func NewApplicationRepository(db *sql.DB) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

// Save persists an application (insert or update).
func (r *ApplicationRepository) Save(ctx context.Context, app *domain.Application) error {
	query := `
		INSERT INTO applications (id, app_slug, app_name, github_url, github_stars, github_stars_updated_at, logo_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (app_slug) DO UPDATE
		SET app_name = EXCLUDED.app_name,
			github_url = COALESCE(EXCLUDED.github_url, applications.github_url),
			github_stars = CASE WHEN EXCLUDED.github_stars > 0 THEN EXCLUDED.github_stars ELSE applications.github_stars END,
			github_stars_updated_at = CASE WHEN EXCLUDED.github_stars > 0 THEN EXCLUDED.github_stars_updated_at ELSE applications.github_stars_updated_at END,
			logo_url = COALESCE(EXCLUDED.logo_url, applications.logo_url),
			updated_at = EXCLUDED.updated_at
		RETURNING id
	`

	var githubURL *string
	if app.GitHubURL != "" {
		url := app.GitHubURL.String()
		githubURL = &url
	}

	var logoURL *string
	if app.LogoURL != "" {
		logoURL = &app.LogoURL
	}

	var id string
	err := r.db.QueryRowContext(ctx, query,
		app.ID.String(),
		app.Slug.String(),
		app.Name,
		githubURL,
		app.Stars,
		app.StarsUpdatedAt,
		logoURL,
		app.CreatedAt,
		app.UpdatedAt,
	).Scan(&id)

	if err != nil {
		return fmt.Errorf("save application %s: %w", app.Slug, err)
	}

	// Update ID if it was generated
	if app.ID == "" {
		appID, _ := domain.NewApplicationID(id)
		app.ID = appID
	}

	return nil
}

// FindByID retrieves an application by its ID.
func (r *ApplicationRepository) FindByID(ctx context.Context, id domain.ApplicationID) (*domain.Application, error) {
	query := `
		SELECT id, app_slug, app_name, github_url, github_stars, github_stars_updated_at, logo_url, created_at, updated_at
		FROM applications
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id.String())

	return r.scanApplication(row, id.String())
}

// FindBySlug retrieves an application by its slug.
func (r *ApplicationRepository) FindBySlug(ctx context.Context, slug domain.AppSlug) (*domain.Application, error) {
	query := `
		SELECT id, app_slug, app_name, github_url, github_stars, github_stars_updated_at, logo_url, created_at, updated_at
		FROM applications
		WHERE app_slug = $1
	`
	row := r.db.QueryRowContext(ctx, query, slug.String())

	return r.scanApplication(row, slug.String())
}

// List retrieves all applications, limited by the specified count.
func (r *ApplicationRepository) List(ctx context.Context, limit int) ([]*domain.Application, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, app_slug, app_name, github_url, github_stars, github_stars_updated_at, logo_url, created_at, updated_at
		FROM applications
		ORDER BY app_name ASC
		LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	apps := make([]*domain.Application, 0)
	for rows.Next() {
		app, err := r.scanApplicationFromRows(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}

	return apps, nil
}

// UpdateStars updates only the GitHub stars count and timestamp.
func (r *ApplicationRepository) UpdateStars(ctx context.Context, id domain.ApplicationID, stars int) error {
	query := `
		UPDATE applications
		SET github_stars = $1, github_stars_updated_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, stars, id.String())
	if err != nil {
		return fmt.Errorf("update stars for %s: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrApplicationNotFound
	}

	return nil
}

// scanApplication scans a single row into an Application entity.
func (r *ApplicationRepository) scanApplication(row *sql.Row, identifier string) (*domain.Application, error) {
	var app domain.Application
	var appID, appSlug string
	var githubURL, logoURL sql.NullString

	err := row.Scan(
		&appID,
		&appSlug,
		&app.Name,
		&githubURL,
		&app.Stars,
		&app.StarsUpdatedAt,
		&logoURL,
		&app.CreatedAt,
		&app.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find application %s: %w", identifier, err)
	}

	// Reconstruct value objects (already validated in DB)
	app.ID = domain.ApplicationID(appID)
	app.Slug = domain.AppSlug(appSlug)

	if githubURL.Valid {
		app.GitHubURL = domain.GitHubURL(githubURL.String)
	}

	if logoURL.Valid {
		app.LogoURL = logoURL.String
	}

	return &app, nil
}

// scanApplicationFromRows scans a row from sql.Rows into an Application entity.
func (r *ApplicationRepository) scanApplicationFromRows(rows *sql.Rows) (*domain.Application, error) {
	var app domain.Application
	var appID, appSlug string
	var githubURL, logoURL sql.NullString

	err := rows.Scan(
		&appID,
		&appSlug,
		&app.Name,
		&githubURL,
		&app.Stars,
		&app.StarsUpdatedAt,
		&logoURL,
		&app.CreatedAt,
		&app.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("scan application: %w", err)
	}

	// Reconstruct value objects
	app.ID = domain.ApplicationID(appID)
	app.Slug = domain.AppSlug(appSlug)

	if githubURL.Valid {
		app.GitHubURL = domain.GitHubURL(githubURL.String)
	}

	if logoURL.Valid {
		app.LogoURL = logoURL.String
	}

	return &app, nil
}
