package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type TopologyHandler struct {
	topology store.TopologyStore
}

func NewTopologyHandler(topology store.TopologyStore) *TopologyHandler {
	return &TopologyHandler{topology: topology}
}

func (h *TopologyHandler) Routes(r chi.Router) {
	// Mounted under /api/v1/runs/{runID}/topology
	r.Get("/", h.get)
}

// get returns the DAG topology (nodes + edges) for a run.
func (h *TopologyHandler) get(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	topo, err := h.topology.GetByRun(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "topology not found")
		return
	}
	httputil.JSON(w, http.StatusOK, topo)
}
