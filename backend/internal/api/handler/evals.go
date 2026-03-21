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
