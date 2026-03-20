package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type RunHandler struct {
	runs  store.RunStore
	spans store.SpanStore
}

func NewRunHandler(runs store.RunStore, spans store.SpanStore) *RunHandler {
	return &RunHandler{runs: runs, spans: spans}
}

func (h *RunHandler) Routes(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/{runID}", h.Get)
	r.Get("/{runID}/spans", h.ListSpans)
}

// List returns paginated runs for a project.
// Route: GET /api/v1/projects/{projectID}/runs
func (h *RunHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 50)
	offset := intQueryParam(r, "offset", 0)

	runs, err := h.runs.List(r.Context(), projectID, limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"runs":   runs,
		"limit":  limit,
		"offset": offset,
	})
}

// Get returns a single run with aggregated metrics.
// Route: GET /api/v1/runs/{runID}
func (h *RunHandler) Get(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := h.runs.Get(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "run not found")
		return
	}
	httputil.JSON(w, http.StatusOK, run)
}

// ListSpans returns all spans for a run.
// Route: GET /api/v1/runs/{runID}/spans
func (h *RunHandler) ListSpans(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	spans, err := h.spans.ListByRun(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list spans")
		return
	}
	httputil.JSON(w, http.StatusOK, spans)
}

func intQueryParam(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
