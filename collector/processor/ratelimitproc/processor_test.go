package ratelimitproc

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// makeTraces builds a ptrace.Traces with one ResourceSpan containing one span.
// If projectID is non-empty it is set as a resource attribute.
func makeTraces(projectID string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	if projectID != "" {
		rs.Resource().Attributes().PutStr(attrProjectID, projectID)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Spans().AppendEmpty().SetName("test-span")
	return td
}

func newTestProcessor(rate float64, burst int) *rateLimitProcessor {
	return newRateLimitProcessor(zap.NewNop(), &Config{
		RatePerSecond: rate,
		BurstSize:     burst,
		StaleAfter:    10 * time.Minute,
	})
}

// TestProcessTraces_BelowRatePassThrough verifies that batches within the burst
// limit are forwarded unchanged.
func TestProcessTraces_BelowRatePassThrough(t *testing.T) {
	proc := newTestProcessor(100, 10)

	for i := range 10 {
		td := makeTraces("proj-a")
		out, err := proc.ProcessTraces(context.Background(), td)
		if err != nil {
			t.Fatalf("batch %d: unexpected error: %v", i, err)
		}
		if out.ResourceSpans().Len() != 1 {
			t.Errorf("batch %d: expected 1 resource span, got %d", i, out.ResourceSpans().Len())
		}
	}
}

// TestProcessTraces_ExcessDropped verifies that batches exceeding the burst
// are dropped.
func TestProcessTraces_ExcessDropped(t *testing.T) {
	// rate=1/s, burst=2 — only the first 2 calls should pass.
	proc := newTestProcessor(1, 2)

	passed := 0
	dropped := 0
	for range 5 {
		td := makeTraces("proj-a")
		out, err := proc.ProcessTraces(context.Background(), td)
		if err != nil {
			t.Fatal(err)
		}
		if out.ResourceSpans().Len() == 1 {
			passed++
		} else {
			dropped++
		}
	}

	if passed != 2 {
		t.Errorf("expected 2 passed batches, got %d", passed)
	}
	if dropped != 3 {
		t.Errorf("expected 3 dropped batches, got %d", dropped)
	}
}

// TestProcessTraces_ProjectsAreIsolated verifies that exhausting one project's
// bucket does not affect another project.
func TestProcessTraces_ProjectsAreIsolated(t *testing.T) {
	proc := newTestProcessor(1, 1)

	// Exhaust project-a's bucket.
	tdA := makeTraces("proj-a")
	proc.ProcessTraces(context.Background(), tdA) // consumes the 1 token
	tdA2 := makeTraces("proj-a")
	outA, _ := proc.ProcessTraces(context.Background(), tdA2)
	if outA.ResourceSpans().Len() != 0 {
		t.Error("proj-a: expected batch to be dropped after exhausting bucket")
	}

	// project-b must still have its own full bucket.
	tdB := makeTraces("proj-b")
	outB, _ := proc.ProcessTraces(context.Background(), tdB)
	if outB.ResourceSpans().Len() != 1 {
		t.Error("proj-b: expected batch to pass through (independent bucket)")
	}
}

// TestProcessTraces_MissingProjectIDPassThrough verifies fail-open behaviour
// for spans without agentpulse.project_id.
func TestProcessTraces_MissingProjectIDPassThrough(t *testing.T) {
	// rate=0 would drop everything — but missing project ID must still pass.
	proc := newTestProcessor(0, 0)

	td := makeTraces("") // no project ID
	out, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 1 {
		t.Error("expected span with no project ID to pass through (fail-open)")
	}
}

// TestProcessTraces_SpanAttrFallback verifies that project_id on individual
// span attributes (not resource) is also detected.
func TestProcessTraces_SpanAttrFallback(t *testing.T) {
	proc := newTestProcessor(1, 1)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	// No resource-level project ID — set it on the span instead.
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.Attributes().PutStr(attrProjectID, "proj-span")

	out, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 1 {
		t.Error("expected span-level project_id to be detected and batch to pass")
	}
}

// ── Token bucket unit tests ───────────────────────────────────────────────────

func TestTokenBucket_BurstLimit(t *testing.T) {
	b := &tokenBucket{
		tokens:     3,
		lastRefill: time.Now(),
	}

	for i := range 3 {
		if !b.allow(0, 3) { // rate=0 so no refill; burst=3
			t.Errorf("call %d: expected allow=true", i)
		}
	}
	if b.allow(0, 3) {
		t.Error("4th call: expected allow=false (bucket empty)")
	}
}

func TestTokenBucket_TokenRefill(t *testing.T) {
	// Start with 0 tokens but a high rate — after a short sleep tokens refill.
	b := &tokenBucket{
		tokens:     0,
		lastRefill: time.Now().Add(-100 * time.Millisecond),
	}
	// At rate=100/s, 100ms = 10 tokens refilled.
	if !b.allow(100, 200) {
		t.Error("expected allow=true after refill")
	}
}

// TestBucketMap_StaleEviction verifies that idle buckets are evicted.
func TestBucketMap_StaleEviction(t *testing.T) {
	m := newBucketMap(100, 200, 50*time.Millisecond)

	// Create a bucket.
	m.allow("proj-stale")
	if m.len() != 1 {
		t.Fatalf("expected 1 bucket, got %d", m.len())
	}

	// Fast-forward: set lastRefill to the past so it looks stale.
	m.mu.RLock()
	b := m.buckets["proj-stale"]
	m.mu.RUnlock()
	b.mu.Lock()
	b.lastRefill = time.Now().Add(-200 * time.Millisecond)
	b.mu.Unlock()

	m.evictStale()

	if m.len() != 0 {
		t.Errorf("expected stale bucket to be evicted, got %d buckets", m.len())
	}
}

// TestBucketMap_StopNoleak verifies the eviction goroutine terminates cleanly.
func TestBucketMap_StopNoLeak(t *testing.T) {
	m := newBucketMap(100, 200, 100*time.Millisecond)
	m.start()

	// Give the goroutine time to start.
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		m.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("stop() did not return within 2s — possible goroutine leak")
	}
}

// ── Attribute helper ──────────────────────────────────────────────────────────

func makeResourceSpanWithAttr(key, value string, onResource bool) ptrace.ResourceSpans {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	if onResource {
		rs.Resource().Attributes().PutStr(key, value)
	} else {
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Spans().AppendEmpty().Attributes().PutStr(key, value)
	}
	return td.ResourceSpans().At(0)
}

func TestExtractProjectID_ResourceAttr(t *testing.T) {
	rs := makeResourceSpanWithAttr(attrProjectID, "proj-res", true)
	if got := extractProjectID(rs); got != "proj-res" {
		t.Errorf("expected proj-res, got %q", got)
	}
}

func TestExtractProjectID_SpanAttrFallback(t *testing.T) {
	rs := makeResourceSpanWithAttr(attrProjectID, "proj-span", false)
	if got := extractProjectID(rs); got != "proj-span" {
		t.Errorf("expected proj-span, got %q", got)
	}
}

func TestExtractProjectID_Missing(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("some.other.attr", "value")
	if got := extractProjectID(rs); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

var _ pcommon.Map // ensure pcommon import is used
