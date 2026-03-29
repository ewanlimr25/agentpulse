package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// mockEvalStore — implements store.EvalStore with controllable responses
// ---------------------------------------------------------------------------

type mockEvalStore struct {
	baseline     *domain.EvalBaseline
	baselineErr  error
	lastNRunsArg int // captured so tests can assert the clamped value

	listByRunResult []*domain.SpanEval
	listByRunErr    error

	summaryResult []*domain.RunEvalSummary
	summaryErr    error
}

func (m *mockEvalStore) Insert(_ context.Context, _ *domain.SpanEval) error {
	return nil
}

func (m *mockEvalStore) ListByRun(_ context.Context, _ string) ([]*domain.SpanEval, error) {
	return m.listByRunResult, m.listByRunErr
}

func (m *mockEvalStore) SummaryByProject(_ context.Context, _ string) ([]*domain.RunEvalSummary, error) {
	return m.summaryResult, m.summaryErr
}

func (m *mockEvalStore) BaselineByProject(_ context.Context, _ string, lastNRuns int) (*domain.EvalBaseline, error) {
	m.lastNRunsArg = lastNRuns
	return m.baseline, m.baselineErr
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newEvalRequest creates an httptest request with the chi route context set.
func newEvalRequest(method, target, projectID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", projectID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// responseEnvelope mirrors the httputil envelope for decoding.
type responseEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

func decodeEnvelope(t *testing.T, body []byte) responseEnvelope {
	t.Helper()
	var env responseEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to decode response envelope: %v — body: %s", err, body)
	}
	return env
}

// ---------------------------------------------------------------------------
// BaselineByProject — happy path
// ---------------------------------------------------------------------------

func TestBaselineByProject_ValidRequest(t *testing.T) {
	ms := &mockEvalStore{
		baseline: &domain.EvalBaseline{
			ProjectID:      "proj-123",
			RunsConsidered: 5,
			OverallScore:   0.82,
			Types: []domain.EvalTypeBaseline{
				{EvalName: "relevance", AvgScore: 0.82, SpanCount: 20, RunCount: 5},
			},
		},
	}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet, "/api/v1/projects/proj-123/evals/baseline?runs=5", "proj-123")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Error != "" {
		t.Errorf("expected no error field, got %q", env.Error)
	}

	var baseline domain.EvalBaseline
	if err := json.Unmarshal(env.Data, &baseline); err != nil {
		t.Fatalf("failed to decode baseline: %v", err)
	}
	if baseline.ProjectID != "proj-123" {
		t.Errorf("expected project_id='proj-123', got %q", baseline.ProjectID)
	}
	if baseline.RunsConsidered != 5 {
		t.Errorf("expected runs_considered=5, got %d", baseline.RunsConsidered)
	}
	if len(baseline.Types) != 1 || baseline.Types[0].EvalName != "relevance" {
		t.Errorf("unexpected types: %+v", baseline.Types)
	}
}

// ---------------------------------------------------------------------------
// BaselineByProject — invalid eval_type characters → 400
// ---------------------------------------------------------------------------

func TestBaselineByProject_InvalidEvalTypeChars(t *testing.T) {
	ms := &mockEvalStore{}
	h := NewEvalHandler(ms)

	// These values must be URL-safe (no raw spaces) but must fail the
	// evalNameRe regex (^[a-z0-9_:\-]{1,64}$).
	invalidTypes := []struct {
		name     string
		urlValue string // URL-encoded value delivered to the handler
	}{
		{"uppercase letters", "UPPER_CASE"},
		{"exclamation mark", "has%21bang"},    // "has!bang" URL-encoded
		{"at sign", "has%40at"},               // "has@at" URL-encoded
		{"too long (65 chars)", strings.Repeat("a", 65)},
		{"dot separator", "eval.name"},        // dot not in allowed set
	}

	for _, tc := range invalidTypes {
		req := newEvalRequest(http.MethodGet,
			"/api/v1/projects/proj-123/evals/baseline?eval_type="+tc.urlValue,
			"proj-123")
		w := httptest.NewRecorder()
		h.BaselineByProject(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("%s (eval_type=%q): expected 400, got %d", tc.name, tc.urlValue, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// BaselineByProject — valid regex but absent from data → 400
// ---------------------------------------------------------------------------

func TestBaselineByProject_EvalTypeNotFoundInData(t *testing.T) {
	ms := &mockEvalStore{
		baseline: &domain.EvalBaseline{
			ProjectID:      "proj-123",
			RunsConsidered: 5,
			OverallScore:   0.80,
			Types: []domain.EvalTypeBaseline{
				{EvalName: "relevance", AvgScore: 0.80, SpanCount: 10, RunCount: 5},
			},
		},
	}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet,
		"/api/v1/projects/proj-123/evals/baseline?eval_type=toxicity",
		"proj-123")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when eval_type not in data, got %d — body: %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if !strings.Contains(env.Error, "toxicity") {
		t.Errorf("expected error to mention 'toxicity', got: %q", env.Error)
	}
}

// ---------------------------------------------------------------------------
// BaselineByProject — ?runs clamping
// ---------------------------------------------------------------------------

func TestBaselineByProject_RunsClamped_AboveMax(t *testing.T) {
	ms := &mockEvalStore{baseline: &domain.EvalBaseline{ProjectID: "p"}}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet, "/api/v1/projects/p/evals/baseline?runs=200", "p")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ms.lastNRunsArg != 100 {
		t.Errorf("expected store called with lastNRuns=100 (clamped), got %d", ms.lastNRunsArg)
	}
}

func TestBaselineByProject_RunsClamped_BelowMin(t *testing.T) {
	ms := &mockEvalStore{baseline: &domain.EvalBaseline{ProjectID: "p"}}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet, "/api/v1/projects/p/evals/baseline?runs=0", "p")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ms.lastNRunsArg < 1 {
		t.Errorf("expected store called with lastNRuns>=1 (clamped from 0), got %d", ms.lastNRunsArg)
	}
}

func TestBaselineByProject_DefaultRuns_WhenParamMissing(t *testing.T) {
	ms := &mockEvalStore{baseline: &domain.EvalBaseline{ProjectID: "p"}}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet, "/api/v1/projects/p/evals/baseline", "p")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ms.lastNRunsArg != 10 {
		t.Errorf("expected default runs=10, store called with %d", ms.lastNRunsArg)
	}
}

// ---------------------------------------------------------------------------
// BaselineByProject — store error → 500
// ---------------------------------------------------------------------------

func TestBaselineByProject_StoreError_Returns500(t *testing.T) {
	ms := &mockEvalStore{baselineErr: errors.New("clickhouse unavailable")}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet, "/api/v1/projects/proj-123/evals/baseline", "proj-123")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on store error, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Error == "" {
		t.Error("expected non-empty error field in 500 response")
	}
}

// ---------------------------------------------------------------------------
// BaselineByProject — eval_type filter shapes the response
// ---------------------------------------------------------------------------

func TestBaselineByProject_EvalTypeFilter_ReturnsOnlyThatType(t *testing.T) {
	ms := &mockEvalStore{
		baseline: &domain.EvalBaseline{
			ProjectID:      "proj-xyz",
			RunsConsidered: 10,
			OverallScore:   0.72,
			Types: []domain.EvalTypeBaseline{
				{EvalName: "relevance", AvgScore: 0.80, SpanCount: 30, RunCount: 10},
				{EvalName: "hallucination", AvgScore: 0.64, SpanCount: 20, RunCount: 9},
			},
		},
	}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet,
		"/api/v1/projects/proj-xyz/evals/baseline?eval_type=hallucination",
		"proj-xyz")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	env := decodeEnvelope(t, w.Body.Bytes())
	var baseline domain.EvalBaseline
	if err := json.Unmarshal(env.Data, &baseline); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if len(baseline.Types) != 1 {
		t.Fatalf("expected exactly 1 type after filter, got %d", len(baseline.Types))
	}
	if baseline.Types[0].EvalName != "hallucination" {
		t.Errorf("expected 'hallucination', got %q", baseline.Types[0].EvalName)
	}
	// OverallScore must be updated to the filtered type's AvgScore.
	if baseline.OverallScore != 0.64 {
		t.Errorf("expected overall_score=0.64 after filter, got %.3f", baseline.OverallScore)
	}
}

func TestBaselineByProject_NoEvalTypeFilter_AllTypesReturned(t *testing.T) {
	ms := &mockEvalStore{
		baseline: &domain.EvalBaseline{
			ProjectID:      "proj-multi",
			RunsConsidered: 5,
			OverallScore:   0.70,
			Types: []domain.EvalTypeBaseline{
				{EvalName: "relevance", AvgScore: 0.80, SpanCount: 10, RunCount: 5},
				{EvalName: "toxicity", AvgScore: 0.60, SpanCount: 8, RunCount: 5},
			},
		},
	}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet, "/api/v1/projects/proj-multi/evals/baseline", "proj-multi")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	env := decodeEnvelope(t, w.Body.Bytes())
	var baseline domain.EvalBaseline
	if err := json.Unmarshal(env.Data, &baseline); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if len(baseline.Types) != 2 {
		t.Errorf("expected 2 types when no filter, got %d", len(baseline.Types))
	}
}

// ---------------------------------------------------------------------------
// BaselineByProject — valid custom eval_type format accepted
// ---------------------------------------------------------------------------

func TestBaselineByProject_ValidCustomEvalTypeFormat(t *testing.T) {
	// "custom:my-eval" must pass the regex and not return 400 on format alone.
	ms := &mockEvalStore{
		baseline: &domain.EvalBaseline{
			ProjectID:      "proj-custom",
			RunsConsidered: 3,
			OverallScore:   0.75,
			Types: []domain.EvalTypeBaseline{
				{EvalName: "custom:my-eval", AvgScore: 0.75, SpanCount: 5, RunCount: 3},
			},
		},
	}
	h := NewEvalHandler(ms)
	req := newEvalRequest(http.MethodGet,
		"/api/v1/projects/proj-custom/evals/baseline?eval_type=custom:my-eval",
		"proj-custom")
	w := httptest.NewRecorder()

	h.BaselineByProject(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid custom eval_type, got %d — body: %s", w.Code, w.Body.String())
	}
}
