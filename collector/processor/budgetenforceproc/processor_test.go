package budgetenforceproc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

func makeSpan(projectID, runID, agentName string, costUSD float64) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(attrProjectID, projectID)
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test.span")
	span.Attributes().PutStr(attrProjectID, projectID)
	span.Attributes().PutStr(attrRunID, runID)
	if agentName != "" {
		span.Attributes().PutStr(attrAgentName, agentName)
	}
	if costUSD > 0 {
		span.Attributes().PutDouble(attrCostUSD, costUSD)
	}
	return td
}

func noopStore() *budgetStore {
	// nil pool — we inject rules directly in tests; store.writeAlert is tested separately.
	return &budgetStore{pool: nil}
}

func newTestProcessor(rules []budgetRule) *budgetProcessor {
	p := &budgetProcessor{
		logger: zap.NewNop(),
		cfg:    defaultConfig(),
		store:  noopStore(),
		acc:    newAccumulator(),
		dedup:  newAlertDedup(),
		http:   &http.Client{Timeout: 2 * time.Second},
		stopCh: make(chan struct{}),
		rules:  rules,
	}
	return p
}

// ── Accumulator tests ─────────────────────────────────────────────────────────

func TestAccumulator_Add(t *testing.T) {
	acc := newAccumulator()
	k := costKey{projectID: "p1", runID: "r1"}
	if got := acc.add(k, 0.01); got != 0.01 {
		t.Fatalf("want 0.01 got %v", got)
	}
	if got := acc.add(k, 0.02); got != 0.03 {
		t.Fatalf("want 0.03 got %v", got)
	}
}

func TestAccumulator_ResetRun(t *testing.T) {
	acc := newAccumulator()
	acc.add(costKey{projectID: "p1", runID: "r1"}, 1.0)
	acc.add(costKey{projectID: "p1", runID: "r1", agentName: "agent-a"}, 0.5)
	acc.add(costKey{projectID: "p1", runID: "r2"}, 2.0) // different run — must survive

	acc.resetRun("p1", "r1")

	if got := acc.get(costKey{projectID: "p1", runID: "r1"}); got != 0 {
		t.Fatalf("expected r1 reset, got %v", got)
	}
	if got := acc.get(costKey{projectID: "p1", runID: "r2"}); got != 2.0 {
		t.Fatalf("r2 should be untouched, got %v", got)
	}
}

// ── Alert dedup tests ─────────────────────────────────────────────────────────

func TestAlertDedup(t *testing.T) {
	d := newAlertDedup()
	if !d.check("rule1", "run1") {
		t.Fatal("first check should be true")
	}
	if d.check("rule1", "run1") {
		t.Fatal("second check should be false (already seen)")
	}
	if !d.check("rule1", "run2") {
		t.Fatal("different run should be fresh")
	}
	if !d.check("rule2", "run1") {
		t.Fatal("different rule should be fresh")
	}
}

// ── ProcessTraces: no rules ───────────────────────────────────────────────────

func TestProcessTraces_NoRules_PassThrough(t *testing.T) {
	p := newTestProcessor(nil)
	td := makeSpan("proj1", "run1", "agent-a", 0.05)
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.SpanCount() != 1 {
		t.Fatalf("expected 1 span, got %d", out.SpanCount())
	}
}

// ── ProcessTraces: span below threshold ───────────────────────────────────────

func TestProcessTraces_BelowThreshold_NoAlert(t *testing.T) {
	fired := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired = true
		w.WriteHeader(200)
	}))
	defer srv.Close()

	url := srv.URL
	rules := []budgetRule{{
		id: "r1", projectID: "proj1", name: "test", thresholdUSD: 1.0,
		action: actionNotify, scope: scopeRun, webhookURL: &url, enabled: true,
	}}
	p := newTestProcessor(rules)
	// Send a span with cost = $0.01 — well below $1.00 threshold.
	td := makeSpan("proj1", "run1", "", 0.01)
	_, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	// Give async goroutine a moment just in case.
	time.Sleep(50 * time.Millisecond)
	if fired {
		t.Fatal("webhook should not have fired below threshold")
	}
}

// ── ProcessTraces: threshold breached fires webhook ───────────────────────────

func TestProcessTraces_ThresholdBreached_WebhookFired(t *testing.T) {
	var mu sync.Mutex
	var received webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p webhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		received = p
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	url := srv.URL
	rules := []budgetRule{{
		id: "rule-1", projectID: "proj1", name: "cheap-rule", thresholdUSD: 0.05,
		action: actionNotify, scope: scopeRun, webhookURL: &url, enabled: true,
	}}
	p := newTestProcessor(rules)

	// $0.10 > $0.05 threshold — should fire.
	td := makeSpan("proj1", "run1", "", 0.10)
	_, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	ruleID, action, projectID := received.RuleID, received.Action, received.ProjectID
	mu.Unlock()

	if ruleID != "rule-1" {
		t.Fatalf("expected rule-1, got %q", ruleID)
	}
	if action != "notify" {
		t.Fatalf("expected notify, got %q", action)
	}
	if projectID != "proj1" {
		t.Fatalf("expected proj1, got %q", projectID)
	}
}

// ── ProcessTraces: dedup prevents double-firing ───────────────────────────────

func TestProcessTraces_Dedup_FiresOnce(t *testing.T) {
	var mu sync.Mutex
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	url := srv.URL
	rules := []budgetRule{{
		id: "r1", projectID: "proj1", name: "test", thresholdUSD: 0.01,
		action: actionNotify, scope: scopeRun, webhookURL: &url, enabled: true,
	}}
	p := newTestProcessor(rules)

	// Two batches both over threshold — alert should only fire once.
	for i := 0; i < 2; i++ {
		td := makeSpan("proj1", "run1", "", 0.05)
		if _, err := p.ProcessTraces(context.Background(), td); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 webhook call, got %d", got)
	}
}

// ── ProcessTraces: halt stamps attribute on spans ─────────────────────────────

func TestProcessTraces_HaltStampsAttribute(t *testing.T) {
	rules := []budgetRule{{
		id: "r-halt", projectID: "proj1", name: "halt-rule", thresholdUSD: 0.01,
		action: actionHalt, scope: scopeRun, enabled: true,
	}}
	p := newTestProcessor(rules)

	td := makeSpan("proj1", "run1", "", 0.10)
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	// Allow async goroutine to finish stamping (it's goroutine-based).
	// The stamp happens synchronously in ProcessTraces — no wait needed.
	span := out.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, ok := span.Attributes().Get(attrBudgetHalted)
	if !ok || !v.Bool() {
		t.Fatal("expected agentpulse.budget.halted=true on halted span")
	}
}

// ── ProcessTraces: agent-scoped rule ─────────────────────────────────────────

func TestProcessTraces_AgentScoped(t *testing.T) {
	var mu sync.Mutex
	var received webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p webhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		received = p
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	url := srv.URL
	rules := []budgetRule{{
		id: "r-agent", projectID: "proj1", name: "agent-rule", thresholdUSD: 0.05,
		action: actionNotify, scope: scopeAgent, webhookURL: &url, enabled: true,
	}}
	p := newTestProcessor(rules)

	// agent-a costs $0.10 — above threshold.
	td := makeSpan("proj1", "run1", "agent-a", 0.10)
	_, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	ruleID := received.RuleID
	mu.Unlock()

	if ruleID != "r-agent" {
		t.Fatalf("agent-scoped rule should have fired, got %q", ruleID)
	}
}

// ── ProcessTraces: wrong project skipped ─────────────────────────────────────

func TestProcessTraces_WrongProject_NotFired(t *testing.T) {
	fired := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fired = true
		w.WriteHeader(200)
	}))
	defer srv.Close()

	url := srv.URL
	rules := []budgetRule{{
		id: "r1", projectID: "proj-OTHER", name: "test", thresholdUSD: 0.01,
		action: actionNotify, scope: scopeRun, webhookURL: &url, enabled: true,
	}}
	p := newTestProcessor(rules)

	td := makeSpan("proj1", "run1", "", 0.50)
	if _, err := p.ProcessTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if fired {
		t.Fatal("rule for different project should not fire")
	}
}

// ── ProcessTraces: spans with no cost are ignored ────────────────────────────

func TestProcessTraces_ZeroCost_Ignored(t *testing.T) {
	rules := []budgetRule{{
		id: "r1", projectID: "proj1", name: "test", thresholdUSD: 0.001,
		action: actionNotify, scope: scopeRun, enabled: true,
	}}
	p := newTestProcessor(rules)

	// Span has no cost attribute.
	td := makeSpan("proj1", "run1", "", 0)
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.SpanCount() != 1 {
		t.Fatal("span should pass through unchanged")
	}
	if p.acc.get(costKey{projectID: "proj1", runID: "run1"}) != 0 {
		t.Fatal("zero-cost span should not accumulate")
	}
}

// ── ProcessTraces: multi-span batch accumulation ─────────────────────────────

func TestProcessTraces_MultiSpanBatch(t *testing.T) {
	var mu sync.Mutex
	var received webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p webhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		received = p
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	url := srv.URL
	rules := []budgetRule{{
		id: "r1", projectID: "proj1", name: "test", thresholdUSD: 0.25,
		action: actionNotify, scope: scopeRun, webhookURL: &url, enabled: true,
	}}
	p := newTestProcessor(rules)

	// Build a single Traces payload with 3 spans totalling $0.30.
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	for _, cost := range []float64{0.05, 0.10, 0.15} {
		span := ss.Spans().AppendEmpty()
		span.Attributes().PutStr(attrProjectID, "proj1")
		span.Attributes().PutStr(attrRunID, "run-multi")
		span.Attributes().PutDouble(attrCostUSD, cost)
	}

	if _, err := p.ProcessTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	ruleID, costUSD := received.RuleID, received.CostUSD
	mu.Unlock()

	if ruleID != "r1" {
		t.Fatalf("multi-span batch should have triggered rule, got %q", ruleID)
	}
	if costUSD < 0.25 {
		t.Fatalf("reported cost should be >= 0.25, got %v", costUSD)
	}
}

// ── getDouble helper ──────────────────────────────────────────────────────────

func TestGetDouble(t *testing.T) {
	m := pcommon.NewMap()
	m.PutDouble("d", 1.5)
	m.PutInt("i", 3)
	m.PutStr("s", "x")

	if got := getDouble(m, "d"); got != 1.5 {
		t.Fatalf("want 1.5 got %v", got)
	}
	if got := getDouble(m, "i"); got != 3.0 {
		t.Fatalf("want 3.0 got %v", got)
	}
	if got := getDouble(m, "s"); got != 0 {
		t.Fatalf("string key should return 0, got %v", got)
	}
	if got := getDouble(m, "missing"); got != 0 {
		t.Fatalf("missing key should return 0, got %v", got)
	}
}
