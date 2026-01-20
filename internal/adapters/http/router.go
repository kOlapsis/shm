// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/btouchard/shm/internal/adapters/github"
	"github.com/btouchard/shm/internal/adapters/postgres"
	"github.com/btouchard/shm/internal/app"
	"github.com/btouchard/shm/internal/middleware"
	"github.com/btouchard/shm/internal/services"
)

// RouterConfig holds the configuration for creating a new router.
type RouterConfig struct {
	Store        *postgres.Store
	RateLimiter  *middleware.RateLimiter
	GitHubToken  string // Optional GitHub API token for higher rate limits
	Logger       *slog.Logger
}

// NewRouter creates a fully wired HTTP router with all handlers and middleware.
func NewRouter(cfg RouterConfig) *http.ServeMux {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	instanceRepo := cfg.Store.InstanceRepository()
	snapshotRepo := cfg.Store.SnapshotRepository()
	applicationRepo := cfg.Store.ApplicationRepository()
	dashboardReader := cfg.Store.DashboardReader()

	githubSvc := github.NewStarsService(cfg.GitHubToken)
	githubSvc.StartCleanup(context.Background())

	applicationSvc := app.NewApplicationService(applicationRepo, githubSvc, logger)
	instanceSvc := app.NewInstanceService(instanceRepo, applicationSvc)
	snapshotSvc := app.NewSnapshotService(snapshotRepo, instanceRepo)
	dashboardSvc := app.NewDashboardService(dashboardReader)

	scheduler := services.NewScheduler(applicationSvc, logger)
	go scheduler.Start(context.Background())

	handlers := NewHandlers(instanceSvc, snapshotSvc, applicationSvc, dashboardSvc, logger)
	authMW := NewAuthMiddlewareFromService(instanceSvc, logger)
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/healthcheck", handlers.Healthcheck)

	mux.HandleFunc("/badge/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/instances"):
			handlers.BadgeInstances(w, r)
		case strings.HasSuffix(path, "/version"):
			handlers.BadgeVersion(w, r)
		case strings.HasSuffix(path, "/combined"):
			handlers.BadgeCombined(w, r)
		case strings.Contains(path, "/metric/"):
			handlers.BadgeMetric(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	rl := cfg.RateLimiter
	if rl != nil {
		mux.HandleFunc("/v1/register", rl.RegisterMiddleware(handlers.Register))
		mux.HandleFunc("/v1/activate", rl.RegisterMiddleware(authMW.RequireSignature(handlers.Activate)))
		mux.HandleFunc("/v1/snapshot", rl.SnapshotMiddleware(authMW.RequireSignature(handlers.Snapshot)))
		mux.HandleFunc("/api/v1/admin/stats", rl.AdminMiddleware(handlers.AdminStats))
		mux.HandleFunc("/api/v1/admin/instances/", rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/admin/instances/" || r.URL.Path == "/api/v1/admin/instances" {
				handlers.AdminInstances(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				handlers.AdminDeleteInstance(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}))
		mux.HandleFunc("/api/v1/admin/instances", rl.AdminMiddleware(handlers.AdminInstances))
		mux.HandleFunc("/api/v1/admin/metrics/", rl.AdminMiddleware(handlers.AdminMetrics))
		mux.HandleFunc("/api/v1/admin/applications", rl.AdminMiddleware(handlers.AdminListApplications))
		mux.HandleFunc("/api/v1/admin/applications/", rl.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/admin/applications/" {
				handlers.AdminListApplications(w, r)
				return
			}
			if len(r.URL.Path) > len("/api/v1/admin/applications/") &&
			   r.URL.Path[len(r.URL.Path)-len("/refresh-stars"):] == "/refresh-stars" {
				handlers.AdminRefreshStars(w, r)
				return
			}
			if r.Method == http.MethodGet {
				handlers.AdminGetApplication(w, r)
			} else if r.Method == http.MethodPut {
				handlers.AdminUpdateApplication(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}))
	} else {
		mux.HandleFunc("/v1/register", handlers.Register)
		mux.HandleFunc("/v1/activate", authMW.RequireSignature(handlers.Activate))
		mux.HandleFunc("/v1/snapshot", authMW.RequireSignature(handlers.Snapshot))
		mux.HandleFunc("/api/v1/admin/stats", handlers.AdminStats)
		mux.HandleFunc("/api/v1/admin/instances/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/admin/instances/" || r.URL.Path == "/api/v1/admin/instances" {
				handlers.AdminInstances(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				handlers.AdminDeleteInstance(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/api/v1/admin/instances", handlers.AdminInstances)
		mux.HandleFunc("/api/v1/admin/metrics/", handlers.AdminMetrics)
		mux.HandleFunc("/api/v1/admin/applications", handlers.AdminListApplications)
		mux.HandleFunc("/api/v1/admin/applications/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/admin/applications/" {
				handlers.AdminListApplications(w, r)
				return
			}
			if len(r.URL.Path) > len("/api/v1/admin/applications/") &&
			   r.URL.Path[len(r.URL.Path)-len("/refresh-stars"):] == "/refresh-stars" {
				handlers.AdminRefreshStars(w, r)
				return
			}
			if r.Method == http.MethodGet {
				handlers.AdminGetApplication(w, r)
			} else if r.Method == http.MethodPut {
				handlers.AdminUpdateApplication(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	return mux
}
