// SPDX-License-Identifier: AGPL-3.0-or-later

// Package http provides HTTP handlers that delegate to application services.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/btouchard/shm/internal/app"
	"github.com/btouchard/shm/internal/app/ports"
)

// Handlers holds HTTP handlers and their dependencies.
type Handlers struct {
	instances    *app.InstanceService
	snapshots    *app.SnapshotService
	applications *app.ApplicationService
	dashboard    *app.DashboardService
	logger       *slog.Logger
}

// NewHandlers creates a new Handlers with the given services.
func NewHandlers(
	instances *app.InstanceService,
	snapshots *app.SnapshotService,
	applications *app.ApplicationService,
	dashboard *app.DashboardService,
	logger *slog.Logger,
) *Handlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{
		instances:    instances,
		snapshots:    snapshots,
		applications: applications,
		dashboard:    dashboard,
		logger:       logger,
	}
}

// Healthcheck returns a simple health status.
func (h *Handlers) Healthcheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RegisterRequest is the JSON payload for instance registration.
type RegisterRequest struct {
	InstanceID     string `json:"instance_id"`
	PublicKey      string `json:"public_key"`
	AppName        string `json:"app_name"`
	AppVersion     string `json:"app_version"`
	DeploymentMode string `json:"deployment_mode"`
	Environment    string `json:"environment"`
	OSArch         string `json:"os_arch"`
}

// Register handles instance registration requests.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("invalid JSON", "error", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	h.logger.Info("registering instance",
		"instance_id", req.InstanceID,
		"app_name", req.AppName,
		"app_version", req.AppVersion,
	)

	err := h.instances.Register(r.Context(), app.RegisterInstanceInput{
		InstanceID:     req.InstanceID,
		PublicKey:      req.PublicKey,
		AppName:        req.AppName,
		AppVersion:     req.AppVersion,
		DeploymentMode: req.DeploymentMode,
		Environment:    req.Environment,
		OSArch:         req.OSArch,
	})
	if err != nil {
		h.logger.Error("registration failed", "instance_id", req.InstanceID, "error", err)
		http.Error(w, "Registration failed", http.StatusBadRequest)
		return
	}

	h.logger.Info("instance registered", "instance_id", req.InstanceID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Registered"})
}

// Activate handles instance activation requests.
func (h *Handlers) Activate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	instanceID := r.Header.Get("X-Instance-ID")
	h.logger.Info("activating instance", "instance_id", instanceID)

	err := h.instances.Activate(r.Context(), instanceID)
	if err != nil {
		h.logger.Error("activation failed", "instance_id", instanceID, "error", err)
		http.Error(w, "Activation failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info("instance activated", "instance_id", instanceID)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "active", "message": "Instance activated successfully"})
}

// SnapshotRequest is the JSON payload for snapshot submission.
type SnapshotRequest struct {
	InstanceID string          `json:"instance_id"`
	Timestamp  time.Time       `json:"timestamp"`
	Metrics    json.RawMessage `json:"metrics"`
}

// Snapshot handles snapshot submission requests.
func (h *Handlers) Snapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	instanceID := r.Header.Get("X-Instance-ID")

	var req SnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("invalid JSON", "error", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err := h.snapshots.Save(r.Context(), app.SaveSnapshotInput{
		InstanceID: req.InstanceID,
		Timestamp:  req.Timestamp,
		Metrics:    req.Metrics,
	})
	if err != nil {
		h.logger.Error("snapshot failed", "instance_id", instanceID, "error", err)
		http.Error(w, "Snapshot failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info("snapshot received", "instance_id", instanceID)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Snapshot received"})
}

// AdminStats handles dashboard statistics requests.
func (h *Handlers) AdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetStats(r.Context())
	if err != nil {
		h.logger.Error("failed to get stats", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("stats retrieved", "total", stats.TotalInstances, "active", stats.ActiveInstances)

	// Convert to JSON-friendly format
	response := map[string]any{
		"total_instances":  stats.TotalInstances,
		"active_instances": stats.ActiveInstances,
		"global_metrics":   stats.GlobalMetrics,
		"per_app_counts":   stats.PerAppCounts,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// AdminInstances handles instance listing requests.
func (h *Handlers) AdminInstances(w http.ResponseWriter, r *http.Request) {
	// Parse pagination params
	offset := 0
	limit := 50
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	// Parse filter params
	appName := r.URL.Query().Get("app")    // Filter by app name
	search := r.URL.Query().Get("q")       // Search in instance_id, version, env, mode

	instances, err := h.dashboard.ListInstances(r.Context(), offset, limit, appName, search)
	if err != nil {
		h.logger.Error("failed to list instances", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if instances == nil {
		instances = []ports.InstanceSummary{}
	}

	h.logger.Info("instances listed", "count", len(instances))

	// Convert to JSON-friendly format
	response := make([]map[string]any, 0, len(instances))
	for _, inst := range instances {
		item := map[string]any{
			"instance_id":     inst.ID.String(),
			"app_name":        inst.AppName,
			"app_slug":        inst.AppSlug,
			"app_version":     inst.AppVersion,
			"environment":     inst.Environment,
			"status":          string(inst.Status),
			"last_seen_at":    inst.LastSeenAt,
			"deployment_mode": inst.DeploymentMode,
			"metrics":         inst.Metrics,
		}

		response = append(response, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// AdminMetrics handles metrics time-series requests.
func (h *Handlers) AdminMetrics(w http.ResponseWriter, r *http.Request) {
	appName := r.URL.Path[len("/api/v1/admin/metrics/"):]
	if appName == "" {
		http.Error(w, "App name required", http.StatusBadRequest)
		return
	}

	periodParam := r.URL.Query().Get("period")
	period := app.ParsePeriod(periodParam)

	h.logger.Info("getting metrics", "app", appName, "period", period)

	data, err := h.dashboard.GetMetricsTimeSeries(r.Context(), appName, period)
	if err != nil {
		h.logger.Error("failed to get metrics", "app", appName, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert timestamps to strings for JSON
	timestamps := make([]string, 0, len(data.Timestamps))
	for _, ts := range data.Timestamps {
		timestamps = append(timestamps, ts.Format(time.RFC3339))
	}

	response := map[string]any{
		"timestamps": timestamps,
		"metrics":    data.Metrics,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// UpdateApplicationRequest is the JSON payload for updating an application.
type UpdateApplicationRequest struct {
	GitHubURL string `json:"github_url"`
	LogoURL   string `json:"logo_url"`
}

// AdminListApplications handles listing all applications.
func (h *Handlers) AdminListApplications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apps, err := h.applications.List(r.Context(), 100)
	if err != nil {
		h.logger.Error("failed to list applications", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to JSON-friendly format
	response := make([]map[string]any, 0, len(apps))
	for _, application := range apps {
		item := map[string]any{
			"id":         application.ID.String(),
			"slug":       application.Slug.String(),
			"name":       application.Name,
			"stars":      application.Stars,
			"logo_url":   application.LogoURL,
			"created_at": application.CreatedAt,
			"updated_at": application.UpdatedAt,
		}

		if application.GitHubURL != "" {
			item["github_url"] = application.GitHubURL.String()
		}

		if application.StarsUpdatedAt != nil {
			item["stars_updated_at"] = application.StarsUpdatedAt
		}

		response = append(response, item)
	}

	h.logger.Info("applications listed", "count", len(apps))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// AdminGetApplication handles getting a single application by slug.
func (h *Handlers) AdminGetApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.URL.Path[len("/api/v1/admin/applications/"):]
	if slug == "" {
		http.Error(w, "Application slug required", http.StatusBadRequest)
		return
	}

	application, err := h.applications.GetBySlug(r.Context(), slug)
	if err != nil {
		h.logger.Error("failed to get application", "slug", slug, "error", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	response := map[string]any{
		"id":         application.ID.String(),
		"slug":       application.Slug.String(),
		"name":       application.Name,
		"stars":      application.Stars,
		"logo_url":   application.LogoURL,
		"created_at": application.CreatedAt,
		"updated_at": application.UpdatedAt,
	}

	if application.GitHubURL != "" {
		response["github_url"] = application.GitHubURL.String()
	}

	if application.StarsUpdatedAt != nil {
		response["stars_updated_at"] = application.StarsUpdatedAt
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// AdminUpdateApplication handles updating an application's metadata.
func (h *Handlers) AdminUpdateApplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.URL.Path[len("/api/v1/admin/applications/"):]
	if slug == "" {
		http.Error(w, "Application slug required", http.StatusBadRequest)
		return
	}

	var req UpdateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("invalid JSON", "error", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err := h.applications.Update(r.Context(), app.UpdateApplicationInput{
		Slug:      slug,
		GitHubURL: req.GitHubURL,
		LogoURL:   req.LogoURL,
	})

	if err != nil {
		h.logger.Error("failed to update application", "slug", slug, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Info("application updated", "slug", slug)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Application updated"})
}

// AdminRefreshStars handles manual GitHub stars refresh for a specific application.
func (h *Handlers) AdminRefreshStars(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract slug from path /api/v1/admin/applications/{slug}/refresh-stars
	path := r.URL.Path
	slug := path[len("/api/v1/admin/applications/"):]
	if idx := len(slug) - len("/refresh-stars"); idx > 0 {
		slug = slug[:idx]
	}

	if slug == "" {
		http.Error(w, "Application slug required", http.StatusBadRequest)
		return
	}

	err := h.applications.RefreshStars(r.Context(), slug)
	if err != nil {
		h.logger.Error("failed to refresh stars", "slug", slug, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("stars refreshed", "slug", slug)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Stars refreshed"})
}

// AdminDeleteInstance handles instance deletion requests.
func (h *Handlers) AdminDeleteInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract instance_id from path /api/v1/admin/instances/{instance_id}
	instanceID := r.URL.Path[len("/api/v1/admin/instances/"):]
	if instanceID == "" {
		http.Error(w, "Instance ID required", http.StatusBadRequest)
		return
	}

	h.logger.Info("deleting instance", "instance_id", instanceID)

	err := h.instances.Delete(r.Context(), instanceID)
	if err != nil {
		h.logger.Error("failed to delete instance", "instance_id", instanceID, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Info("instance deleted", "instance_id", instanceID)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Instance deleted"})
}
