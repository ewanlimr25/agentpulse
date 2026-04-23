package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	internalstorage "github.com/agentpulse/agentpulse/backend/internal/storage"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// StorageHandler handles storage management endpoints (stats, retention, purge).
type StorageHandler struct {
	stats     *internalstorage.StatsService
	retention store.RetentionStore
	purgeJobs store.PurgeJobStore
	executor  *internalstorage.PurgeExecutor
}

// NewStorageHandler creates a StorageHandler.
func NewStorageHandler(
	stats *internalstorage.StatsService,
	retention store.RetentionStore,
	purgeJobs store.PurgeJobStore,
	executor *internalstorage.PurgeExecutor,
) *StorageHandler {
	return &StorageHandler{
		stats:     stats,
		retention: retention,
		purgeJobs: purgeJobs,
		executor:  executor,
	}
}

// GetStats handles GET /api/v1/projects/{projectID}/storage/stats.
// Authenticated via BearerAuth.
func (h *StorageHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	stats, err := h.stats.GetStats(r.Context(), projectID)
	if err != nil {
		slog.Error("storage: get stats", "project_id", projectID, "error", err)
		httputil.Error(w, http.StatusInternalServerError, "failed to compute storage stats")
		return
	}
	httputil.JSON(w, http.StatusOK, stats)
}

// GetRetention handles GET /api/v1/projects/{projectID}/storage/retention.
// Authenticated via BearerAuth.
func (h *StorageHandler) GetRetention(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	cfg, err := h.retention.Get(r.Context(), projectID)
	if err != nil {
		slog.Error("storage: get retention", "project_id", projectID, "error", err)
		httputil.Error(w, http.StatusInternalServerError, "failed to get retention config")
		return
	}
	httputil.JSON(w, http.StatusOK, cfg)
}

type putRetentionRequest struct {
	RetentionDays int `json:"retention_days"`
}

// PutRetention handles PUT /api/v1/projects/{projectID}/storage/retention.
// Authenticated via AdminKeyAuth.
func (h *StorageHandler) PutRetention(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req putRetentionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RetentionDays < 7 || req.RetentionDays > 365 {
		httputil.Error(w, http.StatusBadRequest, "retention_days must be between 7 and 365")
		return
	}

	cfg := &domain.RetentionConfig{
		ProjectID:     projectID,
		RetentionDays: req.RetentionDays,
	}
	if err := h.retention.Upsert(r.Context(), cfg); err != nil {
		slog.Error("storage: put retention", "project_id", projectID, "error", err)
		httputil.Error(w, http.StatusInternalServerError, "failed to save retention config")
		return
	}

	updated, err := h.retention.Get(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to retrieve updated retention config")
		return
	}
	httputil.JSON(w, http.StatusOK, updated)
}

type purgeRunRequest struct {
	IncludeEvals bool `json:"include_evals"`
	DryRun       bool `json:"dry_run"`
}

// PurgeRun handles POST /api/v1/projects/{projectID}/storage/purge/run/{runID}.
// Authenticated via AdminKeyAuth.
func (h *StorageHandler) PurgeRun(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	runID := chi.URLParam(r, "runID")

	var req purgeRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DryRun {
		// Dry run: count spans without deleting anything.
		count, err := h.executor.CountRunSpans(r.Context(), projectID, runID)
		if err != nil {
			slog.Error("storage: purge run dry run count", "run_id", runID, "error", err)
			httputil.Error(w, http.StatusInternalServerError, "failed to count spans")
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]int64{"spans_to_delete": count})
		return
	}

	job := &domain.PurgeJob{
		ProjectID:    projectID,
		RunID:        runID,
		IncludeEvals: req.IncludeEvals,
		Status:       "pending",
	}
	if err := h.purgeJobs.Create(r.Context(), job); err != nil {
		slog.Error("storage: purge run create job", "run_id", runID, "error", err)
		httputil.Error(w, http.StatusInternalServerError, "failed to create purge job")
		return
	}

	// Use context.Background so the goroutine outlives the request.
	go func() {
		if err := h.executor.ExecuteRunPurge(context.Background(), job); err != nil {
			slog.Error("storage: purge run execute", "job_id", job.ID, "run_id", runID, "error", err)
		}
	}()

	httputil.JSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

type purgeByAgeRequest struct {
	BeforeDays   int  `json:"before_days"`
	IncludeEvals bool `json:"include_evals"`
	DryRun       bool `json:"dry_run"`
}

// PurgeByAge handles POST /api/v1/projects/{projectID}/storage/purge/age.
// Authenticated via AdminKeyAuth.
func (h *StorageHandler) PurgeByAge(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req purgeByAgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BeforeDays < 7 {
		httputil.Error(w, http.StatusBadRequest, "before_days must be >= 7")
		return
	}

	cutoff := domain.AgeCutoff(req.BeforeDays)

	if req.DryRun {
		count, err := h.executor.CountAgeSpans(r.Context(), projectID, cutoff)
		if err != nil {
			slog.Error("storage: purge age dry run count", "project_id", projectID, "error", err)
			httputil.Error(w, http.StatusInternalServerError, "failed to count spans")
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]int64{"spans_to_delete": count})
		return
	}

	job := &domain.PurgeJob{
		ProjectID:    projectID,
		CutoffDate:   &cutoff,
		IncludeEvals: req.IncludeEvals,
		Status:       "pending",
	}
	if err := h.purgeJobs.Create(r.Context(), job); err != nil {
		slog.Error("storage: purge age create job", "project_id", projectID, "error", err)
		httputil.Error(w, http.StatusInternalServerError, "failed to create purge job")
		return
	}

	// Use context.Background so the goroutine outlives the request.
	go func() {
		if err := h.executor.ExecuteAgePurge(context.Background(), job); err != nil {
			slog.Error("storage: purge age execute", "job_id", job.ID, "project_id", projectID, "error", err)
		}
	}()

	httputil.JSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}

// GetPurgeJob handles GET /api/v1/projects/{projectID}/storage/purge-jobs/{jobID}.
// Authenticated via BearerAuth.
func (h *StorageHandler) GetPurgeJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := h.purgeJobs.Get(r.Context(), jobID)
	if err != nil {
		slog.Error("storage: get purge job", "job_id", jobID, "error", err)
		httputil.Error(w, http.StatusNotFound, "purge job not found")
		return
	}
	httputil.JSON(w, http.StatusOK, job)
}
