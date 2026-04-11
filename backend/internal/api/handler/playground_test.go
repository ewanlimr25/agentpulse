package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/handler"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/llmclient"
	"github.com/agentpulse/agentpulse/backend/internal/pricing"
)

// ---------------------------------------------------------------------------
// fakePlaygroundStore — in-memory PlaygroundStore for handler tests
// ---------------------------------------------------------------------------

type fakePlaygroundStore struct {
	sessions   map[string]*domain.PlaygroundSession
	executions map[string][]*domain.PlaygroundExecution // keyed by variantID
}

func newFakePlaygroundStore() *fakePlaygroundStore {
	return &fakePlaygroundStore{
		sessions:   make(map[string]*domain.PlaygroundSession),
		executions: make(map[string][]*domain.PlaygroundExecution),
	}
}

func (f *fakePlaygroundStore) CreateSession(_ context.Context, s *domain.PlaygroundSession) error {
	f.sessions[s.ID] = s
	return nil
}

func (f *fakePlaygroundStore) GetSession(_ context.Context, id string) (*domain.PlaygroundSession, error) {
	s, ok := f.sessions[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (f *fakePlaygroundStore) ListSessionsByProject(_ context.Context, projectID string, limit, offset int) ([]*domain.PlaygroundSession, error) {
	var result []*domain.PlaygroundSession
	for _, s := range f.sessions {
		if s.ProjectID == projectID {
			result = append(result, s)
		}
	}
	// Apply simple pagination.
	if offset >= len(result) {
		return nil, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

func (f *fakePlaygroundStore) CountSessionsByProject(_ context.Context, projectID string) (int, error) {
	count := 0
	for _, s := range f.sessions {
		if s.ProjectID == projectID {
			count++
		}
	}
	return count, nil
}

func (f *fakePlaygroundStore) DeleteSession(_ context.Context, id string) error {
	delete(f.sessions, id)
	return nil
}

func (f *fakePlaygroundStore) UpsertVariant(_ context.Context, v *domain.PlaygroundVariant) error {
	s, ok := f.sessions[v.SessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	for i, existing := range s.Variants {
		if existing.ID == v.ID {
			s.Variants[i] = v
			return nil
		}
	}
	s.Variants = append(s.Variants, v)
	return nil
}

func (f *fakePlaygroundStore) ListVariantsBySession(_ context.Context, sessionID string) ([]*domain.PlaygroundVariant, error) {
	s, ok := f.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	return s.Variants, nil
}

func (f *fakePlaygroundStore) RecordExecution(_ context.Context, e *domain.PlaygroundExecution) error {
	f.executions[e.VariantID] = append(f.executions[e.VariantID], e)
	return nil
}

func (f *fakePlaygroundStore) ListExecutionsByVariant(_ context.Context, variantID string, limit int) ([]*domain.PlaygroundExecution, error) {
	execs := f.executions[variantID]
	if len(execs) > limit {
		return execs[len(execs)-limit:], nil
	}
	return execs, nil
}

// ---------------------------------------------------------------------------
// fakeLLMClient — controllable LLM mock
// ---------------------------------------------------------------------------

type fakeLLMClient struct {
	response *llmclient.CompletionResponse
	err      error
}

func (f *fakeLLMClient) Complete(_ context.Context, _ llmclient.CompletionRequest) (*llmclient.CompletionResponse, error) {
	return f.response, f.err
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func testPricingTable() *pricing.Table {
	return &pricing.Table{
		Models: map[string]pricing.Model{
			"claude-3-5-sonnet": {
				Provider:         "anthropic",
				InputPerMillion:  3.0,
				OutputPerMillion: 15.0,
			},
			"gpt-4o": {
				Provider:         "openai",
				InputPerMillion:  5.0,
				OutputPerMillion: 15.0,
			},
		},
		Fallback: pricing.Model{
			InputPerMillion:  10.0,
			OutputPerMillion: 30.0,
		},
	}
}

func playgroundRouter(h *handler.PlaygroundHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/projects/{projectID}/playground", func(r chi.Router) {
		h.Routes(r)
	})
	return r
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	if env.Error != "" && target != nil {
		t.Fatalf("unexpected error in envelope: %s", env.Error)
	}
	if target != nil && env.Data != nil {
		if err := json.Unmarshal(env.Data, target); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}
	}
}

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}
	return env.Error
}

// ---------------------------------------------------------------------------
// CreateSession tests
// ---------------------------------------------------------------------------

func TestCreateSession_HappyPath(t *testing.T) {
	store := newFakePlaygroundStore()
	llm := &fakeLLMClient{}
	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	body := `{
		"name": "test session",
		"variants": [{
			"label": "baseline",
			"model_id": "claude-3-5-sonnet",
			"messages": [{"role": "user", "content": "hello"}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/playground/sessions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var session domain.PlaygroundSession
	decodeEnvelope(t, rec, &session)

	if session.ID == "" {
		t.Error("expected session ID to be set")
	}
	if session.Name != "test session" {
		t.Errorf("expected name 'test session', got %q", session.Name)
	}
	if session.ProjectID != "proj-1" {
		t.Errorf("expected projectID 'proj-1', got %q", session.ProjectID)
	}
	if len(session.Variants) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(session.Variants))
	}
	if session.Variants[0].Label != "baseline" {
		t.Errorf("expected variant label 'baseline', got %q", session.Variants[0].Label)
	}
}

func TestCreateSession_MissingName(t *testing.T) {
	store := newFakePlaygroundStore()
	llm := &fakeLLMClient{}
	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	body := `{
		"variants": [{
			"label": "baseline",
			"model_id": "claude-3-5-sonnet",
			"messages": [{"role": "user", "content": "hello"}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/playground/sessions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	errMsg := decodeErrorEnvelope(t, rec)
	if !strings.Contains(errMsg, "name") {
		t.Errorf("expected error about name, got %q", errMsg)
	}
}

// ---------------------------------------------------------------------------
// ListSessions test
// ---------------------------------------------------------------------------

func TestListSessions_Paginated(t *testing.T) {
	store := newFakePlaygroundStore()
	// Seed two sessions for proj-1.
	store.sessions["s1"] = &domain.PlaygroundSession{ID: "s1", ProjectID: "proj-1", Name: "first"}
	store.sessions["s2"] = &domain.PlaygroundSession{ID: "s2", ProjectID: "proj-1", Name: "second"}
	store.sessions["s3"] = &domain.PlaygroundSession{ID: "s3", ProjectID: "proj-2", Name: "other project"}

	llm := &fakeLLMClient{}
	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/projects/proj-1/playground/sessions?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Sessions []domain.PlaygroundSession `json:"sessions"`
		Total    int                        `json:"total"`
		Limit    int                        `json:"limit"`
		Offset   int                        `json:"offset"`
	}
	decodeEnvelope(t, rec, &result)

	if result.Total != 2 {
		t.Errorf("expected total 2, got %d", result.Total)
	}
	if len(result.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(result.Sessions))
	}
	if result.Limit != 10 {
		t.Errorf("expected limit 10, got %d", result.Limit)
	}
}

// ---------------------------------------------------------------------------
// GetSession tests
// ---------------------------------------------------------------------------

func TestGetSession_Found(t *testing.T) {
	store := newFakePlaygroundStore()
	store.sessions["s1"] = &domain.PlaygroundSession{
		ID:        "s1",
		ProjectID: "proj-1",
		Name:      "my session",
		Variants: []*domain.PlaygroundVariant{
			{ID: "v1", SessionID: "s1", Label: "A", ModelID: "claude-3-5-sonnet"},
		},
	}

	llm := &fakeLLMClient{}
	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/projects/proj-1/playground/sessions/s1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var session domain.PlaygroundSession
	decodeEnvelope(t, rec, &session)

	if session.Name != "my session" {
		t.Errorf("expected name 'my session', got %q", session.Name)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	store := newFakePlaygroundStore()
	llm := &fakeLLMClient{}
	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/projects/proj-1/playground/sessions/nonexistent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// RunVariant tests
// ---------------------------------------------------------------------------

func TestRunVariant_HappyPath(t *testing.T) {
	store := newFakePlaygroundStore()
	store.sessions["s1"] = &domain.PlaygroundSession{
		ID:        "s1",
		ProjectID: "proj-1",
		Name:      "test",
		Variants: []*domain.PlaygroundVariant{
			{
				ID:        "v1",
				SessionID: "s1",
				Label:     "baseline",
				ModelID:   "claude-3-5-sonnet",
				Messages:  []domain.PlaygroundMessage{{Role: "user", Content: "hello"}},
			},
		},
	}

	llm := &fakeLLMClient{
		response: &llmclient.CompletionResponse{
			Text:         "Hi there!",
			InputTokens:  10,
			OutputTokens: 5,
			FinishReason: "end_turn",
			LatencyMS:    200,
		},
	}

	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/playground/sessions/s1/variants/v1/run", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var exec domain.PlaygroundExecution
	decodeEnvelope(t, rec, &exec)

	if exec.ID == "" {
		t.Error("expected execution ID to be set")
	}
	if exec.Output == nil || *exec.Output != "Hi there!" {
		t.Errorf("expected output 'Hi there!', got %v", exec.Output)
	}
	if exec.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", exec.InputTokens)
	}
	if exec.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", exec.OutputTokens)
	}
	// Cost: 10 * 3.0/1M + 5 * 15.0/1M = 0.00003 + 0.000075 = 0.000105
	expectedCost := 0.000105
	if diff := exec.CostUSD - expectedCost; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected cost ~%g, got %g", expectedCost, exec.CostUSD)
	}
	if exec.LatencyMS != 200 {
		t.Errorf("expected latency 200, got %d", exec.LatencyMS)
	}

	// Verify execution was persisted.
	if len(store.executions["v1"]) != 1 {
		t.Errorf("expected 1 recorded execution, got %d", len(store.executions["v1"]))
	}
}

func TestRunVariant_APIKeyEmpty(t *testing.T) {
	store := newFakePlaygroundStore()
	store.sessions["s1"] = &domain.PlaygroundSession{
		ID:        "s1",
		ProjectID: "proj-1",
		Name:      "test",
		Variants: []*domain.PlaygroundVariant{
			{
				ID:        "v1",
				SessionID: "s1",
				Label:     "baseline",
				ModelID:   "claude-3-5-sonnet",
				Messages:  []domain.PlaygroundMessage{{Role: "user", Content: "hello"}},
			},
		},
	}

	llm := &fakeLLMClient{
		err: fmt.Errorf("llmclient: anthropic API key is empty"),
	}

	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/playground/sessions/s1/variants/v1/run", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRunVariant_PayloadTooLarge(t *testing.T) {
	store := newFakePlaygroundStore()

	// Build a message that exceeds 256 KiB when marshalled.
	bigContent := strings.Repeat("x", 300*1024)
	store.sessions["s1"] = &domain.PlaygroundSession{
		ID:        "s1",
		ProjectID: "proj-1",
		Name:      "test",
		Variants: []*domain.PlaygroundVariant{
			{
				ID:        "v1",
				SessionID: "s1",
				Label:     "baseline",
				ModelID:   "claude-3-5-sonnet",
				Messages:  []domain.PlaygroundMessage{{Role: "user", Content: bigContent}},
			},
		},
	}

	llm := &fakeLLMClient{}
	h := handler.NewPlaygroundHandler(store, llm, testPricingTable())
	r := playgroundRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/playground/sessions/s1/variants/v1/run", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ModelsHandler tests
// ---------------------------------------------------------------------------

func TestModelsList_Availability(t *testing.T) {
	pt := testPricingTable()
	keys := llmclient.ProviderKeys{
		Anthropic: "sk-ant-xxx",
		OpenAI:    "", // not configured
	}

	h := handler.NewModelsHandler(pt, keys)

	r := chi.NewRouter()
	r.Get("/models", h.List)

	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	type modelResp struct {
		ModelID   string `json:"model_id"`
		Provider  string `json:"provider"`
		Available bool   `json:"available"`
	}

	var models []modelResp
	decodeEnvelope(t, rec, &models)

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	// Models are sorted by model_id: claude-3-5-sonnet, gpt-4o
	for _, m := range models {
		switch m.ModelID {
		case "claude-3-5-sonnet":
			if !m.Available {
				t.Errorf("claude-3-5-sonnet should be available (anthropic key set)")
			}
		case "gpt-4o":
			if m.Available {
				t.Errorf("gpt-4o should not be available (openai key empty)")
			}
		default:
			t.Errorf("unexpected model %q", m.ModelID)
		}
	}
}
