package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// HealthHandler serves the project collector health endpoint.
type HealthHandler struct {
	spans store.SpanStore
}

// NewHealthHandler constructs a HealthHandler backed by the given SpanStore.
func NewHealthHandler(spans store.SpanStore) *HealthHandler {
	return &HealthHandler{spans: spans}
}

// Status handles GET /api/v1/projects/{projectID}/health.
// It reports whether the project's collector pipeline has received spans recently.
func (h *HealthHandler) Status(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	lastSpanAt, err := h.spans.LatestSpanTime(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query span health")
		return
	}

	reachable := lastSpanAt != nil && time.Since(*lastSpanAt) < 5*time.Minute

	spansPerMinute, err := h.spans.CountSince(r.Context(), projectID, time.Minute)
	if err != nil {
		spansPerMinute = 0 // non-fatal; degrade gracefully
	}

	httputil.JSON(w, http.StatusOK, domain.ProjectHealth{
		CollectorReachable: reachable,
		LastSpanAt:         lastSpanAt,
		SpanCount:          0,
		SpansPerMinute:     spansPerMinute,
	})
}
