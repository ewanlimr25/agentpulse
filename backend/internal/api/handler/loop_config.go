package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// LoopConfigHandler handles GET and PUT for per-project loop-detection thresholds.
type LoopConfigHandler struct {
	projects store.ProjectStore
}

func NewLoopConfigHandler(projects store.ProjectStore) *LoopConfigHandler {
	return &LoopConfigHandler{projects: projects}
}

// Get handles GET /api/v1/projects/{projectID}/loop-config.
// Returns the current config or the global defaults if none is set.
// Authenticated via BearerAuth (read access).
func (h *LoopConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	cfg, err := h.projects.GetLoopConfig(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to get loop config")
		return
	}
	httputil.JSON(w, http.StatusOK, cfg)
}

// Put handles PUT /api/v1/projects/{projectID}/loop-config.
// Saves the config; requires X-Admin-Key.
// Authenticated via AdminKeyAuth (write access).
func (h *LoopConfigHandler) Put(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var cfg domain.LoopConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.Tier1MinCount < 1 {
		httputil.Error(w, http.StatusBadRequest, "tier1_min_count must be >= 1")
		return
	}
	if cfg.Tier2MinCount < 1 {
		httputil.Error(w, http.StatusBadRequest, "tier2_min_count must be >= 1")
		return
	}
	if cfg.Tier2MaxIntervalMs < 1 {
		httputil.Error(w, http.StatusBadRequest, "tier2_max_interval_ms must be >= 1")
		return
	}

	if err := h.projects.PutLoopConfig(r.Context(), projectID, cfg); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save loop config")
		return
	}

	adminKey := r.Header.Get("X-Admin-Key")
	adminKeyPrefix := ""
	if len(adminKey) >= 8 {
		adminKeyPrefix = adminKey[:8]
	} else {
		adminKeyPrefix = adminKey
	}
	slog.Info("loop config updated",
		"project_id", projectID,
		"tier1_min_count", cfg.Tier1MinCount,
		"tier2_min_count", cfg.Tier2MinCount,
		"tier2_max_interval_ms", cfg.Tier2MaxIntervalMs,
		"admin_key_prefix", adminKeyPrefix,
	)

	updated, err := h.projects.GetLoopConfig(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to retrieve updated loop config")
		return
	}
	httputil.JSON(w, http.StatusOK, updated)
}
