package eval

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ── matchesScopeFilter ────────────────────────────────────────────────────────

func TestMatchesScopeFilterEmpty(t *testing.T) {
	if !matchesScopeFilter(nil, "any-agent") {
		t.Error("nil filter should match any agent")
	}
	if !matchesScopeFilter(map[string][]string{}, "any-agent") {
		t.Error("empty filter should match any agent")
	}
}

func TestMatchesScopeFilterMatch(t *testing.T) {
	f := map[string][]string{"agent_name": {"researcher", "writer"}}
	if !matchesScopeFilter(f, "researcher") {
		t.Error("filter should match 'researcher'")
	}
	if !matchesScopeFilter(f, "writer") {
		t.Error("filter should match 'writer'")
	}
}

func TestMatchesScopeFilterNoMatch(t *testing.T) {
	f := map[string][]string{"agent_name": {"researcher"}}
	if matchesScopeFilter(f, "planner") {
		t.Error("filter should not match 'planner'")
	}
}

func TestMatchesScopeFilterNoAgentNameKey(t *testing.T) {
	// A filter with an unrecognised key matches all (conservative default).
	f := map[string][]string{"other_key": {"value"}}
	if !matchesScopeFilter(f, "anyone") {
		t.Error("filter with unknown key should match any agent")
	}
}

// ── buildEvalMap ──────────────────────────────────────────────────────────────

func TestBuildEvalMapGroups(t *testing.T) {
	configs := []*domain.EvalConfig{
		{ProjectID: "p1", SpanKind: "llm.call", EvalName: "relevance"},
		{ProjectID: "p1", SpanKind: "llm.call", EvalName: "hallucination"},
		{ProjectID: "p2", SpanKind: "tool.call", EvalName: "tool_correctness"},
	}
	m := buildEvalMap(configs)
	if len(m["p1"]["llm.call"]) != 2 {
		t.Errorf("expected 2 llm.call evals for p1, got %d", len(m["p1"]["llm.call"]))
	}
	if len(m["p2"]["tool.call"]) != 1 {
		t.Errorf("expected 1 tool.call eval for p2, got %d", len(m["p2"]["tool.call"]))
	}
}

// ── evalNamesForSpan ──────────────────────────────────────────────────────────

func TestEvalNamesForSpanDefault(t *testing.T) {
	// No configs → llm.call should fall back to relevance.
	m := buildEvalMap(nil)
	names := m.evalNamesForSpan("p-unknown", "llm.call", "any")
	if len(names) != 1 || names[0] != "relevance" {
		t.Errorf("expected [relevance], got %v", names)
	}
}

func TestEvalNamesForSpanToolCallNoDefault(t *testing.T) {
	// tool.call with no configs should return nil (no default).
	m := buildEvalMap(nil)
	names := m.evalNamesForSpan("p-unknown", "tool.call", "any")
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestEvalNamesForSpanScopeFilter(t *testing.T) {
	configs := []*domain.EvalConfig{
		{
			ProjectID: "p1", SpanKind: "llm.call", EvalName: "relevance",
			ScopeFilter: map[string][]string{"agent_name": {"researcher"}},
		},
	}
	m := buildEvalMap(configs)

	// Should match
	names := m.evalNamesForSpan("p1", "llm.call", "researcher")
	if len(names) != 1 {
		t.Errorf("expected 1 eval for researcher, got %v", names)
	}

	// Should not match
	names = m.evalNamesForSpan("p1", "llm.call", "planner")
	if len(names) != 0 {
		t.Errorf("expected 0 evals for planner, got %v", names)
	}
}

func TestEvalNamesForSpanNoScopeMatchFallsBackToDefault(t *testing.T) {
	// If project has configs but none match the agent, return empty (no default fallback).
	configs := []*domain.EvalConfig{
		{
			ProjectID: "p1", SpanKind: "llm.call", EvalName: "relevance",
			ScopeFilter: map[string][]string{"agent_name": {"researcher"}},
		},
	}
	m := buildEvalMap(configs)
	names := m.evalNamesForSpan("p1", "llm.call", "unknown-agent")
	if len(names) != 0 {
		t.Errorf("expected empty for unmatched agent, got %v", names)
	}
}

// ── hasToolCallConfigs ────────────────────────────────────────────────────────

func TestHasToolCallConfigs(t *testing.T) {
	configs := []*domain.EvalConfig{
		{SpanKind: "llm.call"},
		{SpanKind: "tool.call"},
	}
	if !hasToolCallConfigs(configs) {
		t.Error("expected true")
	}
}

func TestHasToolCallConfigsFalse(t *testing.T) {
	configs := []*domain.EvalConfig{
		{SpanKind: "llm.call"},
	}
	if hasToolCallConfigs(configs) {
		t.Error("expected false")
	}
}

func TestHasToolCallConfigsEmpty(t *testing.T) {
	if hasToolCallConfigs(nil) {
		t.Error("expected false for nil")
	}
}
