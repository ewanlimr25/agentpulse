package topologyexporter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func span(id, parentID, kind, agentName, modelID string) enrichedSpan {
	return enrichedSpan{
		SpanID:        id,
		ParentSpanID:  parentID,
		SpanName:      "span-" + id,
		RunID:         "run-1",
		ProjectID:     "proj-1",
		AgentSpanKind: kind,
		AgentName:     agentName,
		ModelID:       modelID,
		StartTime:     time.Now(),
		EndTime:       time.Now().Add(100 * time.Millisecond),
	}
}

func nodeBySpanID(nodes []topologyNode, spanID string) (topologyNode, bool) {
	for _, n := range nodes {
		if n.SpanID == spanID {
			return n, true
		}
	}
	return topologyNode{}, false
}

func edgeBetween(edges []topologyEdge, srcSpanID, dstSpanID string) (topologyEdge, bool) {
	for _, e := range edges {
		if e.SourceSpanID == srcSpanID && e.TargetSpanID == dstSpanID {
			return e, true
		}
	}
	return topologyEdge{}, false
}

// ── 1. Empty batch ────────────────────────────────────────────────────────────

func TestInferTopology_EmptyBatch(t *testing.T) {
	nodes, edges := InferTopology(nil)
	assert.Empty(t, nodes)
	assert.Empty(t, edges)
}

func TestInferTopology_EmptySlice(t *testing.T) {
	nodes, edges := InferTopology([]enrichedSpan{})
	assert.Empty(t, nodes)
	assert.Empty(t, edges)
}

// ── 2. Linear chain: A → B → C ───────────────────────────────────────────────

func TestInferTopology_LinearChain(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "agent.handoff", "Supervisor", ""),
		span("B", "A", "llm.call", "", "gpt-4o"),
		span("C", "B", "tool.call", "", ""),
	}
	spans[2].ToolName = "web_search"

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 3)
	require.Len(t, edges, 2)

	// Check node types
	a, _ := nodeBySpanID(nodes, "A")
	b, _ := nodeBySpanID(nodes, "B")
	c, _ := nodeBySpanID(nodes, "C")
	assert.Equal(t, nodeTypeAgent, a.NodeType)
	assert.Equal(t, nodeTypeLLM, b.NodeType)
	assert.Equal(t, nodeTypeTool, c.NodeType)

	// Check edge types
	ab, ok := edgeBetween(edges, "A", "B")
	require.True(t, ok)
	assert.Equal(t, edgeTypeInvocation, ab.EdgeType)

	bc, ok := edgeBetween(edges, "B", "C")
	require.True(t, ok)
	assert.Equal(t, edgeTypeInvocation, bc.EdgeType)
}

// ── 3. Fan-out: A has 3 children ─────────────────────────────────────────────

func TestInferTopology_FanOut(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "agent.handoff", "Orchestrator", ""),
		span("B", "A", "tool.call", "", ""),
		span("C", "A", "tool.call", "", ""),
		span("D", "A", "llm.call", "", "gpt-4o"),
	}

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 4)
	require.Len(t, edges, 3)

	_, ab := edgeBetween(edges, "A", "B")
	_, ac := edgeBetween(edges, "A", "C")
	_, ad := edgeBetween(edges, "A", "D")
	assert.True(t, ab, "expected edge A→B")
	assert.True(t, ac, "expected edge A→C")
	assert.True(t, ad, "expected edge A→D")
}

// ── 4. Agent handoff creates handoff edge type ────────────────────────────────

func TestInferTopology_HandoffEdgeType(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "agent.handoff", "Supervisor", ""),
		span("B", "A", "agent.handoff", "Researcher", ""),
	}

	_, edges := InferTopology(spans)

	require.Len(t, edges, 1)
	assert.Equal(t, edgeTypeHandoff, edges[0].EdgeType)
}

// ── 5. Memory access creates memory_access edge type ─────────────────────────

func TestInferTopology_MemoryAccessEdgeType(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "agent.handoff", "Agent", ""),
		span("B", "A", "memory.read", "", ""),
		span("C", "A", "memory.write", "", ""),
	}

	_, edges := InferTopology(spans)

	require.Len(t, edges, 2)
	ab, ok := edgeBetween(edges, "A", "B")
	require.True(t, ok)
	assert.Equal(t, edgeTypeMemoryAccess, ab.EdgeType)

	ac, ok := edgeBetween(edges, "A", "C")
	require.True(t, ok)
	assert.Equal(t, edgeTypeMemoryAccess, ac.EdgeType)
}

// ── 6. Orphan span: parent not in batch ──────────────────────────────────────

func TestInferTopology_OrphanSpan_NodeCreatedNoEdge(t *testing.T) {
	spans := []enrichedSpan{
		// parent "MISSING" is not in the batch
		span("B", "MISSING", "llm.call", "", "gpt-4o"),
	}

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 1, "orphan span should still create a node")
	assert.Empty(t, edges, "no edge when parent is absent from batch")
	assert.Equal(t, "B", nodes[0].SpanID)
}

// ── 7. Root span: no parent ───────────────────────────────────────────────────

func TestInferTopology_RootSpan_NodeCreatedNoEdge(t *testing.T) {
	spans := []enrichedSpan{
		span("ROOT", "", "agent.handoff", "Planner", ""),
	}

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 1)
	assert.Empty(t, edges)
}

// ── 8. Recursive: same agent name as parent and child ────────────────────────

func TestInferTopology_RecursiveAgent(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "agent.handoff", "SearchAgent", ""),
		span("B", "A", "agent.handoff", "SearchAgent", ""), // same name, different span
	}

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 2, "same agent name but different span IDs → two distinct nodes")
	require.Len(t, edges, 1)
	assert.Equal(t, "A", edges[0].SourceSpanID)
	assert.Equal(t, "B", edges[0].TargetSpanID)
}

// ── 9. Unknown span with no agent name is skipped ────────────────────────────

func TestInferTopology_UnknownSpan_NoAgentName_Skipped(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "unknown", "", ""),  // skipped: no agent name
		span("B", "A", "llm.call", "", "gpt-4o"),
	}

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 1, "only llm.call span should become a node")
	assert.Equal(t, "B", nodes[0].SpanID)
	assert.Empty(t, edges, "no edge: parent A was skipped")
}

// ── 10. Unknown span WITH agent name is included ──────────────────────────────

func TestInferTopology_UnknownSpan_WithAgentName_Included(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "unknown", "MyAgent", ""),
	}

	nodes, edges := InferTopology(spans)

	require.Len(t, nodes, 1)
	assert.Equal(t, nodeTypeAgent, nodes[0].NodeType)
	assert.Equal(t, "MyAgent", nodes[0].NodeName)
	assert.Empty(t, edges)
}

// ── 11. Node naming fallbacks ─────────────────────────────────────────────────

func TestInferTopology_NodeNaming_LLMFallsBackToSpanName(t *testing.T) {
	s := span("A", "", "llm.call", "", "") // no model_id
	s.SpanName = "my-llm-call"
	nodes, _ := InferTopology([]enrichedSpan{s})
	require.Len(t, nodes, 1)
	assert.Equal(t, "my-llm-call", nodes[0].NodeName)
}

func TestInferTopology_NodeNaming_ToolFallsBackToSpanName(t *testing.T) {
	s := span("A", "", "tool.call", "", "") // no tool name
	s.SpanName = "my-tool-call"
	nodes, _ := InferTopology([]enrichedSpan{s})
	require.Len(t, nodes, 1)
	assert.Equal(t, "my-tool-call", nodes[0].NodeName)
}

func TestInferTopology_NodeNaming_MemoryAlwaysNamedMemory(t *testing.T) {
	spans := []enrichedSpan{
		span("A", "", "memory.read", "", ""),
		span("B", "", "memory.write", "", ""),
	}
	nodes, _ := InferTopology(spans)
	require.Len(t, nodes, 2)
	for _, n := range nodes {
		assert.Equal(t, "memory", n.NodeName)
		assert.Equal(t, nodeTypeMemory, n.NodeType)
	}
}

// ── 12. Status code mapping ───────────────────────────────────────────────────

func TestInferTopology_StatusCodeMapping(t *testing.T) {
	cases := []struct {
		code   string
		expect string
	}{
		{"STATUS_CODE_OK", statusOK},
		{"Ok", statusOK},
		{"OK", statusOK},
		{"STATUS_CODE_ERROR", statusError},
		{"Error", statusError},
		{"ERROR", statusError},
		{"", statusUnset},
		{"UNSET", statusUnset},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			s := span("A", "", "llm.call", "", "gpt-4o")
			s.StatusCode = tc.code
			nodes, _ := InferTopology([]enrichedSpan{s})
			require.Len(t, nodes, 1)
			assert.Equal(t, tc.expect, nodes[0].Status)
		})
	}
}

// ── 13. Cost and token counts on nodes ───────────────────────────────────────

func TestInferTopology_CostAndTokens(t *testing.T) {
	s := span("A", "", "llm.call", "", "gpt-4o")
	s.CostUSD = 0.0123
	s.InputTokens = 1000
	s.OutputTokens = 500

	nodes, _ := InferTopology([]enrichedSpan{s})
	require.Len(t, nodes, 1)
	assert.InDelta(t, 0.0123, nodes[0].CostUSD, 0.0001)
	assert.Equal(t, 1500, nodes[0].TokenCount) // input + output
}

// ── 14. Complex multi-agent scenario ─────────────────────────────────────────
//
//	Supervisor → (handoff) → Researcher
//	Researcher → llm.call (gpt-4o)
//	Researcher → tool.call (web_search)
//	Researcher → (handoff) → Writer
//	Writer → llm.call (claude-sonnet)
func TestInferTopology_MultiAgentScenario(t *testing.T) {
	spans := []enrichedSpan{
		span("supervisor", "", "agent.handoff", "Supervisor", ""),
		span("handoff-1", "supervisor", "agent.handoff", "Researcher", ""),
		span("llm-1", "handoff-1", "llm.call", "", "gpt-4o"),
		span("tool-1", "handoff-1", "tool.call", "", ""),
		span("handoff-2", "handoff-1", "agent.handoff", "Writer", ""),
		span("llm-2", "handoff-2", "llm.call", "", "claude-sonnet"),
	}
	spans[3].ToolName = "web_search"

	nodes, edges := InferTopology(spans)

	assert.Len(t, nodes, 6)
	assert.Len(t, edges, 5)

	// Handoff edges
	_, ok := edgeBetween(edges, "supervisor", "handoff-1")
	assert.True(t, ok, "supervisor → handoff-1")

	_, ok = edgeBetween(edges, "handoff-1", "handoff-2")
	assert.True(t, ok, "researcher → writer handoff")

	// Invocation edges from researcher
	_, ok = edgeBetween(edges, "handoff-1", "llm-1")
	assert.True(t, ok, "researcher → gpt-4o")

	_, ok = edgeBetween(edges, "handoff-1", "tool-1")
	assert.True(t, ok, "researcher → web_search")

	// Writer's LLM call
	_, ok = edgeBetween(edges, "handoff-2", "llm-2")
	assert.True(t, ok, "writer → claude-sonnet")
}
