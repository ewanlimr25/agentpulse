package agentsemanticproc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

// testRegistry returns a minimal attributeRegistry for tests.
func testRegistry() *attributeRegistry {
	return &attributeRegistry{
		SpanKindDetection: []spanKindRule{
			{
				Attribute: "gen_ai.operation.name",
				ValueMap: map[string]string{
					"chat":       "llm.call",
					"tool_call":  "tool.call",
					"handoff":    "agent.handoff",
					"memory_read":  "memory.read",
					"memory_write": "memory.write",
				},
			},
			{
				Attribute:   "agentpulse.span_kind",
				Passthrough: true,
			},
			{
				Attribute: "_span_name",
				PrefixMap: map[string]string{
					"llm.":         "llm.call",
					"tool.":        "tool.call",
					"agent.handoff": "agent.handoff",
				},
			},
		},
		FieldExtraction: map[string][]string{
			"agent_name":    {"agent.name"},
			"model_id":      {"gen_ai.request.model"},
			"input_tokens":  {"gen_ai.usage.input_tokens"},
			"output_tokens": {"gen_ai.usage.output_tokens"},
			"run_id":        {"agentpulse.run_id"},
		},
		Cost: costRegistryConfig{
			ExplicitAttribute: "agentpulse.cost_usd",
		},
	}
}

// testPricing returns a minimal pricingRegistry for tests.
func testPricing() *pricingRegistry {
	return &pricingRegistry{
		Models: map[string]modelPrice{
			"gpt-4o": {InputPerMillion: 2.50, OutputPerMillion: 10.00},
		},
		Fallback: modelPrice{InputPerMillion: 0, OutputPerMillion: 0},
	}
}

// makeSpan creates a test span with the given name and attributes.
func makeSpan(name string, attrs map[string]any) ptrace.Span {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName(name)
	for k, v := range attrs {
		switch val := v.(type) {
		case string:
			span.Attributes().PutStr(k, val)
		case int:
			span.Attributes().PutInt(k, int64(val))
		case int64:
			span.Attributes().PutInt(k, val)
		case float64:
			span.Attributes().PutDouble(k, val)
		}
	}
	return span
}

// processSpan runs a single span through the processor and returns the mutated span.
func processSpan(t *testing.T, span ptrace.Span) ptrace.Span {
	t.Helper()
	proc := newSemanticProcessor(zaptest.NewLogger(t), testRegistry(), testPricing())

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span.CopyTo(ss.Spans().AppendEmpty())

	out, err := proc.ProcessTraces(context.Background(), td)
	require.NoError(t, err)
	return out.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
}

func strAttr(t *testing.T, attrs pcommon.Map, key string) string {
	t.Helper()
	v, ok := attrs.Get(key)
	require.True(t, ok, "expected attribute %q to exist", key)
	return v.Str()
}

// ── Span kind classification ──────────────────────────────────────────────────

func TestClassify_LLMCall_ByOperationName(t *testing.T) {
	span := makeSpan("some-span", map[string]any{"gen_ai.operation.name": "chat"})
	out := processSpan(t, span)
	assert.Equal(t, "llm.call", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_ToolCall_ByOperationName(t *testing.T) {
	span := makeSpan("some-span", map[string]any{"gen_ai.operation.name": "tool_call"})
	out := processSpan(t, span)
	assert.Equal(t, "tool.call", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_AgentHandoff_ByOperationName(t *testing.T) {
	span := makeSpan("some-span", map[string]any{"gen_ai.operation.name": "handoff"})
	out := processSpan(t, span)
	assert.Equal(t, "agent.handoff", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_MemoryRead_ByOperationName(t *testing.T) {
	span := makeSpan("some-span", map[string]any{"gen_ai.operation.name": "memory_read"})
	out := processSpan(t, span)
	assert.Equal(t, "memory.read", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_MemoryWrite_ByOperationName(t *testing.T) {
	span := makeSpan("some-span", map[string]any{"gen_ai.operation.name": "memory_write"})
	out := processSpan(t, span)
	assert.Equal(t, "memory.write", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_Passthrough_ExplicitSpanKind(t *testing.T) {
	span := makeSpan("some-span", map[string]any{"agentpulse.span_kind": "tool.call"})
	out := processSpan(t, span)
	assert.Equal(t, "tool.call", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_SpanNamePrefix_LLMCall(t *testing.T) {
	span := makeSpan("llm.chat.completion", nil)
	out := processSpan(t, span)
	assert.Equal(t, "llm.call", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_SpanNamePrefix_ToolCall(t *testing.T) {
	span := makeSpan("tool.web_search", nil)
	out := processSpan(t, span)
	assert.Equal(t, "tool.call", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_Unknown_WhenNoMatchingRule(t *testing.T) {
	span := makeSpan("some-unrelated-span", nil)
	out := processSpan(t, span)
	assert.Equal(t, "unknown", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

func TestClassify_OperationNameTakesPriorityOverSpanName(t *testing.T) {
	// gen_ai.operation.name rule comes before _span_name in registry
	span := makeSpan("tool.something", map[string]any{"gen_ai.operation.name": "chat"})
	out := processSpan(t, span)
	assert.Equal(t, "llm.call", strAttr(t, out.Attributes(), attrAgentSpanKind))
}

// ── Field extraction ──────────────────────────────────────────────────────────

func TestEnrich_ExtractsAgentName(t *testing.T) {
	span := makeSpan("s", map[string]any{
		"gen_ai.operation.name": "chat",
		"agent.name":            "ResearchAgent",
	})
	out := processSpan(t, span)
	assert.Equal(t, "ResearchAgent", strAttr(t, out.Attributes(), attrAgentName))
}

func TestEnrich_ExtractsModelID(t *testing.T) {
	span := makeSpan("s", map[string]any{
		"gen_ai.operation.name": "chat",
		"gen_ai.request.model":  "gpt-4o",
	})
	out := processSpan(t, span)
	assert.Equal(t, "gpt-4o", strAttr(t, out.Attributes(), attrModelID))
}

func TestEnrich_ExtractsRunID(t *testing.T) {
	span := makeSpan("s", map[string]any{
		"agentpulse.run_id": "run-abc-123",
	})
	out := processSpan(t, span)
	assert.Equal(t, "run-abc-123", strAttr(t, out.Attributes(), attrRunID))
}

func TestEnrich_NoAgentName_WhenAttributeMissing(t *testing.T) {
	span := makeSpan("s", nil)
	out := processSpan(t, span)
	_, ok := out.Attributes().Get(attrAgentName)
	assert.False(t, ok, "agent name attribute should not be set when source attribute is absent")
}

// ── Cost computation ──────────────────────────────────────────────────────────

func TestEnrich_ComputesCost_KnownModel(t *testing.T) {
	span := makeSpan("s", map[string]any{
		"gen_ai.operation.name":    "chat",
		"gen_ai.request.model":     "gpt-4o",
		"gen_ai.usage.input_tokens":  int64(1_000_000),
		"gen_ai.usage.output_tokens": int64(500_000),
	})
	out := processSpan(t, span)

	v, ok := out.Attributes().Get(attrCostUSD)
	require.True(t, ok)
	// 1M input @ $2.50 + 0.5M output @ $10.00 = $2.50 + $5.00 = $7.50
	assert.InDelta(t, 7.50, v.Double(), 0.001)
}

func TestEnrich_ZeroCost_UnknownModel(t *testing.T) {
	span := makeSpan("s", map[string]any{
		"gen_ai.operation.name":    "chat",
		"gen_ai.request.model":     "unknown-model-xyz",
		"gen_ai.usage.input_tokens":  int64(1000),
		"gen_ai.usage.output_tokens": int64(500),
	})
	out := processSpan(t, span)
	// Fallback pricing is 0, so no cost attribute should be set
	_, ok := out.Attributes().Get(attrCostUSD)
	assert.False(t, ok, "cost attribute should not be set when pricing is zero")
}

func TestEnrich_SkipsCostComputation_WhenExplicitCostPresent(t *testing.T) {
	span := makeSpan("s", map[string]any{
		"gen_ai.operation.name":    "chat",
		"gen_ai.request.model":     "gpt-4o",
		"gen_ai.usage.input_tokens":  int64(1_000_000),
		"gen_ai.usage.output_tokens": int64(1_000_000),
		"agentpulse.cost_usd":        0.001, // explicit override
	})
	out := processSpan(t, span)
	v, ok := out.Attributes().Get(attrCostUSD)
	require.True(t, ok)
	assert.InDelta(t, 0.001, v.Double(), 0.0001, "explicit cost should not be overwritten")
}

// ── Pass-through safety ───────────────────────────────────────────────────────

func TestEnrich_UnknownSpan_PassesThrough_WithoutError(t *testing.T) {
	span := makeSpan("some-arbitrary-span", map[string]any{
		"custom.attribute": "some-value",
	})
	out := processSpan(t, span)

	// Unknown spans still get a span_kind tag
	assert.Equal(t, "unknown", strAttr(t, out.Attributes(), attrAgentSpanKind))

	// Original attribute is preserved
	v, ok := out.Attributes().Get("custom.attribute")
	require.True(t, ok)
	assert.Equal(t, "some-value", v.Str())
}

func TestEnrich_EmptySpan_NoError(t *testing.T) {
	span := makeSpan("", nil)
	out := processSpan(t, span)
	assert.Equal(t, "unknown", strAttr(t, out.Attributes(), attrAgentSpanKind))
}
