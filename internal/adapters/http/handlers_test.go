// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/btouchard/shm/internal/app"
	"github.com/btouchard/shm/internal/app/ports"
	"github.com/btouchard/shm/internal/domain"
)

// Test fixtures
const (
	testUUID = "550e8400-e29b-41d4-a716-446655440000"
	testKey  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

// Mock repositories
type mockInstanceRepo struct {
	instances map[string]*domain.Instance
	saveErr   error
}

func newMockInstanceRepo() *mockInstanceRepo {
	return &mockInstanceRepo{instances: make(map[string]*domain.Instance)}
}

func (m *mockInstanceRepo) Save(ctx context.Context, instance *domain.Instance) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.instances[instance.ID.String()] = instance
	return nil
}

func (m *mockInstanceRepo) FindByID(ctx context.Context, id domain.InstanceID) (*domain.Instance, error) {
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
	inst, ok := m.instances[id.String()]
	if !ok {
		return domain.ErrInstanceNotFound
	}
	inst.Status = status
	return nil
}

func (m *mockInstanceRepo) Delete(ctx context.Context, id domain.InstanceID) error {
	if _, ok := m.instances[id.String()]; !ok {
		return domain.ErrInstanceNotFound
	}
	delete(m.instances, id.String())
	return nil
}

type mockSnapshotRepo struct {
	snapshots []*domain.Snapshot
	saveErr   error
}

func (m *mockSnapshotRepo) Save(ctx context.Context, snapshot *domain.Snapshot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.snapshots = append(m.snapshots, snapshot)
	return nil
}

func (m *mockSnapshotRepo) FindByInstanceID(ctx context.Context, id domain.InstanceID, limit int) ([]*domain.Snapshot, error) {
	return m.snapshots, nil
}

func (m *mockSnapshotRepo) GetLatestByInstanceID(ctx context.Context, id domain.InstanceID) (*domain.Snapshot, error) {
	if len(m.snapshots) == 0 {
		return nil, errors.New("no snapshots")
	}
	return m.snapshots[len(m.snapshots)-1], nil
}

type mockDashboardReader struct {
	stats     ports.DashboardStats
	instances []ports.InstanceSummary
}

func (m *mockDashboardReader) GetStats(ctx context.Context) (ports.DashboardStats, error) {
	return m.stats, nil
}

func (m *mockDashboardReader) ListInstances(ctx context.Context, offset, limit int, appName, search string) ([]ports.InstanceSummary, error) {
	return m.instances, nil
}

func (m *mockDashboardReader) GetMetricsTimeSeries(ctx context.Context, appName string, since time.Time) (ports.MetricsTimeSeries, error) {
	return ports.MetricsTimeSeries{
		Timestamps: []time.Time{time.Now().UTC()},
		Metrics:    map[string][]float64{"cpu": {0.5}},
	}, nil
}

func (m *mockDashboardReader) GetActiveInstancesCount(ctx context.Context, appSlug string) (int, error) {
	return 0, nil
}

func (m *mockDashboardReader) GetMostUsedVersion(ctx context.Context, appSlug string) (string, error) {
	return "", nil
}

func (m *mockDashboardReader) GetAggregatedMetric(ctx context.Context, appSlug, metricName string) (float64, error) {
	return 0, nil
}

func (m *mockDashboardReader) GetCombinedStats(ctx context.Context, appSlug, metricName string) (float64, int, error) {
	return 0, 0, nil
}

// mockApplicationRepo for HTTP tests
type mockApplicationRepo struct {
	apps map[string]*domain.Application
}

func newMockApplicationRepo() *mockApplicationRepo {
	return &mockApplicationRepo{apps: make(map[string]*domain.Application)}
}

func (m *mockApplicationRepo) Save(ctx context.Context, app *domain.Application) error {
	m.apps[app.Slug.String()] = app
	return nil
}

func (m *mockApplicationRepo) FindByID(ctx context.Context, id domain.ApplicationID) (*domain.Application, error) {
	for _, app := range m.apps {
		if app.ID == id {
			return app, nil
		}
	}
	return nil, domain.ErrApplicationNotFound
}

func (m *mockApplicationRepo) FindBySlug(ctx context.Context, slug domain.AppSlug) (*domain.Application, error) {
	if app, ok := m.apps[slug.String()]; ok {
		return app, nil
	}
	return nil, domain.ErrApplicationNotFound
}

func (m *mockApplicationRepo) List(ctx context.Context, limit int) ([]*domain.Application, error) {
	result := make([]*domain.Application, 0)
	for _, app := range m.apps {
		result = append(result, app)
	}
	return result, nil
}

func (m *mockApplicationRepo) UpdateStars(ctx context.Context, id domain.ApplicationID, stars int) error {
	return nil
}

// mockGitHubService for HTTP tests
type mockGitHubService struct{}

func (m *mockGitHubService) GetStars(ctx context.Context, repoURL domain.GitHubURL) (int, error) {
	return 0, nil
}

// Helper to create a test ApplicationService
func newTestApplicationService() *app.ApplicationService {
	return app.NewApplicationService(newMockApplicationRepo(), &mockGitHubService{}, nil)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestHandlers_Healthcheck(t *testing.T) {
	instanceRepo := newMockInstanceRepo()
	snapshotRepo := &mockSnapshotRepo{}
	dashboardReader := &mockDashboardReader{}

	instanceSvc := app.NewInstanceService(instanceRepo, nil)
	snapshotSvc := app.NewSnapshotService(snapshotRepo, instanceRepo)
	dashboardSvc := app.NewDashboardService(dashboardReader)

	handlers := NewHandlers(instanceSvc, snapshotSvc, nil, dashboardSvc, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthcheck", nil)
	rec := httptest.NewRecorder()

	handlers.Healthcheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var response map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &response)
	if response["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", response["status"])
	}
}

func TestHandlers_Register(t *testing.T) {
	t.Run("registers new instance", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		appSvc := newTestApplicationService()
		instanceSvc := app.NewInstanceService(instanceRepo, appSvc)
		snapshotSvc := app.NewSnapshotService(&mockSnapshotRepo{}, instanceRepo)
		dashboardSvc := app.NewDashboardService(&mockDashboardReader{})
		handlers := NewHandlers(instanceSvc, snapshotSvc, nil, dashboardSvc, testLogger())

		body := `{
			"instance_id": "` + testUUID + `",
			"public_key": "` + testKey + `",
			"app_name": "myapp",
			"app_version": "1.0.0"
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/register", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handlers.Register(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
		}

		if _, ok := instanceRepo.instances[testUUID]; !ok {
			t.Error("instance not saved")
		}
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		appSvc := newTestApplicationService()
		instanceSvc := app.NewInstanceService(instanceRepo, appSvc)
		snapshotSvc := app.NewSnapshotService(&mockSnapshotRepo{}, instanceRepo)
		dashboardSvc := app.NewDashboardService(&mockDashboardReader{})
		handlers := NewHandlers(instanceSvc, snapshotSvc, nil, dashboardSvc, testLogger())

		req := httptest.NewRequest(http.MethodPost, "/v1/register", strings.NewReader("{invalid"))
		rec := httptest.NewRecorder()

		handlers.Register(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("rejects non-POST", func(t *testing.T) {
		handlers := NewHandlers(nil, nil, nil, nil, testLogger())

		req := httptest.NewRequest(http.MethodGet, "/v1/register", nil)
		rec := httptest.NewRecorder()

		handlers.Register(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status 405, got %d", rec.Code)
		}
	})
}

func TestHandlers_Activate(t *testing.T) {
	t.Run("activates pending instance", func(t *testing.T) {
		instanceRepo := newMockInstanceRepo()
		inst, _ := domain.NewInstance(testUUID, testKey, "myapp", "1.0", "docker", "prod", "linux/amd64")
		instanceRepo.instances[testUUID] = inst

		instanceSvc := app.NewInstanceService(instanceRepo, nil)
		snapshotSvc := app.NewSnapshotService(&mockSnapshotRepo{}, instanceRepo)
		dashboardSvc := app.NewDashboardService(&mockDashboardReader{})
		handlers := NewHandlers(instanceSvc, snapshotSvc, nil, dashboardSvc, testLogger())

		req := httptest.NewRequest(http.MethodPost, "/v1/activate", nil)
		req.Header.Set("X-Instance-ID", testUUID)
		rec := httptest.NewRecorder()

		handlers.Activate(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		if instanceRepo.instances[testUUID].Status != domain.StatusActive {
			t.Error("instance should be active")
		}
	})
}

func TestHandlers_AdminStats(t *testing.T) {
	dashboardReader := &mockDashboardReader{
		stats: ports.DashboardStats{
			TotalInstances:  100,
			ActiveInstances: 75,
			GlobalMetrics:   map[string]int64{"cpu": 500},
		},
	}

	instanceSvc := app.NewInstanceService(newMockInstanceRepo(), nil)
	snapshotSvc := app.NewSnapshotService(&mockSnapshotRepo{}, newMockInstanceRepo())
	dashboardSvc := app.NewDashboardService(dashboardReader)
	handlers := NewHandlers(instanceSvc, snapshotSvc, nil, dashboardSvc, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats", nil)
	rec := httptest.NewRecorder()

	handlers.AdminStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var response map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &response)

	if response["total_instances"].(float64) != 100 {
		t.Errorf("expected total_instances=100, got %v", response["total_instances"])
	}
}

func TestHandlers_AdminInstances(t *testing.T) {
	id, _ := domain.NewInstanceID(testUUID)
	dashboardReader := &mockDashboardReader{
		instances: []ports.InstanceSummary{
			{
				ID:         id,
				AppName:    "myapp",
				AppVersion: "1.0.0",
				Status:     domain.StatusActive,
			},
		},
	}

	instanceSvc := app.NewInstanceService(newMockInstanceRepo(), nil)
	snapshotSvc := app.NewSnapshotService(&mockSnapshotRepo{}, newMockInstanceRepo())
	dashboardSvc := app.NewDashboardService(dashboardReader)
	handlers := NewHandlers(instanceSvc, snapshotSvc, nil, dashboardSvc, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/instances", nil)
	rec := httptest.NewRecorder()

	handlers.AdminInstances(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var response []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &response)

	if len(response) != 1 {
		t.Errorf("expected 1 instance, got %d", len(response))
	}
	if response[0]["app_name"] != "myapp" {
		t.Errorf("expected app_name=myapp, got %v", response[0]["app_name"])
	}
}
