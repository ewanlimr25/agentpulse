package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestEvaluate — pure logic, no I/O
// ---------------------------------------------------------------------------

func TestEvaluate_OverallAboveThreshold(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 5,
		OverallScore:   0.85,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.85, SpanCount: 10, RunCount: 5},
		},
	}
	pass, score, msg := evaluate(b, 0.80, "")
	if !pass {
		t.Fatalf("expected pass=true, got false (score=%.3f)", score)
	}
	if score != 0.85 {
		t.Errorf("expected score=0.85, got %.3f", score)
	}
	if !strings.Contains(msg, "overall") {
		t.Errorf("expected message to mention 'overall', got: %q", msg)
	}
}

func TestEvaluate_OverallBelowThreshold(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 5,
		OverallScore:   0.55,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.55, SpanCount: 8, RunCount: 5},
		},
	}
	pass, score, _ := evaluate(b, 0.70, "")
	if pass {
		t.Fatalf("expected pass=false, got true (score=%.3f)", score)
	}
	if score != 0.55 {
		t.Errorf("expected score=0.55, got %.3f", score)
	}
}

func TestEvaluate_SpecificEvalTypeAboveThreshold(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 5,
		OverallScore:   0.70,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.90, SpanCount: 20, RunCount: 5},
			{EvalName: "hallucination", AvgScore: 0.50, SpanCount: 10, RunCount: 5},
		},
	}
	pass, score, msg := evaluate(b, 0.80, "relevance")
	if !pass {
		t.Fatalf("expected pass=true for relevance, got false (score=%.3f)", score)
	}
	if score != 0.90 {
		t.Errorf("expected score=0.90, got %.3f", score)
	}
	if !strings.Contains(msg, "relevance") {
		t.Errorf("expected message to contain 'relevance', got: %q", msg)
	}
}

func TestEvaluate_SpecificEvalTypeBelowThreshold(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 5,
		OverallScore:   0.70,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.90, SpanCount: 20, RunCount: 5},
			{EvalName: "hallucination", AvgScore: 0.50, SpanCount: 10, RunCount: 5},
		},
	}
	pass, score, msg := evaluate(b, 0.75, "hallucination")
	if pass {
		t.Fatalf("expected pass=false for hallucination, got true (score=%.3f)", score)
	}
	if score != 0.50 {
		t.Errorf("expected score=0.50, got %.3f", score)
	}
	if !strings.Contains(msg, "hallucination") {
		t.Errorf("expected message to contain 'hallucination', got: %q", msg)
	}
}

func TestEvaluate_EvalTypeNotFound(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 5,
		OverallScore:   0.80,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.80, SpanCount: 10, RunCount: 5},
		},
	}
	pass, score, msg := evaluate(b, 0.70, "toxicity")
	if pass {
		t.Error("expected pass=false for missing eval type, got true")
	}
	if score != 0 {
		t.Errorf("expected score=0 for missing eval type, got %.3f", score)
	}
	if !strings.Contains(msg, "toxicity") {
		t.Errorf("expected message to mention missing eval type 'toxicity', got: %q", msg)
	}
}

func TestEvaluate_PartialRunCoverageInMessage(t *testing.T) {
	// RunCount (3) < RunsConsidered (5) — message should mention partial coverage.
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 5,
		OverallScore:   0.75,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.75, SpanCount: 6, RunCount: 3},
		},
	}
	_, _, msg := evaluate(b, 0.70, "relevance")
	// The message must include both RunCount and RunsConsidered so the caller
	// can see that coverage is partial — e.g. "3/5 runs".
	if !strings.Contains(msg, "3") || !strings.Contains(msg, "5") {
		t.Errorf("expected partial coverage info (3/5) in message, got: %q", msg)
	}
}

func TestEvaluate_ExactThresholdPassesAtBoundary(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-1",
		RunsConsidered: 2,
		OverallScore:   0.70,
		Types:          []evalTypeBaseline{{EvalName: "relevance", AvgScore: 0.70, SpanCount: 4, RunCount: 2}},
	}
	// Score exactly equals threshold — should pass (>= comparison).
	pass, _, _ := evaluate(b, 0.70, "")
	if !pass {
		t.Error("expected score == threshold to pass (>= comparison)")
	}
}

func TestEvaluate_EmptyTypes_OverallZero(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-empty",
		RunsConsidered: 0,
		OverallScore:   0,
		Types:          nil,
	}
	pass, score, _ := evaluate(b, 0.50, "")
	if pass {
		t.Error("expected pass=false when overall score is 0 and threshold is 0.50")
	}
	if score != 0 {
		t.Errorf("expected score=0, got %.3f", score)
	}
}

// ---------------------------------------------------------------------------
// TestValidateEndpoint
// ---------------------------------------------------------------------------

func TestValidateEndpoint_ValidHTTPS(t *testing.T) {
	if err := validateEndpoint("https://api.agentpulse.io"); err != nil {
		t.Errorf("expected no error for valid https URL, got: %v", err)
	}
}

func TestValidateEndpoint_ValidHTTP(t *testing.T) {
	if err := validateEndpoint("http://localhost:8080"); err != nil {
		t.Errorf("expected no error for valid http URL, got: %v", err)
	}
}

func TestValidateEndpoint_FTPSchemeRejected(t *testing.T) {
	err := validateEndpoint("ftp://example.com")
	if err == nil {
		t.Error("expected error for ftp:// scheme, got nil")
	}
	if !strings.Contains(err.Error(), "http") {
		t.Errorf("expected error message to mention http/https, got: %q", err.Error())
	}
}

func TestValidateEndpoint_EmptySchemRejected(t *testing.T) {
	err := validateEndpoint("example.com")
	if err == nil {
		t.Error("expected error for URL without scheme, got nil")
	}
}

func TestValidateEndpoint_CredentialsInHostRejected(t *testing.T) {
	// Credentials embedded in the URL are an SSRF / credential leakage risk.
	err := validateEndpoint("https://user:pass@api.example.com")
	if err == nil {
		t.Error("expected error for URL with credentials in host, got nil")
	}
	if !strings.Contains(err.Error(), "credentials") {
		t.Errorf("expected error to mention credentials, got: %q", err.Error())
	}
}

func TestValidateEndpoint_MissingHostRejected(t *testing.T) {
	err := validateEndpoint("https://")
	if err == nil {
		t.Error("expected error for URL with empty host, got nil")
	}
}

func TestValidateEndpoint_TrailingSlashAccepted(t *testing.T) {
	// Common user mistake — should still be accepted.
	if err := validateEndpoint("https://api.agentpulse.io/"); err != nil {
		t.Errorf("expected no error for URL with trailing slash, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestPrintJSON — captures stdout and validates structure
// ---------------------------------------------------------------------------

func captureStdout(f func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintJSON_ValidOutput(t *testing.T) {
	out := &jsonOutput{
		Pass:      true,
		ExitCode:  exitPass,
		Threshold: 0.80,
		EvalType:  "relevance",
		Baseline: &evalBaseline{
			ProjectID:      "proj-abc",
			RunsConsidered: 10,
			OverallScore:   0.85,
			Types: []evalTypeBaseline{
				{EvalName: "relevance", AvgScore: 0.85, SpanCount: 50, RunCount: 10},
			},
		},
		Message: "relevance score 0.850 (10 runs, 50 spans)",
	}

	captured := captureStdout(func() {
		printJSON(out)
	})

	if !json.Valid([]byte(captured)) {
		t.Fatalf("printJSON output is not valid JSON: %q", captured)
	}

	var decoded jsonOutput
	if err := json.Unmarshal([]byte(captured), &decoded); err != nil {
		t.Fatalf("failed to unmarshal printJSON output: %v", err)
	}

	if decoded.Pass != true {
		t.Errorf("expected pass=true, got %v", decoded.Pass)
	}
	if decoded.ExitCode != exitPass {
		t.Errorf("expected exit_code=%d, got %d", exitPass, decoded.ExitCode)
	}
	if decoded.Threshold != 0.80 {
		t.Errorf("expected threshold=0.80, got %v", decoded.Threshold)
	}
	if decoded.EvalType != "relevance" {
		t.Errorf("expected eval_type='relevance', got %q", decoded.EvalType)
	}
	if decoded.Baseline == nil {
		t.Fatal("expected baseline to be non-nil in JSON output")
	}
	if decoded.Baseline.ProjectID != "proj-abc" {
		t.Errorf("expected baseline.project_id='proj-abc', got %q", decoded.Baseline.ProjectID)
	}
	if decoded.Message == "" {
		t.Error("expected non-empty message field")
	}
}

func TestPrintJSON_FailOutput(t *testing.T) {
	out := &jsonOutput{
		Pass:      false,
		ExitCode:  exitFail,
		Threshold: 0.75,
		Baseline: &evalBaseline{
			ProjectID:      "proj-xyz",
			RunsConsidered: 5,
			OverallScore:   0.60,
		},
		Message: "overall score 0.600 across 0 eval type(s), 5 run(s)",
	}

	captured := captureStdout(func() {
		printJSON(out)
	})

	if !json.Valid([]byte(captured)) {
		t.Fatalf("printJSON output is not valid JSON: %q", captured)
	}

	var decoded jsonOutput
	if err := json.Unmarshal([]byte(captured), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Pass != false {
		t.Errorf("expected pass=false, got %v", decoded.Pass)
	}
	if decoded.ExitCode != exitFail {
		t.Errorf("expected exit_code=%d, got %d", exitFail, decoded.ExitCode)
	}
}

func TestPrintJSON_OmitsEvalTypeWhenEmpty(t *testing.T) {
	out := &jsonOutput{
		Pass:      true,
		ExitCode:  exitPass,
		Threshold: 0.50,
		EvalType:  "", // should be omitted from JSON (omitempty)
		Baseline:  &evalBaseline{ProjectID: "p"},
		Message:   "ok",
	}

	captured := captureStdout(func() {
		printJSON(out)
	})

	// The "eval_type" key should not appear when empty due to omitempty tag.
	if strings.Contains(captured, `"eval_type"`) {
		t.Errorf("expected eval_type to be omitted when empty, but found it in: %q", captured)
	}
}

// ---------------------------------------------------------------------------
// TestExtractProject — helper used in error messages
// ---------------------------------------------------------------------------

func TestExtractProject_NormalURL(t *testing.T) {
	got := extractProject("https://api.agentpulse.io/api/v1/projects/my-proj-id/evals/baseline?runs=10")
	if got != "my-proj-id" {
		t.Errorf("expected 'my-proj-id', got %q", got)
	}
}

func TestExtractProject_MalformedURL(t *testing.T) {
	got := extractProject("://not a url")
	if got != "unknown" {
		t.Errorf("expected 'unknown' for malformed URL, got %q", got)
	}
}

func TestExtractProject_NoProjectsSegment(t *testing.T) {
	got := extractProject("https://api.agentpulse.io/api/v1/evals/baseline")
	if got != "unknown" {
		t.Errorf("expected 'unknown' when path has no 'projects' segment, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// TestValidateEndpoint — url.Parse error path
// ---------------------------------------------------------------------------

func TestValidateEndpoint_UnparsableURL(t *testing.T) {
	// "%zz" is an invalid percent-encoded sequence that causes url.Parse to fail.
	err := validateEndpoint("%zzz://bad")
	if err == nil {
		t.Error("expected error for unparsable URL, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestPrintHuman — captures stdout and validates human-readable output
// ---------------------------------------------------------------------------

func TestPrintHuman_PassOutput(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-abc",
		RunsConsidered: 5,
		OverallScore:   0.85,
		Types: []evalTypeBaseline{
			{EvalName: "relevance", AvgScore: 0.85, SpanCount: 10, RunCount: 5},
		},
	}

	captured := captureStdout(func() {
		printHuman(b, 0.80, "", true, 0.85, "overall score 0.850 across 1 eval type(s), 5 run(s)")
	})

	if !strings.Contains(captured, "PASS") {
		t.Errorf("expected output to contain 'PASS', got:\n%s", captured)
	}
	if !strings.Contains(captured, "proj-abc") {
		t.Errorf("expected output to contain project ID, got:\n%s", captured)
	}
	if !strings.Contains(captured, "relevance") {
		t.Errorf("expected output to contain eval type name, got:\n%s", captured)
	}
}

func TestPrintHuman_FailOutput(t *testing.T) {
	b := &evalBaseline{
		ProjectID:      "proj-xyz",
		RunsConsidered: 3,
		OverallScore:   0.45,
		Types: []evalTypeBaseline{
			{EvalName: "hallucination", AvgScore: 0.45, SpanCount: 6, RunCount: 3},
		},
	}

	captured := captureStdout(func() {
		printHuman(b, 0.70, "", false, 0.45, "overall score 0.450 across 1 eval type(s), 3 run(s)")
	})

	if !strings.Contains(captured, "FAIL") {
		t.Errorf("expected output to contain 'FAIL', got:\n%s", captured)
	}
}

func TestPrintHuman_PartialCoverageIndicator(t *testing.T) {
	// When RunCount < RunsConsidered, the table must show the partial fraction.
	b := &evalBaseline{
		ProjectID:      "proj-partial",
		RunsConsidered: 10,
		OverallScore:   0.75,
		Types: []evalTypeBaseline{
			// RunCount=4 out of RunsConsidered=10 → should display "4/10 runs"
			{EvalName: "toxicity", AvgScore: 0.75, SpanCount: 8, RunCount: 4},
		},
	}

	captured := captureStdout(func() {
		printHuman(b, 0.70, "", true, 0.75, "toxicity score 0.750 (based on 4/10 runs, 8 spans)")
	})

	if !strings.Contains(captured, "4/10") {
		t.Errorf("expected partial coverage '4/10' in output, got:\n%s", captured)
	}
}

func TestPrintHuman_NoTypes_NoTable(t *testing.T) {
	// When Types is empty, the table section must be skipped gracefully.
	b := &evalBaseline{
		ProjectID:      "proj-empty",
		RunsConsidered: 0,
		OverallScore:   0,
		Types:          nil,
	}

	// Should not panic.
	captured := captureStdout(func() {
		printHuman(b, 0.70, "", false, 0, "overall score 0.000 across 0 eval type(s), 0 run(s)")
	})

	if !strings.Contains(captured, "proj-empty") {
		t.Errorf("expected project ID in output, got:\n%s", captured)
	}
}
