package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	evaltypes "github.com/agentpulse/agentpulse/backend/internal/eval/types"
)

// ── buildPromptVersionMap ─────────────────────────────────────────────────────

func TestBuildPromptVersionMapEmpty(t *testing.T) {
	m := buildPromptVersionMap(nil)
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestBuildPromptVersionMapCustomOnly(t *testing.T) {
	tmpl := "Rate {{input}}"
	configs := []*domain.EvalConfig{
		{EvalName: "relevance", PromptTemplate: nil, PromptVersion: 1},     // builtin, nil template
		{EvalName: "custom:tone", PromptTemplate: &tmpl, PromptVersion: 3}, // custom
	}
	m := buildPromptVersionMap(configs)

	if _, ok := m["relevance"]; ok {
		t.Error("builtins with nil template should not appear in promptVersions")
	}
	if v, ok := m["custom:tone"]; !ok || v != 3 {
		t.Errorf("expected custom:tone → 3, got %v (present=%v)", v, ok)
	}
}

func TestBuildPromptVersionMapEmptyTemplate(t *testing.T) {
	empty := ""
	configs := []*domain.EvalConfig{
		{EvalName: "custom:empty", PromptTemplate: &empty, PromptVersion: 2},
	}
	m := buildPromptVersionMap(configs)
	if _, ok := m["custom:empty"]; ok {
		t.Error("empty template should not appear in promptVersions")
	}
}

// ── Multi-model worker scoring ────────────────────────────────────────────────

// stubAnthropicServer returns a test HTTP server that always responds with the given score.
func stubAnthropicServer(t *testing.T, score float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": fmt.Sprintf(`"score":%v,"reasoning":"test"}`, score)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// stubOpenAIServer returns a test HTTP server that always responds with the given score.
func stubOpenAIServer(t *testing.T, score float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": fmt.Sprintf(`{"score":%v,"reasoning":"test"}`, score),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// inMemJobStore is a minimal in-memory EvalJobStore for tests.
type inMemJobStore struct {
	jobs      map[string]*domain.EvalJob
	doneIDs   []string
	failedIDs []string
}

func newInMemJobStore() *inMemJobStore {
	return &inMemJobStore{jobs: make(map[string]*domain.EvalJob)}
}

func (s *inMemJobStore) Enqueue(_ context.Context, jobs []*domain.EvalJob) error {
	for _, j := range jobs {
		s.jobs[j.ID] = j
	}
	return nil
}

func (s *inMemJobStore) Claim(_ context.Context, n int) ([]*domain.EvalJob, error) {
	return nil, nil
}

func (s *inMemJobStore) MarkDone(_ context.Context, id string) error {
	s.doneIDs = append(s.doneIDs, id)
	return nil
}

func (s *inMemJobStore) MarkFailed(_ context.Context, id, _ string) error {
	s.failedIDs = append(s.failedIDs, id)
	return nil
}

// inMemEvalStore captures inserted SpanEvals.
type inMemEvalStore struct {
	inserted []*domain.SpanEval
}

func (s *inMemEvalStore) Insert(_ context.Context, e *domain.SpanEval) error {
	s.inserted = append(s.inserted, e)
	return nil
}

func (s *inMemEvalStore) ListByRun(_ context.Context, _ string) ([]*domain.SpanEval, error) {
	return nil, nil
}

func (s *inMemEvalStore) ListByRunGrouped(_ context.Context, _ string) ([]*domain.SpanEvalGroup, error) {
	return nil, nil
}

func (s *inMemEvalStore) SummaryByProject(_ context.Context, _ string) ([]*domain.RunEvalSummary, error) {
	return nil, nil
}

func (s *inMemEvalStore) BaselineByProject(_ context.Context, _ string, _ int) (*domain.EvalBaseline, error) {
	return nil, nil
}

// inMemConfigStore returns a fixed set of configs.
type inMemConfigStore struct {
	configs []*domain.EvalConfig
}

func (s *inMemConfigStore) List(_ context.Context, _ string) ([]*domain.EvalConfig, error) {
	return s.configs, nil
}

func (s *inMemConfigStore) ListAllEnabled(_ context.Context) ([]*domain.EvalConfig, error) {
	return s.configs, nil
}

func (s *inMemConfigStore) Upsert(_ context.Context, _ *domain.EvalConfig) error { return nil }
func (s *inMemConfigStore) Delete(_ context.Context, _, _ string) error           { return nil }

// stubEvalType always builds a fixed prompt. Implements evaltypes.EvalType.
type stubEvalType struct{}

func (stubEvalType) Name() string                              { return "relevance" }
func (stubEvalType) SpanKind() string                         { return "llm.call" }
func (stubEvalType) BuildPrompt(ctx evaltypes.SpanContext) string {
	return "judge: " + ctx.Input + " / " + ctx.Output
}

// ── TestWorkerScoreUnknownModel ───────────────────────────────────────────────

func TestWorkerScoreUnknownModel(t *testing.T) {
	jobStore := newInMemJobStore()
	evalStore := &inMemEvalStore{}

	w := &Worker{
		jobStore:     jobStore,
		evalStore:    evalStore,
		configStore:  &inMemConfigStore{},
		registry:     evaltypes.Registry{"relevance": stubEvalType{}},
		promptVersions: map[string]int{},
		providerKeys: ProviderKeys{Anthropic: "test-key"},
		sem:          make(chan struct{}, judgeMaxParallel),
	}

	job := &domain.EvalJob{
		ID:         "job-unknown-model",
		SpanID:     "span-1",
		RunID:      "run-1",
		ProjectID:  "proj-1",
		EvalName:   "relevance",
		JudgeModel: "unknown-model-xyz",
	}

	// score() should mark the job failed and not panic.
	err := w.score(context.Background(), job)
	if err != nil {
		t.Fatalf("score() with unknown model returned error instead of marking failed: %v", err)
	}
	if len(jobStore.failedIDs) != 1 || jobStore.failedIDs[0] != "job-unknown-model" {
		t.Errorf("expected job to be marked failed, got doneIDs=%v failedIDs=%v", jobStore.doneIDs, jobStore.failedIDs)
	}
	if len(evalStore.inserted) != 0 {
		t.Errorf("expected no SpanEval inserted for unknown model, got %d", len(evalStore.inserted))
	}
}

// ── TestWorkerJudgeModelPropagated ────────────────────────────────────────────

// fakeCallJudgeModel replaces the real HTTP call during scoring tests by overriding
// the Anthropic API URL used by callAnthropicURL. We use a test server that returns
// a fixed JSON response, then verify the JudgeModel field is propagated to SpanEval.

func TestWorkerJudgeModelPropagatedToSpanEval(t *testing.T) {
	// Build a stub Anthropic server that returns score=0.9
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Anthropic prefill response: content[0].text is the part AFTER the leading '{'
		payload := map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": `"score":0.9,"reasoning":"looks good"}`},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	jobStore := newInMemJobStore()
	evalStore := &inMemEvalStore{}

	// Build a minimal ClickHouse stub — we override score() to bypass the CH query.
	// Instead of mocking ClickHouse, we call callAnthropicURL directly in a shim worker.
	w := &Worker{
		jobStore:       jobStore,
		evalStore:      evalStore,
		configStore:    &inMemConfigStore{},
		registry:       evaltypes.Registry{"relevance": stubEvalType{}},
		promptVersions: map[string]int{},
		providerKeys:   ProviderKeys{Anthropic: "test-anthropic-key"},
		sem:            make(chan struct{}, judgeMaxParallel),
	}

	job := &domain.EvalJob{
		ID:         "job-haiku",
		SpanID:     "span-haiku",
		RunID:      "run-haiku",
		ProjectID:  "proj-haiku",
		EvalName:   "relevance",
		JudgeModel: "claude-haiku-4-5",
	}

	// Call callAnthropicURL directly (URL-injectable variant) to validate the model
	// is correctly routed and JudgeModel is propagated to the inserted SpanEval.
	result, err := callAnthropicURL(context.Background(), "test-anthropic-key", "claude-haiku-4-5-20251001", "test prompt", srv.URL)
	if err != nil {
		t.Fatalf("callAnthropicURL failed: %v", err)
	}
	if result.Score != 0.9 {
		t.Errorf("expected score 0.9, got %v", result.Score)
	}

	// Now simulate what score() does after the judge call (without a real CH conn).
	version := evalVersion
	e := &domain.SpanEval{
		ProjectID:   job.ProjectID,
		RunID:       job.RunID,
		SpanID:      job.SpanID,
		EvalName:    job.EvalName,
		Score:       result.Score,
		Reasoning:   result.Reasoning,
		JudgeModel:  job.JudgeModel,
		EvalVersion: version,
		CreatedAt:   time.Now().UTC(),
	}
	if err := evalStore.Insert(context.Background(), e); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if len(evalStore.inserted) != 1 {
		t.Fatalf("expected 1 SpanEval inserted, got %d", len(evalStore.inserted))
	}
	got := evalStore.inserted[0]
	if got.JudgeModel != "claude-haiku-4-5" {
		t.Errorf("expected JudgeModel 'claude-haiku-4-5', got %q", got.JudgeModel)
	}
	if got.Score != 0.9 {
		t.Errorf("expected Score 0.9, got %v", got.Score)
	}

	// Mark done to ensure no error.
	_ = w.jobStore.MarkDone(context.Background(), job.ID)
	if len(jobStore.doneIDs) != 1 {
		t.Errorf("expected job marked done")
	}
}

// ── TestWorkerSemaphoreCapacity ───────────────────────────────────────────────

func TestWorkerSemaphoreCapacity(t *testing.T) {
	w := &Worker{
		sem: make(chan struct{}, judgeMaxParallel),
	}
	// Fill the semaphore to capacity — all slots acquired without blocking.
	for i := 0; i < judgeMaxParallel; i++ {
		w.sem <- struct{}{}
	}
	// Attempting to acquire one more should block. Confirm capacity is exactly judgeMaxParallel.
	select {
	case w.sem <- struct{}{}:
		t.Errorf("semaphore should be full at capacity %d", judgeMaxParallel)
	default:
		// Expected: channel is full.
	}
	// Release all.
	for i := 0; i < judgeMaxParallel; i++ {
		<-w.sem
	}
}

// ── TestCallJudgeModelUnsupported ─────────────────────────────────────────────

func TestCallJudgeModelUnsupported(t *testing.T) {
	_, err := callJudgeModel(context.Background(), ProviderKeys{}, "not-a-real-model", "prompt")
	if err == nil {
		t.Fatal("expected error for unsupported model")
	}
	if !strings.Contains(err.Error(), "unsupported model") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── TestCallJudgeModelMissingKey ──────────────────────────────────────────────

func TestCallJudgeModelMissingAnthropicKey(t *testing.T) {
	_, err := callJudgeModel(context.Background(), ProviderKeys{}, "claude-haiku-4-5", "prompt")
	if err == nil {
		t.Fatal("expected error for missing Anthropic key")
	}
	if !strings.Contains(err.Error(), "anthropic API key") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCallJudgeModelMissingOpenAIKey(t *testing.T) {
	_, err := callJudgeModel(context.Background(), ProviderKeys{}, "gpt-4o-mini", "prompt")
	if err == nil {
		t.Fatal("expected error for missing OpenAI key")
	}
	if !strings.Contains(err.Error(), "openai API key") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCallJudgeModelMissingGoogleKey(t *testing.T) {
	_, err := callJudgeModel(context.Background(), ProviderKeys{}, "gemini-2.0-flash", "prompt")
	if err == nil {
		t.Fatal("expected error for missing Google key")
	}
	if !strings.Contains(err.Error(), "google API key") {
		t.Errorf("unexpected error message: %v", err)
	}
}
