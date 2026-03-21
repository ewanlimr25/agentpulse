package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type EvalHandler struct {
	evals store.EvalStore
}

func NewEvalHandler(evals store.EvalStore) *EvalHandler {
	return &EvalHandler{evals: evals}
}

// ListByRun returns all evals for spans in a run.
// Route: GET /api/v1/runs/{runID}/evals
func (h *EvalHandler) ListByRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	evals, err := h.evals.ListByRun(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list evals")
		return
	}
	httputil.JSON(w, http.StatusOK, evals)
}

// SummaryByProject returns avg quality score per run for a project.
// Route: GET /api/v1/projects/{projectID}/evals/summary
func (h *EvalHandler) SummaryByProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	summaries, err := h.evals.SummaryByProject(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to get eval summary")
		return
	}
	httputil.JSON(w, http.StatusOK, summaries)
}
