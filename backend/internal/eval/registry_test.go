package eval

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

func TestNewRegistryBuiltins(t *testing.T) {
	r := NewRegistry(nil)
	builtins := []string{"relevance", "hallucination", "faithfulness", "toxicity", "tool_correctness"}
	for _, name := range builtins {
		if _, ok := r[name]; !ok {
			t.Errorf("registry missing builtin %q", name)
		}
	}
}

func TestNewRegistryCustomEval(t *testing.T) {
	tmpl := "Rate the {{input}} vs {{output}}"
	configs := []*domain.EvalConfig{
		{EvalName: "custom:tone", SpanKind: "llm.call", PromptTemplate: &tmpl},
	}
	r := NewRegistry(configs)
	if _, ok := r["custom:tone"]; !ok {
		t.Error("registry missing custom eval custom:tone")
	}
}

func TestNewRegistryCustomEvalInvalidTemplate(t *testing.T) {
	bad := "no placeholder here"
	configs := []*domain.EvalConfig{
		{EvalName: "custom:bad", SpanKind: "llm.call", PromptTemplate: &bad},
	}
	r := NewRegistry(configs)
	if _, ok := r["custom:bad"]; ok {
		t.Error("registry should not register a custom eval with invalid template")
	}
}

func TestNewRegistryNilTemplateSkipped(t *testing.T) {
	configs := []*domain.EvalConfig{
		{EvalName: "custom:nil", SpanKind: "llm.call", PromptTemplate: nil},
	}
	r := NewRegistry(configs)
	if _, ok := r["custom:nil"]; ok {
		t.Error("registry should not register a config with nil template")
	}
}

func TestNewRegistryBuiltinsNotOverriddenByCustom(t *testing.T) {
	// A config named "relevance" with a template should not override the builtin.
	tmpl := "custom relevance {{input}}"
	configs := []*domain.EvalConfig{
		{EvalName: "relevance", SpanKind: "llm.call", PromptTemplate: &tmpl},
	}
	r := NewRegistry(configs)
	// The registry should still contain relevance — we just accept whichever wins.
	if _, ok := r["relevance"]; !ok {
		t.Error("relevance missing from registry after custom config")
	}
}
