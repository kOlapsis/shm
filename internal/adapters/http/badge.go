// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	"net/http"
	"strings"

	"github.com/kolapsis/shm/internal/services/badge"
)

func (h *Handlers) BadgeInstances(w http.ResponseWriter, r *http.Request) {
	appSlug := extractSlugFromPath(r.URL.Path, "/badge/", "/instances")
	if appSlug == "" {
		renderErrorBadge(w, "invalid slug")
		return
	}

	count, err := h.dashboard.GetActiveInstancesCount(r.Context(), appSlug)
	if err != nil {
		h.logger.Warn("failed to get instances count", "slug", appSlug, "error", err)
		renderErrorBadge(w, "error")
		return
	}

	color := badge.GetInstancesColor(count)
	if customColor := r.URL.Query().Get("color"); customColor != "" {
		color = "#" + strings.TrimPrefix(customColor, "#")
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = "instances"
	}

	b := badge.NewBadge(label, badge.FormatNumber(float64(count)), color)
	renderSVGBadge(w, b.ToSVG())
}

func (h *Handlers) BadgeVersion(w http.ResponseWriter, r *http.Request) {
	appSlug := extractSlugFromPath(r.URL.Path, "/badge/", "/version")
	if appSlug == "" {
		renderErrorBadge(w, "invalid slug")
		return
	}

	version, err := h.dashboard.GetMostUsedVersion(r.Context(), appSlug)
	if err != nil {
		h.logger.Warn("failed to get version", "slug", appSlug, "error", err)
		renderErrorBadge(w, "error")
		return
	}

	if version == "" {
		version = "no data"
	}

	color := badge.ColorPurple
	if customColor := r.URL.Query().Get("color"); customColor != "" {
		color = "#" + strings.TrimPrefix(customColor, "#")
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = "version"
	}

	b := badge.NewBadge(label, version, color)
	renderSVGBadge(w, b.ToSVG())
}

func (h *Handlers) BadgeMetric(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/badge/"), "/")
	if len(parts) < 3 || parts[1] != "metric" {
		renderErrorBadge(w, "invalid path")
		return
	}

	appSlug := parts[0]
	metricName := parts[2]

	if appSlug == "" || metricName == "" {
		renderErrorBadge(w, "invalid params")
		return
	}

	value, err := h.dashboard.GetAggregatedMetric(r.Context(), appSlug, metricName)
	if err != nil {
		h.logger.Warn("failed to get metric", "slug", appSlug, "metric", metricName, "error", err)
		renderErrorBadge(w, "error")
		return
	}

	color := badge.GetMetricColor(value)
	if customColor := r.URL.Query().Get("color"); customColor != "" {
		color = "#" + strings.TrimPrefix(customColor, "#")
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = metricName
	}

	b := badge.NewBadge(label, badge.FormatNumber(value), color)
	renderSVGBadge(w, b.ToSVG())
}

func (h *Handlers) BadgeCombined(w http.ResponseWriter, r *http.Request) {
	appSlug := extractSlugFromPath(r.URL.Path, "/badge/", "/combined")
	if appSlug == "" {
		renderErrorBadge(w, "invalid slug")
		return
	}

	metricName := r.URL.Query().Get("metric")
	if metricName == "" {
		metricName = "users_count"
	}

	metricValue, instanceCount, err := h.dashboard.GetCombinedStats(r.Context(), appSlug, metricName)
	if err != nil {
		h.logger.Warn("failed to get combined stats", "slug", appSlug, "metric", metricName, "error", err)
		renderErrorBadge(w, "error")
		return
	}

	color := badge.ColorIndigo
	if customColor := r.URL.Query().Get("color"); customColor != "" {
		color = "#" + strings.TrimPrefix(customColor, "#")
	}

	label := r.URL.Query().Get("label")
	if label == "" {
		label = "adoption"
	}

	value := badge.FormatNumber(metricValue) + " / " + badge.FormatNumber(float64(instanceCount))
	b := badge.NewBadge(label, value, color)
	renderSVGBadge(w, b.ToSVG())
}

func extractSlugFromPath(path, prefix, suffix string) string {
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimSuffix(s, suffix)
	return s
}

func renderSVGBadge(w http.ResponseWriter, svg string) {
	w.Header().Set("Content-Type", "image/svg+xml;charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(svg))
}

func renderErrorBadge(w http.ResponseWriter, message string) {
	b := badge.NewBadge("error", message, badge.ColorRed)
	renderSVGBadge(w, b.ToSVG())
}
