package piimaskerproc

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

// makeProcessor builds a piiMaskerProcessor with a pre-populated
// enabledProjects map — no Postgres required for unit tests.
func makeProcessor(t *testing.T, enabled map[string]*projectPIISettings) *piiMaskerProcessor {
	t.Helper()
	logger := zaptest.NewLogger(t)
	proc := &piiMaskerProcessor{
		cfg:             defaultConfig(),
		store:           nil, // not used in unit tests
		logger:          logger,
		enabledProjects: enabled,
		done:            make(chan struct{}),
	}
	// Compile builtins (mirrors Start).
	proc.builtins = builtinPatterns()
	proc.combinedBuiltin = buildCombinedRegex(proc.builtins)
	return proc
}

// makeTrace builds a minimal ptrace.Traces with one ResourceSpan, one
// ScopeSpan, and one Span. projectID is set on the resource attributes.
func makeTrace(projectID string, spanAttrs map[string]string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	if projectID != "" {
		rs.Resource().Attributes().PutStr("agentpulse.project_id", projectID)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("test-span")
	for k, v := range spanAttrs {
		span.Attributes().PutStr(k, v)
	}
	return td
}

// ── Core redaction tests ──────────────────────────────────────────────────────

func TestProcessTraces_RedactionEnabled_PIIRedacted(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {customRules: nil},
	})

	td := makeTrace("proj-1", map[string]string{
		"gen_ai.prompt": "My email is alice@example.com please help",
	})

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, ok := span.Attributes().Get("gen_ai.prompt")
	if !ok {
		t.Fatal("gen_ai.prompt attribute missing")
	}
	if v.Str() == "My email is alice@example.com please help" {
		t.Error("expected email to be redacted but it was not")
	}
	if !containsStr(v.Str(), "[REDACTED:email]") {
		t.Errorf("expected [REDACTED:email] token, got %q", v.Str())
	}
}

func TestProcessTraces_RedactionDisabled_AttributesUnchanged(t *testing.T) {
	// Project not in enabledProjects map → redaction disabled.
	proc := makeProcessor(t, map[string]*projectPIISettings{})

	td := makeTrace("proj-no-pii", map[string]string{
		"gen_ai.prompt": "My email is alice@example.com please help",
	})

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, _ := span.Attributes().Get("gen_ai.prompt")
	if v.Str() != "My email is alice@example.com please help" {
		t.Errorf("expected prompt unchanged, got %q", v.Str())
	}
}

func TestProcessTraces_NoProjectID_PassThrough(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	// Build trace without project_id.
	td := makeTrace("", map[string]string{
		"gen_ai.prompt": "SSN 123-45-6789",
	})

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, _ := span.Attributes().Get("gen_ai.prompt")
	if v.Str() != "SSN 123-45-6789" {
		t.Errorf("expected prompt unchanged (no project_id), got %q", v.Str())
	}
}

// ── Allowlist tests ───────────────────────────────────────────────────────────

func TestProcessTraces_AllowlistedFields_NeverModified(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	// Put an email address as the value of allowlisted fields — they must NOT
	// be modified even if they pattern-match.
	td := makeTrace("proj-1", map[string]string{
		"agentpulse.project_id": "alice@example.com",
		"agentpulse.run_id":     "alice@example.com",
		"gen_ai.prompt":         "alice@example.com",
	})

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

	for _, key := range []string{"agentpulse.project_id", "agentpulse.run_id"} {
		v, ok := span.Attributes().Get(key)
		if !ok {
			t.Fatalf("allowlisted key %q missing", key)
		}
		if v.Str() != "alice@example.com" {
			t.Errorf("allowlisted field %q was modified: got %q", key, v.Str())
		}
	}

	// Non-allowlisted field should be redacted.
	v, _ := span.Attributes().Get("gen_ai.prompt")
	if v.Str() == "alice@example.com" {
		t.Error("gen_ai.prompt should have been redacted")
	}
}

// ── Stamp tests ───────────────────────────────────────────────────────────────

func TestProcessTraces_RedactionsCount_Stamped(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	td := makeTrace("proj-1", map[string]string{
		"gen_ai.prompt": "email alice@example.com and SSN 123-45-6789",
	})

	result, _ := proc.ProcessTraces(context.Background(), td)
	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

	countVal, ok := span.Attributes().Get("agentpulse.pii_redactions_count")
	if !ok {
		t.Fatal("agentpulse.pii_redactions_count not stamped")
	}
	if countVal.Int() < 2 {
		t.Errorf("expected at least 2 redactions, got %d", countVal.Int())
	}
}

func TestProcessTraces_NoRedactions_CountNotStamped(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	td := makeTrace("proj-1", map[string]string{
		"gen_ai.prompt": "no sensitive data here",
	})

	result, _ := proc.ProcessTraces(context.Background(), td)
	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

	if _, ok := span.Attributes().Get("agentpulse.pii_redactions_count"); ok {
		t.Error("agentpulse.pii_redactions_count should NOT be stamped when there are no redactions")
	}
}

// ── Resource attribute tests ──────────────────────────────────────────────────

func TestProcessTraces_ResourceAttributes_Scanned(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("agentpulse.project_id", "proj-1")
	rs.Resource().Attributes().PutStr("service.instance.id", "alice@example.com")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Spans().AppendEmpty().SetName("span")

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, ok := result.ResourceSpans().At(0).Resource().Attributes().Get("service.instance.id")
	if !ok {
		t.Fatal("service.instance.id missing")
	}
	if v.Str() == "alice@example.com" {
		t.Error("resource attribute with email should have been redacted")
	}
}

func TestProcessTraces_ResourceAgentpulseFields_NotScanned(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("agentpulse.project_id", "proj-1")
	// agentpulse.* resource keys should be skipped.
	rs.Resource().Attributes().PutStr("agentpulse.custom_meta", "alice@example.com")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Spans().AppendEmpty().SetName("span")

	result, _ := proc.ProcessTraces(context.Background(), td)
	v, ok := result.ResourceSpans().At(0).Resource().Attributes().Get("agentpulse.custom_meta")
	if !ok {
		t.Fatal("agentpulse.custom_meta missing")
	}
	if v.Str() != "alice@example.com" {
		t.Errorf("agentpulse.* resource key was incorrectly modified: %q", v.Str())
	}
}

// ── Fail-closed mode tests ────────────────────────────────────────────────────

func TestProcessTraces_FailedLoad_AllProjectsRedacted(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{})
	proc.failedLoad = true

	// Even though enabledProjects is empty, failedLoad=true means redact all.
	td := makeTrace("any-project-id", map[string]string{
		"gen_ai.prompt": "My email is alice@example.com",
	})

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, _ := span.Attributes().Get("gen_ai.prompt")
	if containsStr(v.Str(), "alice@example.com") {
		t.Errorf("expected email redacted in fail-closed mode, got %q", v.Str())
	}
}

func TestProcessTraces_FailedLoad_BuiltinsOnly_NoCustomRules(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{})
	proc.failedLoad = true

	// Custom rule that would match "ACCT-12345678" — should NOT apply in
	// fail-closed mode (builtins only).
	td := makeTrace("proj-1", map[string]string{
		"gen_ai.prompt": "account ACCT-12345678 is flagged",
	})

	result, _ := proc.ProcessTraces(context.Background(), td)
	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, _ := span.Attributes().Get("gen_ai.prompt")
	// ACCT- pattern is not a builtin, so it should not be redacted.
	if !containsStr(v.Str(), "ACCT-12345678") {
		t.Errorf("custom pattern should not apply in fail-closed mode, got %q", v.Str())
	}
}

// ── Span events tests ─────────────────────────────────────────────────────────

func TestProcessTraces_SpanEvents_Scanned(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("agentpulse.project_id", "proj-1")
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("span")
	event := span.Events().AppendEmpty()
	event.SetName("user_message")
	event.Attributes().PutStr("message", "my SSN is 123-45-6789")

	result, _ := proc.ProcessTraces(context.Background(), td)
	ev := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Events().At(0)
	msgVal, _ := ev.Attributes().Get("message")
	if containsStr(msgVal.Str(), "123-45-6789") {
		t.Errorf("SSN in span event was not redacted: %q", msgVal.Str())
	}
}

// ── Custom rule integration ───────────────────────────────────────────────────

func TestProcessTraces_CustomRule_Applied(t *testing.T) {
	customPatterns := parseCustomRules(zaptest.NewLogger(t), []piiCustomRule{
		{Name: "acct_id", Pattern: `\bACCT-\d{8}\b`, Enabled: true},
	})

	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {customRules: customPatterns},
	})

	td := makeTrace("proj-1", map[string]string{
		"gen_ai.prompt": "account ACCT-12345678 was flagged",
	})

	result, _ := proc.ProcessTraces(context.Background(), td)
	span := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, _ := span.Attributes().Get("gen_ai.prompt")
	if containsStr(v.Str(), "ACCT-12345678") {
		t.Errorf("custom rule ACCT- pattern not applied: %q", v.Str())
	}
	if !containsStr(v.Str(), "[REDACTED:acct_id]") {
		t.Errorf("expected [REDACTED:acct_id] token, got %q", v.Str())
	}
}

// ── Non-string attributes untouched ──────────────────────────────────────────

func TestProcessTraces_NonStringAttributes_Untouched(t *testing.T) {
	proc := makeProcessor(t, map[string]*projectPIISettings{
		"proj-1": {},
	})

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("agentpulse.project_id", "proj-1")
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("span")
	span.Attributes().PutInt("token_count", 42)
	span.Attributes().PutDouble("cost_usd", 0.0012)
	span.Attributes().PutBool("streaming", true)

	result, err := proc.ProcessTraces(context.Background(), td)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	checkInt := func(key string, want int64) {
		v, ok := attrs.Get(key)
		if !ok {
			t.Fatalf("%s missing", key)
		}
		if v.Type() != pcommon.ValueTypeInt || v.Int() != want {
			t.Errorf("%s: expected int %d, got type=%v val=%v", key, want, v.Type(), v)
		}
	}
	checkInt("token_count", 42)
}

// ── helper ────────────────────────────────────────────────────────────────────

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
