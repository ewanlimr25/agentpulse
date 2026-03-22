package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type LoopHandler struct {
	loops store.LoopStore
}

func NewLoopHandler(loops store.LoopStore) *LoopHandler {
	return &LoopHandler{loops: loops}
}

// ListByRun handles GET /api/v1/runs/{runID}/loops
func (h *LoopHandler) ListByRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	loops, err := h.loops.ListByRun(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list loops")
		return
	}
	if loops == nil {
		loops = []*domain.RunLoop{}
	}
	httputil.JSON(w, http.StatusOK, loops)
}
