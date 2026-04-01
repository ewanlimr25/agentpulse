package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const maxCorrectedOutputLen = 10_000

// SpanFeedbackHandler handles human-in-the-loop ratings for individual spans.
type SpanFeedbackHandler struct {
	feedback store.SpanFeedbackStore
}

func NewSpanFeedbackHandler(feedback store.SpanFeedbackStore) *SpanFeedbackHandler {
	return &SpanFeedbackHandler{feedback: feedback}
}

type spanFeedbackRequest struct {
	// RunID is required so feedback can be listed by run without a ClickHouse lookup.
	RunID           string  `json:"run_id"`
	Rating          string  `json:"rating"`
	CorrectedOutput *string `json:"corrected_output,omitempty"`
}

func (req *spanFeedbackRequest) validate() string {
	if req.RunID == "" {
		return "run_id is required"
	}
	if req.Rating != "good" && req.Rating != "bad" {
		return "rating must be 'good' or 'bad'"
	}
	if req.CorrectedOutput != nil {
		// Reject null bytes — they cause issues in Postgres text columns and logging.
		if strings.ContainsRune(*req.CorrectedOutput, 0) {
			return "corrected_output must not contain null bytes"
		}
		if len(*req.CorrectedOutput) > maxCorrectedOutputLen {
			return "corrected_output must not exceed 10 000 characters"
		}
	}
	return ""
}

// Upsert handles POST /projects/{projectID}/spans/{spanID}/feedback.
// Creates or replaces feedback for a span. Allows changing your mind.
//
// Note: spanID is not validated against ClickHouse — spans live in ClickHouse
// and a cross-store lookup on every write would be expensive. A caller with a
// valid Bearer token for this project can only affect rows scoped to that project,
// so phantom span IDs are a data-integrity concern, not a confidentiality one.
func (h *SpanFeedbackHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	spanID := chi.URLParam(r, "spanID")

	var req spanFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		httputil.Error(w, http.StatusBadRequest, msg)
		return
	}

	f := &domain.SpanFeedback{
		ID:              uuid.New().String(),
		ProjectID:       projectID,
		SpanID:          spanID,
		RunID:           req.RunID,
		Rating:          req.Rating,
		CorrectedOutput: req.CorrectedOutput,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := h.feedback.Upsert(r.Context(), f); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save feedback")
		return
	}
	httputil.JSON(w, http.StatusOK, f)
}

// Get handles GET /projects/{projectID}/spans/{spanID}/feedback.
func (h *SpanFeedbackHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	spanID := chi.URLParam(r, "spanID")

	f, err := h.feedback.GetBySpan(r.Context(), projectID, spanID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to get feedback")
		return
	}
	if f == nil {
		httputil.Error(w, http.StatusNotFound, "no feedback found for this span")
		return
	}
	httputil.JSON(w, http.StatusOK, f)
}

// Delete handles DELETE /projects/{projectID}/spans/{spanID}/feedback.
func (h *SpanFeedbackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	spanID := chi.URLParam(r, "spanID")

	if err := h.feedback.Delete(r.Context(), projectID, spanID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete feedback")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListByRun handles GET /projects/{projectID}/runs/{runID}/feedback.
// Returns all feedback for spans in a run in a single call (avoids N+1 from the span tree).
func (h *SpanFeedbackHandler) ListByRun(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	runID := chi.URLParam(r, "runID")

	feedbacks, err := h.feedback.ListByRun(r.Context(), projectID, runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list feedback")
		return
	}
	if feedbacks == nil {
		feedbacks = []*domain.SpanFeedback{}
	}
	httputil.JSON(w, http.StatusOK, feedbacks)
}
