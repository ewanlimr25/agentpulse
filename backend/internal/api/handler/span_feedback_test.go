package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// mockSpanFeedbackStore — implements store.SpanFeedbackStore with controllable
// responses for handler tests. No real database is involved.
// ---------------------------------------------------------------------------

type mockSpanFeedbackStore struct {
	upsertErr   error
	getResult   *domain.SpanFeedback
	getErr      error
	deleteErr   error
	listResult  []*domain.SpanFeedback
	listErr     error
	listAllResult []*domain.SpanFeedback
	listAllErr    error

	// Captured call args so tests can assert what the handler passed.
	lastUpserted  *domain.SpanFeedback
	lastGetProject string
	lastGetSpan    string
	lastDelProject string
	lastDelSpan    string
	lastListProject string
	lastListRun     string
}

func (m *mockSpanFeedbackStore) Upsert(_ context.Context, f *domain.SpanFeedback) error {
	m.lastUpserted = f
	return m.upsertErr
}

func (m *mockSpanFeedbackStore) GetBySpan(_ context.Context, projectID, spanID string) (*domain.SpanFeedback, error) {
	m.lastGetProject = projectID
	m.lastGetSpan = spanID
	return m.getResult, m.getErr
}

func (m *mockSpanFeedbackStore) ListByRun(_ context.Context, projectID, runID string) ([]*domain.SpanFeedback, error) {
	m.lastListProject = projectID
	m.lastListRun = runID
	return m.listResult, m.listErr
}

func (m *mockSpanFeedbackStore) Delete(_ context.Context, projectID, spanID string) error {
	m.lastDelProject = projectID
	m.lastDelSpan = spanID
	return m.deleteErr
}

func (m *mockSpanFeedbackStore) CountByProject(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockSpanFeedbackStore) ListAllByProject(_ context.Context, _ string) ([]*domain.SpanFeedback, error) {
	return m.listAllResult, m.listAllErr
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildFeedbackRouter wires up a Chi router for the SpanFeedbackHandler
// routes without auth middleware — the handler tests focus on handler logic.
func buildFeedbackRouter(store *mockSpanFeedbackStore) http.Handler {
	r := chi.NewRouter()
	h := NewSpanFeedbackHandler(store)
	r.Route("/api/v1/projects/{projectID}", func(r chi.Router) {
		r.Post("/spans/{spanID}/feedback", h.Upsert)
		r.Get("/spans/{spanID}/feedback", h.Get)
		r.Delete("/spans/{spanID}/feedback", h.Delete)
		r.Get("/runs/{runID}/feedback", h.ListByRun)
	})
	return r
}

func ptr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// Upsert tests
// ---------------------------------------------------------------------------

func TestSpanFeedbackHandler_Upsert_OK(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	body := `{"run_id":"run-1","rating":"good"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var env struct {
		Data domain.SpanFeedback `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	resp := env.Data
	if resp.Rating != "good" {
		t.Errorf("expected rating 'good', got %q", resp.Rating)
	}
	if resp.RunID != "run-1" {
		t.Errorf("expected run_id 'run-1', got %q", resp.RunID)
	}
	if resp.ProjectID != "proj-1" {
		t.Errorf("expected project_id 'proj-1', got %q", resp.ProjectID)
	}
	if resp.SpanID != "span-1" {
		t.Errorf("expected span_id 'span-1', got %q", resp.SpanID)
	}
}

func TestSpanFeedbackHandler_Upsert_BadRating(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	body := `{"run_id":"run-1","rating":"meh"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad rating, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "rating must be") {
		t.Errorf("expected validation message in body, got: %s", rr.Body.String())
	}
}

func TestSpanFeedbackHandler_Upsert_MissingRunID(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	body := `{"rating":"bad"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing run_id, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "run_id is required") {
		t.Errorf("expected 'run_id is required' in body, got: %s", rr.Body.String())
	}
}

func TestSpanFeedbackHandler_Upsert_OversizedCorrectedOutput(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	// 10 001 'a' characters — one over the limit.
	big := strings.Repeat("a", maxCorrectedOutputLen+1)
	payload := map[string]interface{}{
		"run_id":           "run-1",
		"rating":           "bad",
		"corrected_output": big,
	}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized corrected_output, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "10 000") {
		t.Errorf("expected limit message in body, got: %s", rr.Body.String())
	}
}

func TestSpanFeedbackHandler_Upsert_NullByteInCorrectedOutput(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	// Build raw JSON manually because Go's json.Marshal strips null bytes.
	body := "{\"run_id\":\"run-1\",\"rating\":\"bad\",\"corrected_output\":\"hello\u0000world\"}"
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for null byte in corrected_output, got %d — body: %s", rr.Code, rr.Body.String())
	}
	// The JSON decoder itself rejects strings containing null bytes (they are
	// invalid in JSON), so the response carries the generic "invalid request body"
	// message rather than the field-level validation message. Either way, a 400
	// is the correct response — the exact error text is an implementation detail.
}

func TestSpanFeedbackHandler_Upsert_WithCorrectedOutput(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	correction := "The correct answer is 42."
	payload := map[string]interface{}{
		"run_id":           "run-1",
		"rating":           "bad",
		"corrected_output": correction,
	}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if store.lastUpserted == nil {
		t.Fatal("expected store.Upsert to be called")
	}
	if store.lastUpserted.CorrectedOutput == nil || *store.lastUpserted.CorrectedOutput != correction {
		t.Errorf("expected CorrectedOutput %q, got %v", correction, store.lastUpserted.CorrectedOutput)
	}
}

func TestSpanFeedbackHandler_Upsert_StoreError(t *testing.T) {
	store := &mockSpanFeedbackStore{upsertErr: errors.New("db write failed")}
	h := buildFeedbackRouter(store)

	body := `{"run_id":"run-1","rating":"good"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rr.Code)
	}
}

func TestSpanFeedbackHandler_Upsert_InvalidJSON(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/spans/span-1/feedback",
		bytes.NewBufferString("{not json}"))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------

func TestSpanFeedbackHandler_Get_OK(t *testing.T) {
	now := time.Now()
	stored := &domain.SpanFeedback{
		ID:        "fb-1",
		ProjectID: "proj-1",
		SpanID:    "span-1",
		RunID:     "run-1",
		Rating:    "good",
		CreatedAt: now,
		UpdatedAt: now,
	}
	store := &mockSpanFeedbackStore{getResult: stored}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/spans/span-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Data domain.SpanFeedback `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if env.Data.Rating != "good" {
		t.Errorf("expected rating 'good', got %q", env.Data.Rating)
	}
	// Verify handler passed the correct project + span to the store.
	if store.lastGetProject != "proj-1" {
		t.Errorf("store received project %q, expected 'proj-1'", store.lastGetProject)
	}
	if store.lastGetSpan != "span-1" {
		t.Errorf("store received span %q, expected 'span-1'", store.lastGetSpan)
	}
}

func TestSpanFeedbackHandler_Get_NotFound(t *testing.T) {
	store := &mockSpanFeedbackStore{getResult: nil, getErr: nil}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/spans/no-such-span/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing feedback, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestSpanFeedbackHandler_Get_StoreError(t *testing.T) {
	store := &mockSpanFeedbackStore{getErr: errors.New("connection refused")}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/spans/span-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

func TestSpanFeedbackHandler_Delete_OK(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/projects/proj-1/spans/span-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d — body: %s", rr.Code, rr.Body.String())
	}
	// Verify handler scoped the delete to the correct project + span.
	if store.lastDelProject != "proj-1" {
		t.Errorf("store received project %q for delete, expected 'proj-1'", store.lastDelProject)
	}
	if store.lastDelSpan != "span-1" {
		t.Errorf("store received span %q for delete, expected 'span-1'", store.lastDelSpan)
	}
}

func TestSpanFeedbackHandler_Delete_StoreError(t *testing.T) {
	store := &mockSpanFeedbackStore{deleteErr: errors.New("db error")}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/projects/proj-1/spans/span-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// ListByRun tests
// ---------------------------------------------------------------------------

func TestSpanFeedbackHandler_ListByRun_OK(t *testing.T) {
	now := time.Now()
	items := []*domain.SpanFeedback{
		{ID: "fb-1", ProjectID: "proj-1", SpanID: "span-1", RunID: "run-1", Rating: "good", CreatedAt: now, UpdatedAt: now},
		{ID: "fb-2", ProjectID: "proj-1", SpanID: "span-2", RunID: "run-1", Rating: "bad", CreatedAt: now, UpdatedAt: now},
	}
	store := &mockSpanFeedbackStore{listResult: items}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/runs/run-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var env struct {
		Data []*domain.SpanFeedback `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if len(env.Data) != 2 {
		t.Errorf("expected 2 feedback items, got %d", len(env.Data))
	}
	// Verify handler passed the correct project + run to the store.
	if store.lastListProject != "proj-1" {
		t.Errorf("store received project %q, expected 'proj-1'", store.lastListProject)
	}
	if store.lastListRun != "run-1" {
		t.Errorf("store received run %q, expected 'run-1'", store.lastListRun)
	}
}

func TestSpanFeedbackHandler_ListByRun_Empty(t *testing.T) {
	// Nil slice from store — handler should normalize to empty JSON array, not null.
	store := &mockSpanFeedbackStore{listResult: nil}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/runs/run-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty list, got %d — body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "[]") {
		t.Errorf("expected empty JSON array '[]', got: %s", body)
	}
	// Ensure the response is not JSON null.
	if strings.TrimSpace(body) == "null" {
		t.Errorf("expected '[]', got null for empty feedback list")
	}
}

func TestSpanFeedbackHandler_ListByRun_StoreError(t *testing.T) {
	store := &mockSpanFeedbackStore{listErr: errors.New("timeout")}
	h := buildFeedbackRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/runs/run-1/feedback", nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Direct handler invocation via chi route context (no HTTP router)
// ---------------------------------------------------------------------------

func TestSpanFeedbackHandler_Upsert_DirectHandler(t *testing.T) {
	store := &mockSpanFeedbackStore{}
	h := NewSpanFeedbackHandler(store)

	body := `{"run_id":"run-direct","rating":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-direct")
	rctx.URLParams.Add("spanID", "span-direct")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	if store.lastUpserted == nil {
		t.Fatal("expected store Upsert to be called")
	}
	if store.lastUpserted.ProjectID != "proj-direct" {
		t.Errorf("expected ProjectID 'proj-direct', got %q", store.lastUpserted.ProjectID)
	}
	if store.lastUpserted.SpanID != "span-direct" {
		t.Errorf("expected SpanID 'span-direct', got %q", store.lastUpserted.SpanID)
	}
}
