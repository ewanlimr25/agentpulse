package handler_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/handler"
	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	chstore "github.com/agentpulse/agentpulse/backend/internal/store/clickhouse"
)

// ---------------------------------------------------------------------------
// Mock stores
// ---------------------------------------------------------------------------

type fakeRunStore struct {
	run            *domain.Run
	getErr         error
	projectsByRun  map[string]string
	getProjectErr  error
}

func (f *fakeRunStore) List(_ context.Context, _ string, _, _ int) ([]*domain.Run, error) {
	return nil, nil
}
func (f *fakeRunStore) Count(_ context.Context, _ string) (int, error) { return 0, nil }
func (f *fakeRunStore) Get(_ context.Context, _ string) (*domain.Run, error) {
	return f.run, f.getErr
}
func (f *fakeRunStore) GetMulti(_ context.Context, _ []string) ([]*domain.Run, error) {
	return nil, nil
}
func (f *fakeRunStore) ListBySession(_ context.Context, _, _ string) ([]*domain.Run, error) {
	return nil, nil
}
func (f *fakeRunStore) GetProjectID(_ context.Context, runID string) (string, error) {
	if f.getProjectErr != nil {
		return "", f.getProjectErr
	}
	pid, ok := f.projectsByRun[runID]
	if !ok {
		return "", fmt.Errorf("run not found: %s", runID)
	}
	return pid, nil
}

type fakeSpanStore struct {
	spans []*domain.Span
	err   error
}

func (f *fakeSpanStore) ListByRun(_ context.Context, _ string) ([]*domain.Span, error) {
	return f.spans, f.err
}
func (f *fakeSpanStore) GetByID(_ context.Context, _, _ string) (*domain.Span, error) {
	return nil, chstore.ErrSpanNotFound
}

type fakeTopologyStore struct {
	topology *domain.Topology
	err      error
}

func (f *fakeTopologyStore) GetByRun(_ context.Context, _ string) (*domain.Topology, error) {
	return f.topology, f.err
}

type fakeLoopStore struct{}

func (f *fakeLoopStore) Upsert(_ context.Context, _ *domain.RunLoop) error { return nil }
func (f *fakeLoopStore) ListByRun(_ context.Context, _ string) ([]*domain.RunLoop, error) {
	return nil, nil
}
func (f *fakeLoopStore) HasLoops(_ context.Context, _ []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (f *fakeLoopStore) CountByProject(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

type fakeEvalStore struct{}

func (f *fakeEvalStore) Insert(_ context.Context, _ *domain.SpanEval) error { return nil }
func (f *fakeEvalStore) ListByRun(_ context.Context, _ string) ([]*domain.SpanEval, error) {
	return nil, nil
}
func (f *fakeEvalStore) ListByRunGrouped(_ context.Context, _ string) ([]*domain.SpanEvalGroup, error) {
	return nil, nil
}
func (f *fakeEvalStore) SummaryByProject(_ context.Context, _ string) ([]*domain.RunEvalSummary, error) {
	return nil, nil
}
func (f *fakeEvalStore) BaselineByProject(_ context.Context, _ string, _ int) (*domain.EvalBaseline, error) {
	return nil, nil
}

// fakePayloadStore returns a fixed JSON blob for any key it knows about.
type fakePayloadStore struct {
	data map[string][]byte
}

func (f *fakePayloadStore) Get(_ context.Context, key string) ([]byte, error) {
	if b, ok := f.data[key]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("payload not found: %s", key)
}

// fakeProjectStore mirrors the middleware tests' mockProjectStore.
type fakeProjectStore struct {
	projects map[string]*domain.Project
}

func (m *fakeProjectStore) List(_ context.Context) ([]*domain.Project, error) { return nil, nil }
func (m *fakeProjectStore) Get(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *fakeProjectStore) Create(_ context.Context, _ *domain.Project) error { return nil }
func (m *fakeProjectStore) GetByAPIKeyHash(_ context.Context, hash string) (*domain.Project, error) {
	p, ok := m.projects[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}
func (m *fakeProjectStore) GetByAdminKeyHash(_ context.Context, _ string) (*domain.Project, error) {
	return nil, fmt.Errorf("not found")
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newReplayRequest builds a request with chi URL params injected so the
// handler can be invoked directly without setting up the full router.
func newReplayRequest(runID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID+"/replay-bundle", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", runID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// ---------------------------------------------------------------------------
// ReplayBundle — happy path
// ---------------------------------------------------------------------------

func TestReplayBundle_ResolvesPayloadsAndComputesCallIndex(t *testing.T) {
	now := time.Now().UTC()

	// Two spans by the same agent, both calling tool "search". The first has
	// inline attributes; the second has its prompt/completion offloaded to S3.
	span1 := &domain.Span{
		SpanID:    "span-1",
		RunID:     "run-1",
		ProjectID: "proj-1",
		AgentName: "researcher",
		SpanName:  "search",
		StartTime: now,
		Attributes: map[string]string{
			"gen_ai.prompt":     "first inline prompt",
			"gen_ai.completion": "first inline completion",
		},
	}
	span2 := &domain.Span{
		SpanID:       "span-2",
		RunID:        "run-1",
		ProjectID:    "proj-1",
		AgentName:    "researcher",
		SpanName:     "search",
		StartTime:    now.Add(time.Second),
		Attributes:   map[string]string{}, // initially empty
		PayloadS3Key: "proj-1/run-1/span-2.json",
	}

	payloadJSON := []byte(`{"gen_ai.prompt":"second prompt from s3","gen_ai.completion":"second completion from s3"}`)
	payloads := &fakePayloadStore{data: map[string][]byte{
		"proj-1/run-1/span-2.json": payloadJSON,
	}}

	runs := &fakeRunStore{run: &domain.Run{RunID: "run-1", ProjectID: "proj-1"}}
	spans := &fakeSpanStore{spans: []*domain.Span{span1, span2}}
	topo := &fakeTopologyStore{topology: &domain.Topology{RunID: "run-1"}}

	h := handler.NewRunHandler(runs, spans, &fakeLoopStore{}, topo, &fakeEvalStore{}, payloads)

	rr := httptest.NewRecorder()
	h.ReplayBundle(rr, newReplayRequest("run-1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var env struct {
		Data struct {
			SchemaVersion int `json:"schema_version"`
			Spans         []struct {
				SpanID     string            `json:"SpanID"`
				CallIndex  int               `json:"call_index"`
				Attributes map[string]string `json:"Attributes"`
			} `json:"spans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v — body=%s", err, rr.Body.String())
	}

	if env.Data.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion=1, got %d", env.Data.SchemaVersion)
	}
	if len(env.Data.Spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(env.Data.Spans))
	}

	// CallIndex must increment for repeat (agent_name, span_name).
	if env.Data.Spans[0].CallIndex != 0 {
		t.Errorf("span[0] CallIndex=%d, want 0", env.Data.Spans[0].CallIndex)
	}
	if env.Data.Spans[1].CallIndex != 1 {
		t.Errorf("span[1] CallIndex=%d, want 1", env.Data.Spans[1].CallIndex)
	}

	// span[0] keeps inline attrs.
	if got := env.Data.Spans[0].Attributes["gen_ai.prompt"]; got != "first inline prompt" {
		t.Errorf("span[0] gen_ai.prompt=%q, want %q", got, "first inline prompt")
	}

	// span[1] should have its offloaded payload merged inline.
	if got := env.Data.Spans[1].Attributes["gen_ai.prompt"]; got != "second prompt from s3" {
		t.Errorf("span[1] gen_ai.prompt=%q, want resolved S3 value", got)
	}
	if got := env.Data.Spans[1].Attributes["gen_ai.completion"]; got != "second completion from s3" {
		t.Errorf("span[1] gen_ai.completion=%q, want resolved S3 value", got)
	}
}

// ---------------------------------------------------------------------------
// IDOR — token for project B cannot fetch a replay bundle for a run in
// project A. Mirrors TestRunAuth_RunBelongsToDifferentProject from the
// middleware test suite.
// ---------------------------------------------------------------------------

func TestReplayBundle_IDOR_WrongProjectToken(t *testing.T) {
	// run-A belongs to proj-A; the caller's token belongs to proj-B.
	ps := &fakeProjectStore{
		projects: map[string]*domain.Project{
			hashToken("token-B"): {ID: "proj-B", Name: "Project B"},
		},
	}
	rs := &fakeRunStore{
		projectsByRun: map[string]string{"run-A": "proj-A"},
	}

	runHandler := handler.NewRunHandler(
		rs,
		&fakeSpanStore{},
		&fakeLoopStore{},
		&fakeTopologyStore{},
		&fakeEvalStore{},
		nil,
	)

	r := chi.NewRouter()
	r.Route("/api/v1/runs/{runID}", func(r chi.Router) {
		r.Use(middleware.RunAuth(ps, rs))
		r.Get("/replay-bundle", runHandler.ReplayBundle)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-A/replay-bundle", nil)
	req.Header.Set("Authorization", "Bearer token-B")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// RunAuth rejects cross-project access with 403 — same as the
	// existing middleware IDOR test. The handler must never run.
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-project access, got %d: %s", rr.Code, rr.Body.String())
	}
}
