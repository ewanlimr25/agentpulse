package eval

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
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
