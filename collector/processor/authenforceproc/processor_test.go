package authenforceproc

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

func makeTraces(projectID, rawToken string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	if projectID != "" {
		rs.Resource().Attributes().PutStr(attrProjectID, projectID)
	}
	if rawToken != "" {
		rs.Resource().Attributes().PutStr(attrIngestToken, rawToken)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test.span")
	return td
}

// newDisabledProcessor returns a processor with Enabled=false (no DB needed).
func newDisabledProcessor() *authEnforceProcessor {
	cfg := defaultConfig()
	cfg.Enabled = false
	return &authEnforceProcessor{cfg: cfg, store: nil, logger: zap.NewNop()}
}

// newFailOpenNoDBProcessor returns an enabled, fail-open processor with no DB connection.
func newFailOpenNoDBProcessor() *authEnforceProcessor {
	cfg := defaultConfig()
	cfg.Enabled = true
	cfg.FailOpen = true
	return &authEnforceProcessor{cfg: cfg, store: nil, logger: zap.NewNop()}
}

// newFailClosedNoDBProcessor returns an enabled, fail-closed processor with no DB connection.
func newFailClosedNoDBProcessor() *authEnforceProcessor {
	cfg := defaultConfig()
	cfg.Enabled = true
	cfg.FailOpen = false
	return &authEnforceProcessor{cfg: cfg, store: nil, logger: zap.NewNop()}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestDisabled: when Enabled=false, all spans pass through without any validation.
func TestDisabled_PassThrough(t *testing.T) {
	p := newDisabledProcessor()
	td := makeTraces("", "") // no token or project_id
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 1 {
		t.Fatalf("expected 1 ResourceSpans, got %d", out.ResourceSpans().Len())
	}
}

// TestFailOpen_MissingToken: missing token with fail-open passes spans through.
func TestFailOpen_MissingToken_PassThrough(t *testing.T) {
	p := newFailOpenNoDBProcessor()
	td := makeTraces("proj1", "") // no token
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 1 {
		t.Fatalf("expected 1 ResourceSpans (fail-open), got %d", out.ResourceSpans().Len())
	}
}

// TestFailClosed_MissingToken: missing token with fail-closed drops spans.
func TestFailClosed_MissingToken_Drops(t *testing.T) {
	p := newFailClosedNoDBProcessor()
	td := makeTraces("proj1", "") // no token
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 0 {
		t.Fatalf("expected 0 ResourceSpans (fail-closed, no token), got %d", out.ResourceSpans().Len())
	}
}

// TestFailOpen_NoDB: valid token+project_id but no DB connection passes through (fail-open).
func TestFailOpen_NoDB_PassThrough(t *testing.T) {
	p := newFailOpenNoDBProcessor()
	td := makeTraces("proj1", "some-token")
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 1 {
		t.Fatalf("expected 1 ResourceSpans (fail-open, no DB), got %d", out.ResourceSpans().Len())
	}
}

// TestFailClosed_NoDB: valid token+project_id but no DB connection drops spans (fail-closed).
func TestFailClosed_NoDB_Drops(t *testing.T) {
	p := newFailClosedNoDBProcessor()
	td := makeTraces("proj1", "some-token")
	out, err := p.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if out.ResourceSpans().Len() != 0 {
		t.Fatalf("expected 0 ResourceSpans (fail-closed, no DB), got %d", out.ResourceSpans().Len())
	}
}

// TestHashToken: verify hash is deterministic and matches standard SHA-256.
func TestHashToken_Deterministic(t *testing.T) {
	h1 := hashToken("my-secret-token")
	h2 := hashToken("my-secret-token")
	if h1 != h2 {
		t.Fatal("hashToken should be deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char hex SHA-256, got len %d", len(h1))
	}
}

// TestHashToken_Different: different tokens produce different hashes.
func TestHashToken_Different(t *testing.T) {
	h1 := hashToken("token-a")
	h2 := hashToken("token-b")
	if h1 == h2 {
		t.Fatal("different tokens should produce different hashes")
	}
}

// TestExtractAttr_FromResource: attribute found in resource takes precedence.
func TestExtractAttr_FromResource(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(attrProjectID, "from-resource")
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.Attributes().PutStr(attrProjectID, "from-span")

	got := extractAttr(rs, attrProjectID)
	if got != "from-resource" {
		t.Fatalf("expected 'from-resource', got %q", got)
	}
}

// TestExtractAttr_FallbackToSpan: falls back to span attribute when not in resource.
func TestExtractAttr_FallbackToSpan(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	// No resource attribute for project_id.
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.Attributes().PutStr(attrProjectID, "from-span")

	got := extractAttr(rs, attrProjectID)
	if got != "from-span" {
		t.Fatalf("expected 'from-span', got %q", got)
	}
}

// TestExtractAttr_Missing: returns empty string when attribute is absent.
func TestExtractAttr_Missing(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	got := extractAttr(rs, attrProjectID)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
