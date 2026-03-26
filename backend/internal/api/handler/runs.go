package handler

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type RunHandler struct {
	runs     store.RunStore
	spans    store.SpanStore
	loops    store.LoopStore
	topology store.TopologyStore
	evals    store.EvalStore
}

func NewRunHandler(runs store.RunStore, spans store.SpanStore, loops store.LoopStore, topology store.TopologyStore, evals store.EvalStore) *RunHandler {
	return &RunHandler{runs: runs, spans: spans, loops: loops, topology: topology, evals: evals}
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
	total, err := h.runs.Count(r.Context(), projectID)
	if err != nil {
		total = 0 // non-fatal; frontend degrades gracefully
	}

	// Annotate runs with loop detection flag
	if len(runs) > 0 {
		runIDs := make([]string, len(runs))
		for i, run := range runs {
			runIDs[i] = run.RunID
		}
		loopMap, err := h.loops.HasLoops(r.Context(), runIDs)
		if err == nil {
			for _, run := range runs {
				run.LoopDetected = loopMap[run.RunID]
			}
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"runs":   runs,
		"limit":  limit,
		"offset": offset,
		"total":  total,
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

	// Annotate run with loop detection flag
	loopMap, err := h.loops.HasLoops(r.Context(), []string{runID})
	if err == nil {
		run.LoopDetected = loopMap[runID]
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

// Compare returns side-by-side data for two runs within the same project.
// Route: GET /api/v1/projects/{projectID}/runs/compare?a={runId}&b={runId}
func (h *RunHandler) Compare(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	runIDA := r.URL.Query().Get("a")
	runIDB := r.URL.Query().Get("b")

	if runIDA == "" || runIDB == "" {
		httputil.Error(w, http.StatusBadRequest, "query params 'a' and 'b' are required")
		return
	}
	if runIDA == runIDB {
		httputil.Error(w, http.StatusBadRequest, "query params 'a' and 'b' must be different run IDs")
		return
	}

	// Fetch both runs in parallel.
	runs, err := h.runs.GetMulti(r.Context(), []string{runIDA, runIDB})
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "one or both runs not found")
		return
	}
	runA, runB := runs[0], runs[1]

	if runA.ProjectID != projectID || runB.ProjectID != projectID {
		httputil.Error(w, http.StatusForbidden, "run does not belong to this project")
		return
	}

	// Fetch topology and evals for both runs in parallel.
	var (
		topologyA, topologyB *domain.Topology
		evalsA, evalsB       []*domain.SpanEval
		mu                   sync.Mutex
		fetchErr             error
	)

	type fetchFunc func()
	fetchers := []fetchFunc{
		func() {
			t, e := h.topology.GetByRun(r.Context(), runIDA)
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				topologyA = t
			}
		},
		func() {
			t, e := h.topology.GetByRun(r.Context(), runIDB)
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				topologyB = t
			}
		},
		func() {
			ev, e := h.evals.ListByRun(r.Context(), runIDA)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				fetchErr = e
			} else {
				evalsA = ev
			}
		},
		func() {
			ev, e := h.evals.ListByRun(r.Context(), runIDB)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				fetchErr = e
			} else {
				evalsB = ev
			}
		},
	}

	var wg sync.WaitGroup
	for _, fn := range fetchers {
		wg.Add(1)
		go func(f fetchFunc) {
			defer wg.Done()
			f()
		}(fn)
	}
	wg.Wait()

	if fetchErr != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to fetch comparison data")
		return
	}

	comparison := &domain.RunComparison{
		RunA:      runA,
		RunB:      runB,
		TopologyA: topologyA,
		TopologyB: topologyB,
		EvalsA:    evalsA,
		EvalsB:    evalsB,
	}

	httputil.JSON(w, http.StatusOK, comparison)
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
