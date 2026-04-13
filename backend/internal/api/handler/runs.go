package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
	chstore "github.com/agentpulse/agentpulse/backend/internal/store/clickhouse"
)

type RunHandler struct {
	runs        store.RunStore
	spans       store.SpanStore
	loops       store.LoopStore
	topology    store.TopologyStore
	evals       store.EvalStore
	payloads    store.PayloadStore // nullable — nil means S3 disabled
	tags        store.RunTagStore
	annotations store.RunAnnotationStore
}

func NewRunHandler(runs store.RunStore, spans store.SpanStore, loops store.LoopStore, topology store.TopologyStore, evals store.EvalStore, payloads store.PayloadStore, tags store.RunTagStore, annotations store.RunAnnotationStore) *RunHandler {
	return &RunHandler{runs: runs, spans: spans, loops: loops, topology: topology, evals: evals, payloads: payloads, tags: tags, annotations: annotations}
}

func (h *RunHandler) Routes(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/{runID}", h.Get)
	r.Get("/{runID}/spans", h.ListSpans)
}

// maxTagFilterIDs caps the number of run IDs collected when filtering by tag to
// prevent excessively large IN-list queries against ClickHouse.
const maxTagFilterIDs = 500

// List returns paginated runs for a project.
// Route: GET /api/v1/projects/{projectID}/runs
//
// Optional query params:
//   - tag (repeatable) — filter to runs that have ALL specified tags
func (h *RunHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 50)
	offset := intQueryParam(r, "offset", 0)

	// ── Tag-based filtering ───────────────────────────────────────────────────
	// Collect all ?tag= values and resolve the intersection of run ID sets.
	filterTags := r.URL.Query()["tag"]
	var filteredRunIDs []string
	truncated := false

	if len(filterTags) > 0 && h.tags != nil {
		// For each tag, fetch the matching run IDs and intersect.
		// We fetch up to maxTagFilterIDs+1 to detect truncation.
		const fetchLimit = maxTagFilterIDs + 1

		var intersection map[string]struct{}
		for _, tag := range filterTags {
			ids, err := h.tags.ListRuns(r.Context(), projectID, tag, fetchLimit, 0)
			if err != nil {
				httputil.Error(w, http.StatusInternalServerError, "failed to filter by tag")
				return
			}
			set := make(map[string]struct{}, len(ids))
			for _, id := range ids {
				set[id] = struct{}{}
			}
			if intersection == nil {
				intersection = set
			} else {
				for id := range intersection {
					if _, ok := set[id]; !ok {
						delete(intersection, id)
					}
				}
			}
		}

		filteredRunIDs = make([]string, 0, len(intersection))
		for id := range intersection {
			filteredRunIDs = append(filteredRunIDs, id)
		}
		if len(filteredRunIDs) > maxTagFilterIDs {
			filteredRunIDs = filteredRunIDs[:maxTagFilterIDs]
			truncated = true
		}
	}

	var runs []*domain.Run
	var total int
	var err error

	if len(filterTags) > 0 {
		// When tag filters are active, fetch only matching runs.
		if len(filteredRunIDs) == 0 {
			runs = []*domain.Run{}
			total = 0
		} else {
			runs, err = h.runs.GetMulti(r.Context(), filteredRunIDs)
			if err != nil {
				httputil.Error(w, http.StatusInternalServerError, "failed to list runs")
				return
			}
			total = len(filteredRunIDs)
		}
	} else {
		runs, err = h.runs.List(r.Context(), projectID, limit, offset)
		if err != nil {
			httputil.Error(w, http.StatusInternalServerError, "failed to list runs")
			return
		}
		total, err = h.runs.Count(r.Context(), projectID)
		if err != nil {
			total = 0 // non-fatal; frontend degrades gracefully
		}
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

		// Fetch tags and annotations for all runs in parallel.
		if h.tags != nil && h.annotations != nil {
			var (
				tagMap    map[string][]string
				annotMap  map[string]*domain.RunAnnotation
				tagErr    error
				annotErr  error
				wg        sync.WaitGroup
			)
			wg.Add(2)
			go func() {
				defer wg.Done()
				tagMap, tagErr = h.tags.ListByRuns(r.Context(), projectID, runIDs)
			}()
			go func() {
				defer wg.Done()
				annotMap, annotErr = h.annotations.GetByRuns(r.Context(), projectID, runIDs)
			}()
			wg.Wait()

			if tagErr == nil && annotErr == nil {
				for _, run := range runs {
					if tags, ok := tagMap[run.RunID]; ok {
						run.Tags = tags
					} else {
						run.Tags = []string{}
					}
					if a, ok := annotMap[run.RunID]; ok {
						run.Annotation = &a.Note
					}
				}
			}
		}
	}

	// Annotate runs with active status (span activity within last 30s)
	activeMap, err := h.runs.ListActiveRunIDs(r.Context(), projectID, 30)
	if err == nil {
		for _, run := range runs {
			run.IsActive = activeMap[run.RunID]
		}
	}

	if truncated {
		w.Header().Set("X-Truncated", "true")
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

	// Mark run as active if it ended recently (within last 30s)
	if !run.EndTime.IsZero() && time.Since(run.EndTime) < 30*time.Second {
		run.IsActive = true
	}

	// Fetch tags and annotation in parallel.
	if h.tags != nil && h.annotations != nil {
		var (
			tags     []string
			annot    *domain.RunAnnotation
			tagErr   error
			annotErr error
			wg       sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			tags, tagErr = h.tags.List(r.Context(), run.ProjectID, runID)
		}()
		go func() {
			defer wg.Done()
			annot, annotErr = h.annotations.Get(r.Context(), run.ProjectID, runID)
		}()
		wg.Wait()

		if tagErr == nil {
			if tags == nil {
				run.Tags = []string{}
			} else {
				run.Tags = tags
			}
		}
		if annotErr == nil && annot != nil {
			run.Annotation = &annot.Note
		}
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

// GetSpan returns a single span with payload fields resolved from S3 if offloaded.
// Route: GET /runs/{runID}/spans/{spanID}
func (h *RunHandler) GetSpan(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	spanID := chi.URLParam(r, "spanID")

	// Get authenticated project from context (set by RunAuth middleware).
	project, ok := middleware.ProjectFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	span, err := h.spans.GetByID(r.Context(), project.ID, spanID)
	if err != nil {
		if errors.Is(err, chstore.ErrSpanNotFound) {
			httputil.Error(w, http.StatusNotFound, "span not found")
			return
		}
		httputil.Error(w, http.StatusNotFound, "span not found")
		return
	}

	// Verify span belongs to this run (IDOR check).
	if span.RunID != runID {
		httputil.Error(w, http.StatusNotFound, "span not found")
		return
	}

	// Resolve S3 payloads (fail-open: no-op if payloads is nil or fetch fails).
	chstore.ResolvePayloads(r.Context(), span, h.payloads)

	httputil.JSON(w, http.StatusOK, span)
}

// replayBundleMaxBytes caps the size of a single replay bundle response.
// Larger bundles return 413; the CLI hint suggests --span-limit (future flag).
const replayBundleMaxBytes = 50 << 20 // 50 MiB

// replayPayloadResolveConcurrency bounds concurrent S3 fetches per request.
const replayPayloadResolveConcurrency = 8

// ReplayBundle returns a self-contained snapshot of a run that an SDK can
// load in "replay mode" to deterministically re-execute against the recorded
// tool/LLM responses.
//
// Route: GET /api/v1/runs/{runID}/replay-bundle
func (h *RunHandler) ReplayBundle(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	run, err := h.runs.Get(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "run not found")
		return
	}

	spans, err := h.spans.ListByRun(r.Context(), runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list spans")
		return
	}

	// Topology is optional — fail-open if it's missing or errors.
	topology, _ := h.topology.GetByRun(r.Context(), runID)

	// Resolve offloaded payloads in parallel with a bounded worker pool.
	// ResolvePayloads is fail-open and logs internally.
	if len(spans) > 0 {
		sem := make(chan struct{}, replayPayloadResolveConcurrency)
		var wg sync.WaitGroup
		for _, sp := range spans {
			if sp.PayloadS3Key == "" {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(span *domain.Span) {
				defer wg.Done()
				defer func() { <-sem }()
				chstore.ResolvePayloads(r.Context(), span, h.payloads)
			}(sp)
		}
		wg.Wait()
	}

	// Compute CallIndex per (agent_name, span_name) by walking spans in
	// start_time order. The store already returns spans ordered ASC by
	// start_time (see span_store.go listByRunQuery).
	replaySpans := make([]*domain.ReplaySpan, len(spans))
	counts := make(map[string]int, len(spans))
	for i, sp := range spans {
		key := sp.AgentName + "\x00" + sp.SpanName
		idx := counts[key]
		counts[key] = idx + 1
		replaySpans[i] = &domain.ReplaySpan{Span: sp, CallIndex: idx}
	}

	bundle := &domain.ReplayBundle{
		SchemaVersion: 1,
		Run:           run,
		Spans:         replaySpans,
		Topology:      topology,
	}

	// Encode to a buffer first so we can enforce the 50MB cap before
	// committing to a 200 OK response.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]any{"data": bundle}); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to encode replay bundle")
		return
	}
	if buf.Len() > replayBundleMaxBytes {
		httputil.Error(w, http.StatusRequestEntityTooLarge,
			"replay bundle exceeds 50MB limit; use --span-limit to reduce span count")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
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
