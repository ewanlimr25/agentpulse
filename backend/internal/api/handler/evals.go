package handler

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// evalNameRe validates eval_type query param: alphanumeric, underscores, hyphens,
// and colons (to allow "custom:my-eval"). Max 64 chars.
var evalNameRe = regexp.MustCompile(`^[a-z0-9_:\-]{1,64}$`)

type EvalHandler struct {
	evals    store.EvalStore
	feedback store.SpanFeedbackStore
}

func NewEvalHandler(evals store.EvalStore, feedback store.SpanFeedbackStore) *EvalHandler {
	return &EvalHandler{evals: evals, feedback: feedback}
}

// applyFeedbackOverrides merges human ratings into LLM judge baseline scores.
// Rules:
//   - rating="bad"  → score is overridden to 0.0 regardless of judge score
//   - rating="good" → score is floored at 0.8 (max(judgeScore, 0.8))
//
// feedbackBySpan maps span_id → rating string ("good" or "bad").
// The function returns a new slice; it does not mutate the input.
func applyFeedbackOverrides(types []domain.EvalTypeBaseline, feedbackBySpan map[string]string) []domain.EvalTypeBaseline {
	// This is a project-level baseline — feedback overrides the overall avg score
	// by adjusting the OverallScore. Per-span overrides are not possible at this
	// aggregation level, so we compute an override factor from feedback counts.
	//
	// Approach: count good/bad feedback across all spans in the project.
	// - Each "bad" feedback record caps one span's contribution at 0.
	// - Each "good" feedback record floors one span's contribution at 0.8.
	// We apply this as a weighted adjustment to OverallScore.
	//
	// For simplicity in v1: apply the override directly to OverallScore using
	// the fraction of "bad" and "good" ratings in the feedback map.
	if len(feedbackBySpan) == 0 {
		return types
	}

	var goodCount, badCount int
	for _, rating := range feedbackBySpan {
		if rating == "bad" {
			badCount++
		} else if rating == "good" {
			goodCount++
		}
	}
	total := goodCount + badCount
	if total == 0 {
		return types
	}

	badFraction := float32(badCount) / float32(total)
	goodFraction := float32(goodCount) / float32(total)

	out := make([]domain.EvalTypeBaseline, len(types))
	for i, t := range types {
		adjusted := t.AvgScore
		// bad feedback pulls score down proportionally
		adjusted = adjusted*(1-badFraction) + 0.0*badFraction
		// good feedback floors at 0.8 proportionally
		floorContribution := float32(0.8) * goodFraction
		if adjusted < floorContribution {
			adjusted = floorContribution
		}
		out[i] = domain.EvalTypeBaseline{
			EvalName:  t.EvalName,
			AvgScore:  adjusted,
			SpanCount: t.SpanCount,
			RunCount:  t.RunCount,
		}
	}
	return out
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

// ListByRunGrouped returns evals for all spans in a run, grouped by (span_id, eval_name).
// Each group includes per-model scores, a consensus mean, and a disagreement flag.
// Route: GET /api/v1/runs/{runID}/evals/grouped
func (h *EvalHandler) ListByRunGrouped(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	groups, err := h.evals.ListByRunGrouped(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list grouped evals")
		return
	}
	// Always return an array, never null.
	if groups == nil {
		groups = []*domain.SpanEvalGroup{}
	}
	httputil.JSON(w, http.StatusOK, groups)
}

// BaselineByProject returns per-eval-type avg scores across the last N runs.
// Used by the agentpulse CLI quality gate check.
// Route: GET /api/v1/projects/{projectID}/evals/baseline
// Query params:
//
//	runs=N        number of recent runs to average over (default 10, max 100)
//	eval_type=X   filter to a single eval type (optional)
func (h *EvalHandler) BaselineByProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	// Parse and clamp ?runs=N.
	runs, _ := strconv.Atoi(r.URL.Query().Get("runs"))
	if runs < 1 {
		runs = 10
	}
	if runs > 100 {
		runs = 100
	}

	// Validate optional ?eval_type=X against a safe character set.
	evalType := r.URL.Query().Get("eval_type")
	if evalType != "" && !evalNameRe.MatchString(evalType) {
		httputil.Error(w, http.StatusBadRequest, "eval_type contains invalid characters; use lowercase letters, digits, underscores, hyphens, or colons")
		return
	}

	baseline, err := h.evals.BaselineByProject(r.Context(), projectID, runs)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to compute eval baseline")
		return
	}

	// Merge human feedback overrides into the baseline scores.
	feedbacks, feedbackErr := h.feedback.ListAllByProject(r.Context(), projectID)
	if feedbackErr != nil {
		// Fail open — log the error but return the unadjusted baseline.
		slog.Warn("failed to fetch human feedback for baseline", "project_id", projectID, "error", feedbackErr)
		feedbacks = nil
	}
	feedbackBySpan := make(map[string]string, len(feedbacks))
	for _, f := range feedbacks {
		feedbackBySpan[f.SpanID] = f.Rating
	}
	baseline.Types = applyFeedbackOverrides(baseline.Types, feedbackBySpan)
	// Recompute overall score as mean of adjusted per-type scores.
	if len(baseline.Types) > 0 {
		var sum float32
		for _, t := range baseline.Types {
			sum += t.AvgScore
		}
		baseline.OverallScore = sum / float32(len(baseline.Types))
	}

	// Filter to requested eval type if provided.
	if evalType != "" {
		found := false
		for _, t := range baseline.Types {
			if t.EvalName == evalType {
				baseline.Types = []domain.EvalTypeBaseline{t}
				baseline.OverallScore = t.AvgScore
				found = true
				break
			}
		}
		if !found {
			httputil.Error(w, http.StatusBadRequest, "no eval data found for type '"+evalType+"' — verify it is enabled for this project")
			return
		}
	}

	httputil.JSON(w, http.StatusOK, baseline)
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
