package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/eval"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// ---------------------------------------------------------------------------
// mockEvalConfigStore — implements store.EvalConfigStore with controllable behaviour
// ---------------------------------------------------------------------------

type mockEvalConfigStore struct {
	listResult  []*domain.EvalConfig
	listErr     error
	upsertErr   error
	deleteErr   error
	upsertCalls int
}

func (m *mockEvalConfigStore) List(_ context.Context, _ string) ([]*domain.EvalConfig, error) {
	return m.listResult, m.listErr
}

func (m *mockEvalConfigStore) ListAllEnabled(_ context.Context) ([]*domain.EvalConfig, error) {
	return m.listResult, m.listErr
}

func (m *mockEvalConfigStore) Upsert(_ context.Context, _ *domain.EvalConfig) error {
	m.upsertCalls++
	return m.upsertErr
}

func (m *mockEvalConfigStore) Delete(_ context.Context, _, _ string) error {
	return m.deleteErr
}

// Compile-time check.
var _ store.EvalConfigStore = (*mockEvalConfigStore)(nil)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newEvalConfigRequest builds an HTTP POST request with a JSON body and the
// chi route context set for {projectID}.
func newEvalConfigRequest(t *testing.T, body interface{}, projectID string) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/evals/config", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", projectID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// evalConfigBody is a convenience struct for constructing request bodies in tests.
type evalConfigBody struct {
	EvalName    string   `json:"eval_name"`
	Enabled     bool     `json:"enabled"`
	SpanKind    string   `json:"span_kind"`
	JudgeModels []string `json:"judge_models,omitempty"`
}

// ---------------------------------------------------------------------------
// validateJudgeModels — direct unit tests (method on handler struct)
// ---------------------------------------------------------------------------

// TestValidateJudgeModels exercises the validateJudgeModels method directly
// without going through HTTP. This keeps the test independent of the upsert
// handler's other validation concerns.
func TestValidateJudgeModels(t *testing.T) {
	allKeysHandler := NewEvalConfigHandlerWithKeys(
		&mockEvalConfigStore{},
		eval.ProviderKeys{
			Anthropic: "ant-key",
			OpenAI:    "oai-key",
			Google:    "ggl-key",
		},
	)
	noOpenAIHandler := NewEvalConfigHandlerWithKeys(
		&mockEvalConfigStore{},
		eval.ProviderKeys{
			Anthropic: "ant-key",
			// OpenAI intentionally empty
			Google: "ggl-key",
		},
	)

	tests := []struct {
		name        string
		handler     *EvalConfigHandler
		models      []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "single valid anthropic model",
			handler: allKeysHandler,
			models:  []string{"claude-haiku-4-5"},
			wantErr: false,
		},
		{
			name:    "two valid models from different providers",
			handler: allKeysHandler,
			models:  []string{"claude-haiku-4-5", "gpt-4o-mini"},
			wantErr: false,
		},
		{
			name:    "all three supported models",
			handler: allKeysHandler,
			models:  []string{"claude-haiku-4-5", "gpt-4o-mini", "gemini-2.0-flash"},
			wantErr: false,
		},
		{
			name:        "unknown model name returns error",
			handler:     allKeysHandler,
			models:      []string{"unknown-model"},
			wantErr:     true,
			errContains: "unsupported judge model",
		},
		{
			name:        "mix of valid and invalid models returns error on invalid",
			handler:     allKeysHandler,
			models:      []string{"claude-haiku-4-5", "not-a-real-model"},
			wantErr:     true,
			errContains: "unsupported judge model",
		},
		{
			name:        "openai model with missing openai key returns error",
			handler:     noOpenAIHandler,
			models:      []string{"gpt-4o-mini"},
			wantErr:     true,
			errContains: "OPENAI_API_KEY",
		},
		{
			name:    "anthropic model with missing openai key still succeeds",
			handler: noOpenAIHandler,
			models:  []string{"claude-haiku-4-5"},
			wantErr: false,
		},
		{
			name:    "google model with all keys present succeeds",
			handler: allKeysHandler,
			models:  []string{"gemini-2.0-flash"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.handler.validateJudgeModels(tc.models)
			if tc.wantErr {
				if msg == "" {
					t.Errorf("expected validation error, got empty string")
				}
				if tc.errContains != "" && msg != "" {
					if len(msg) == 0 || !containsSubstring(msg, tc.errContains) {
						t.Errorf("expected error to contain %q, got: %q", tc.errContains, msg)
					}
				}
			} else {
				if msg != "" {
					t.Errorf("expected no error, got: %q", msg)
				}
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// EvalConfigHandler.upsert — HTTP-level tests via httptest
// ---------------------------------------------------------------------------

// TestUpsertEvalConfig_ValidSingleModel verifies a well-formed request with
// a single known judge model returns 200 and calls the store.
func TestUpsertEvalConfig_ValidSingleModel(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{Anthropic: "key"})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "relevance",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"claude-haiku-4-5"},
	}, "proj-1")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	if ms.upsertCalls != 1 {
		t.Errorf("expected 1 store upsert call, got %d", ms.upsertCalls)
	}
}

// TestUpsertEvalConfig_ValidTwoModels verifies two models from different
// providers are accepted when both provider keys are present.
func TestUpsertEvalConfig_ValidTwoModels(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{
		Anthropic: "ant-key",
		OpenAI:    "oai-key",
	})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "hallucination",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"claude-haiku-4-5", "gpt-4o-mini"},
	}, "proj-2")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	if ms.upsertCalls != 1 {
		t.Errorf("expected 1 store upsert call, got %d", ms.upsertCalls)
	}
}

// TestUpsertEvalConfig_UnknownModelReturns422 verifies that an unknown judge
// model name causes the handler to return 422 Unprocessable Entity.
func TestUpsertEvalConfig_UnknownModelReturns422(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{Anthropic: "key"})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "relevance",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"unknown-model"},
	}, "proj-3")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d — body: %s", w.Code, w.Body.String())
	}
	if ms.upsertCalls != 0 {
		t.Errorf("expected no store upsert on invalid model, got %d calls", ms.upsertCalls)
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Error == "" {
		t.Error("expected non-empty error field in 422 response")
	}
}

// TestUpsertEvalConfig_EmptyJudgeModels verifies that omitting judge_models
// (empty list) is accepted — the handler skips model validation and lets the
// store apply defaults.
func TestUpsertEvalConfig_EmptyJudgeModels(t *testing.T) {
	ms := &mockEvalConfigStore{}
	// No provider keys — if an empty list triggered validation, this would fail.
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "relevance",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{}, // explicitly empty
	}, "proj-4")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	// An empty slice skips validateJudgeModels and the request should succeed.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty judge_models, got %d — body: %s", w.Code, w.Body.String())
	}
	if ms.upsertCalls != 1 {
		t.Errorf("expected 1 store upsert call, got %d", ms.upsertCalls)
	}
}

// TestUpsertEvalConfig_NilJudgeModels verifies that omitting the judge_models
// field entirely (nil in Go, absent in JSON) is treated the same as empty.
func TestUpsertEvalConfig_NilJudgeModels(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{})

	// Omit judge_models from the body entirely.
	body := map[string]interface{}{
		"eval_name": "toxicity",
		"enabled":   true,
		"span_kind": "llm.call",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-5/evals/config", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "proj-5")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when judge_models absent, got %d — body: %s", w.Code, w.Body.String())
	}
}

// TestUpsertEvalConfig_MissingProviderKeyReturns422 verifies that providing an
// OpenAI model when the OpenAI key is empty returns 422.
func TestUpsertEvalConfig_MissingProviderKeyReturns422(t *testing.T) {
	ms := &mockEvalConfigStore{}
	// OpenAI key intentionally missing.
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{Anthropic: "ant-key"})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "relevance",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"gpt-4o-mini"},
	}, "proj-6")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing provider key, got %d — body: %s", w.Code, w.Body.String())
	}
	if ms.upsertCalls != 0 {
		t.Errorf("expected no store upsert on missing key, got %d calls", ms.upsertCalls)
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if !containsSubstring(env.Error, "OPENAI_API_KEY") {
		t.Errorf("expected error to mention OPENAI_API_KEY, got: %q", env.Error)
	}
}

// TestUpsertEvalConfig_MissingAnthropicKeyReturns422 mirrors the OpenAI test for Anthropic.
func TestUpsertEvalConfig_MissingAnthropicKeyReturns422(t *testing.T) {
	ms := &mockEvalConfigStore{}
	// Anthropic key intentionally missing.
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{OpenAI: "oai-key"})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "relevance",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"claude-haiku-4-5"},
	}, "proj-7")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing Anthropic key, got %d — body: %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if !containsSubstring(env.Error, "ANTHROPIC_API_KEY") {
		t.Errorf("expected error to mention ANTHROPIC_API_KEY, got: %q", env.Error)
	}
}

// TestUpsertEvalConfig_MissingGoogleKeyReturns422 mirrors the pattern for Google.
func TestUpsertEvalConfig_MissingGoogleKeyReturns422(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{Anthropic: "ant-key"})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "toxicity",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"gemini-2.0-flash"},
	}, "proj-8")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing Google key, got %d — body: %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if !containsSubstring(env.Error, "GOOGLE_AI_API_KEY") {
		t.Errorf("expected error to mention GOOGLE_AI_API_KEY, got: %q", env.Error)
	}
}

// TestUpsertEvalConfig_ResponseContainsJudgeModels verifies that the returned
// EvalConfig carries the judge_models field as submitted.
func TestUpsertEvalConfig_ResponseContainsJudgeModels(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{
		Anthropic: "ant-key",
		OpenAI:    "oai-key",
	})

	req := newEvalConfigRequest(t, evalConfigBody{
		EvalName:    "faithfulness",
		Enabled:     true,
		SpanKind:    "llm.call",
		JudgeModels: []string{"claude-haiku-4-5", "gpt-4o-mini"},
	}, "proj-9")
	w := httptest.NewRecorder()
	h.upsert(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	env := decodeEnvelope(t, w.Body.Bytes())
	var cfg domain.EvalConfig
	if err := json.Unmarshal(env.Data, &cfg); err != nil {
		t.Fatalf("decode EvalConfig: %v", err)
	}
	if len(cfg.JudgeModels) != 2 {
		t.Errorf("expected 2 judge_models in response, got %d: %v", len(cfg.JudgeModels), cfg.JudgeModels)
	}
	foundHaiku := false
	foundGPT := false
	for _, m := range cfg.JudgeModels {
		if m == "claude-haiku-4-5" {
			foundHaiku = true
		}
		if m == "gpt-4o-mini" {
			foundGPT = true
		}
	}
	if !foundHaiku || !foundGPT {
		t.Errorf("expected both models in response, got: %v", cfg.JudgeModels)
	}
}

// TestUpsertEvalConfig_BasicFieldValidation verifies that the handler still
// enforces the base validate() checks (eval_name, span_kind) independent of
// judge_models.
func TestUpsertEvalConfig_BasicFieldValidation(t *testing.T) {
	ms := &mockEvalConfigStore{}
	h := NewEvalConfigHandlerWithKeys(ms, eval.ProviderKeys{Anthropic: "key"})

	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode int
	}{
		{
			name: "missing eval_name",
			body: map[string]interface{}{
				"eval_name": "",
				"enabled":   true,
				"span_kind": "llm.call",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid span_kind",
			body: map[string]interface{}{
				"eval_name": "relevance",
				"enabled":   true,
				"span_kind": "invalid.kind",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "valid builtin eval with tool.call span_kind",
			body: map[string]interface{}{
				"eval_name": "tool_correctness",
				"enabled":   true,
				"span_kind": "tool.call",
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj-val/evals/config", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("projectID", "proj-val")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.upsert(w, req)

			if w.Code != tc.wantCode {
				t.Errorf("expected %d, got %d — body: %s", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}
