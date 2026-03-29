package handler

import (
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
