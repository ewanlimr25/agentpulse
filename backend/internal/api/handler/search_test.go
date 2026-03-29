package handler

// Unit tests for SearchHandler.
//
// All external dependencies (store.SearchStore) are replaced with the
// mockSearchStore defined below. No ClickHouse connection is needed.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// mockSearchStore — implements store.SearchStore with controllable responses
// ---------------------------------------------------------------------------

type mockSearchStore struct {
	searchResult []*domain.SearchResult
	searchErr    error
	countResult  int
	countErr     error

	// Captured params so tests can assert what was passed to the store.
	capturedParams *domain.SearchParams
}

func (m *mockSearchStore) Search(_ context.Context, params *domain.SearchParams) ([]*domain.SearchResult, error) {
	m.capturedParams = params
	return m.searchResult, m.searchErr
}

func (m *mockSearchStore) SearchCount(_ context.Context, _ *domain.SearchParams) (int, error) {
	return m.countResult, m.countErr
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newSearchRequest creates an httptest.Request with the chi route context
// populated so that chi.URLParam(r, "projectID") works correctly.
func newSearchRequest(target, projectID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", projectID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// searchResponseEnvelope mirrors the httputil outer envelope.
type searchResponseEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

func decodeSearchEnvelope(t *testing.T, body []byte) searchResponseEnvelope {
	t.Helper()
	var env searchResponseEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to decode response envelope: %v — body: %s", err, body)
	}
	return env
}

// searchResponse is the shape of the data payload returned on success.
type searchResponse struct {
	Results []json.RawMessage `json:"results"`
	Total   int               `json:"total"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	Query   string            `json:"query"`
}

func decodeSearchResponse(t *testing.T, data json.RawMessage) searchResponse {
	t.Helper()
	var resp searchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to decode search response data: %v — raw: %s", err, data)
	}
	return resp
}

// drainSearchSem empties the searchSem channel so tests never block on the
// semaphore limit. Called in t.Cleanup so the global semaphore is always
// restored between tests.
func drainSearchSem() {
	for {
		select {
		case <-searchSem:
		default:
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Validation — query length
// ---------------------------------------------------------------------------

func TestSearchHandler_QTooShort_Returns400(t *testing.T) {
	ms := &mockSearchStore{}
	h := NewSearchHandler(ms)

	for _, q := range []string{"", "a", "ab"} {
		req := newSearchRequest("/api/v1/projects/p/search?q="+q, "p")
		w := httptest.NewRecorder()

		h.Search(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("q=%q: expected 400, got %d", q, w.Code)
		}
		env := decodeSearchEnvelope(t, w.Body.Bytes())
		if env.Error == "" {
			t.Errorf("q=%q: expected non-empty error field", q)
		}
	}
}

func TestSearchHandler_QExactly3Chars_PassesValidation(t *testing.T) {
	ms := &mockSearchStore{searchResult: []*domain.SearchResult{}, countResult: 0}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for q with exactly 3 chars, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Clamping — limit and offset
// ---------------------------------------------------------------------------

func TestSearchHandler_LimitClamped_AboveMax(t *testing.T) {
	ms := &mockSearchStore{searchResult: []*domain.SearchResult{}, countResult: 0}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc&limit=200", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	env := decodeSearchEnvelope(t, w.Body.Bytes())
	resp := decodeSearchResponse(t, env.Data)

	if resp.Limit != 50 {
		t.Errorf("expected limit clamped to 50, got %d", resp.Limit)
	}
	// Verify the clamped value was passed to the store.
	if ms.capturedParams != nil && ms.capturedParams.Limit != 50 {
		t.Errorf("expected store called with limit=50 (clamped), got %d", ms.capturedParams.Limit)
	}
}

func TestSearchHandler_OffsetClamped_AboveMax(t *testing.T) {
	ms := &mockSearchStore{searchResult: []*domain.SearchResult{}, countResult: 0}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc&offset=999", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	env := decodeSearchEnvelope(t, w.Body.Bytes())
	resp := decodeSearchResponse(t, env.Data)

	if resp.Offset != 500 {
		t.Errorf("expected offset clamped to 500, got %d", resp.Offset)
	}
	if ms.capturedParams != nil && ms.capturedParams.Offset != 500 {
		t.Errorf("expected store called with offset=500 (clamped), got %d", ms.capturedParams.Offset)
	}
}

// ---------------------------------------------------------------------------
// span_kind validation
// ---------------------------------------------------------------------------

func TestSearchHandler_UnknownSpanKind_Returns400(t *testing.T) {
	ms := &mockSearchStore{}
	h := NewSearchHandler(ms)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc&span_kind=not.a.kind", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown span_kind, got %d", w.Code)
	}
	env := decodeSearchEnvelope(t, w.Body.Bytes())
	if env.Error == "" {
		t.Error("expected non-empty error for unknown span_kind")
	}
}

func TestSearchHandler_ValidSpanKinds_PassThrough(t *testing.T) {
	validKinds := []string{
		"llm.call",
		"tool.call",
		"agent.handoff",
		"memory.read",
		"memory.write",
		"unknown",
	}

	for _, kind := range validKinds {
		t.Run(kind, func(t *testing.T) {
			ms := &mockSearchStore{searchResult: []*domain.SearchResult{}, countResult: 0}
			h := NewSearchHandler(ms)
			t.Cleanup(drainSearchSem)

			req := newSearchRequest("/api/v1/projects/p/search?q=abc&span_kind="+kind, "p")
			w := httptest.NewRecorder()

			h.Search(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("span_kind=%q: expected 200, got %d — body: %s", kind, w.Code, w.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Store error paths
// ---------------------------------------------------------------------------

func TestSearchHandler_SearchStoreError_Returns500(t *testing.T) {
	ms := &mockSearchStore{
		searchErr:   errors.New("clickhouse unavailable"),
		countResult: 0,
	}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on search store error, got %d", w.Code)
	}
	env := decodeSearchEnvelope(t, w.Body.Bytes())
	if env.Error == "" {
		t.Error("expected non-empty error field in 500 response")
	}
}

// ---------------------------------------------------------------------------
// Successful request
// ---------------------------------------------------------------------------

func TestSearchHandler_SuccessfulRequest_Returns200WithCorrectShape(t *testing.T) {
	results := []*domain.SearchResult{
		{
			TraceID:      "trace-1",
			SpanID:       "span-1",
			RunID:        "run-1",
			ProjectID:    "proj-1",
			SpanName:     "llm_call",
			MatchedField: "gen_ai.prompt",
			Snippet:      "...the rate limit...",
		},
	}
	ms := &mockSearchStore{searchResult: results, countResult: 42}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/proj-1/search?q=rate+limit", "proj-1")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	env := decodeSearchEnvelope(t, w.Body.Bytes())
	if env.Error != "" {
		t.Errorf("expected no error field, got %q", env.Error)
	}

	resp := decodeSearchResponse(t, env.Data)

	if resp.Total != 42 {
		t.Errorf("expected total=42, got %d", resp.Total)
	}
	if resp.Query != "rate limit" {
		t.Errorf("expected query='rate limit', got %q", resp.Query)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Limit != 20 {
		t.Errorf("expected default limit=20, got %d", resp.Limit)
	}
	if resp.Offset != 0 {
		t.Errorf("expected default offset=0, got %d", resp.Offset)
	}
}

func TestSearchHandler_EmptyResults_ReturnsEmptyArray(t *testing.T) {
	// When the store returns nil, the response must have results: [] not null.
	ms := &mockSearchStore{searchResult: nil, countResult: 0}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := decodeSearchEnvelope(t, w.Body.Bytes())
	resp := decodeSearchResponse(t, env.Data)

	if resp.Results == nil {
		t.Error("expected results to be an empty array, not null")
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

// ---------------------------------------------------------------------------
// Count error does not fail the request — total falls back to 0
// ---------------------------------------------------------------------------

func TestSearchHandler_CountStoreError_StillReturns200WithTotalZero(t *testing.T) {
	ms := &mockSearchStore{
		searchResult: []*domain.SearchResult{},
		countErr:     errors.New("count failed"),
	}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=abc", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	// The handler treats a count error as a non-fatal degraded response.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even on count error, got %d — body: %s", w.Code, w.Body.String())
	}

	env := decodeSearchEnvelope(t, w.Body.Bytes())
	resp := decodeSearchResponse(t, env.Data)

	if resp.Total != 0 {
		t.Errorf("expected total=0 when count errors, got %d", resp.Total)
	}
}

// ---------------------------------------------------------------------------
// Query is echo'd back in the response
// ---------------------------------------------------------------------------

func TestSearchHandler_QueryEchoedInResponse(t *testing.T) {
	ms := &mockSearchStore{searchResult: []*domain.SearchResult{}, countResult: 0}
	h := NewSearchHandler(ms)
	t.Cleanup(drainSearchSem)

	req := newSearchRequest("/api/v1/projects/p/search?q=hello+world", "p")
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := decodeSearchEnvelope(t, w.Body.Bytes())
	resp := decodeSearchResponse(t, env.Data)

	if resp.Query != "hello world" {
		t.Errorf("expected query='hello world', got %q", resp.Query)
	}
}
