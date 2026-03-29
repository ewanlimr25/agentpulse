package handler

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// searchSem caps concurrent ClickHouse search queries to avoid overload.
var searchSem = make(chan struct{}, 3)

// SearchHandler handles full-text span search requests.
type SearchHandler struct {
	search store.SearchStore
}

func NewSearchHandler(search store.SearchStore) *SearchHandler {
	return &SearchHandler{search: search}
}

// Search handles GET /api/v1/projects/{projectID}/search
// Query params:
//   - q        (required, min 3 chars)
//   - span_kind (optional)
//   - from      (optional, RFC3339)
//   - to        (optional, RFC3339)
//   - limit     (default 20, max 50)
//   - offset    (default 0, max 500)
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	select {
	case searchSem <- struct{}{}:
		defer func() { <-searchSem }()
	default:
		httputil.Error(w, http.StatusTooManyRequests, "too many concurrent search requests")
		return
	}

	projectID := chi.URLParam(r, "projectID")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 3 {
		httputil.Error(w, http.StatusBadRequest, "query 'q' must be at least 3 characters")
		return
	}

	limit := intQueryParam(r, "limit", 20)
	if limit > 50 {
		limit = 50
	}
	offset := intQueryParam(r, "offset", 0)
	if offset > 500 {
		offset = 500
	}

	params := &domain.SearchParams{
		ProjectID: projectID,
		Query:     q,
		Limit:     limit,
		Offset:    offset,
	}

	if sk := r.URL.Query().Get("span_kind"); sk != "" {
		switch domain.AgentSpanKind(sk) {
		case domain.SpanKindLLMCall, domain.SpanKindToolCall, domain.SpanKindAgentHandoff,
			domain.SpanKindMemoryRead, domain.SpanKindMemoryWrite, domain.SpanKindUnknown:
			params.SpanKind = domain.AgentSpanKind(sk)
		default:
			httputil.Error(w, http.StatusBadRequest, "invalid span_kind; valid values: llm.call, tool.call, agent.handoff, memory.read, memory.write, unknown")
			return
		}
	}

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			params.From = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			params.To = t
		}
	}

	// Run Search and SearchCount in parallel.
	type searchResult struct {
		results []*domain.SearchResult
		err     error
	}
	type countResult struct {
		total int
		err   error
	}

	searchCh := make(chan searchResult, 1)
	countCh := make(chan countResult, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		results, err := h.search.Search(r.Context(), params)
		searchCh <- searchResult{results: results, err: err}
	}()

	go func() {
		defer wg.Done()
		total, err := h.search.SearchCount(r.Context(), params)
		countCh <- countResult{total: total, err: err}
	}()

	wg.Wait()
	close(searchCh)
	close(countCh)

	sr := <-searchCh
	cr := <-countCh

	if sr.err != nil {
		httputil.Error(w, http.StatusInternalServerError, "search query failed")
		return
	}

	total := 0
	if cr.err == nil {
		total = cr.total
	}

	// Return empty array rather than null when there are no results.
	results := sr.results
	if results == nil {
		results = []*domain.SearchResult{}
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
		"query":   q,
	})
}
