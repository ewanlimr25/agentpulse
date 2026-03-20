package topologyexporter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

// ── Mock store ────────────────────────────────────────────────────────────────

type mockStore struct {
	nodes []topologyNode
	edges []topologyEdge
}

func (m *mockStore) UpsertNodes(_ context.Context, _ string, nodes []topologyNode) (map[string]string, error) {
	m.nodes = append(m.nodes, nodes...)
	ids := make(map[string]string, len(nodes))
	for _, n := range nodes {
		ids[n.SpanID] = "uuid-" + n.SpanID
	}
	return ids, nil
}

func (m *mockStore) UpsertEdges(_ context.Context, _ string, edges []topologyEdge, _ map[string]string) error {
	m.edges = append(m.edges, edges...)
	return nil
}

func (m *mockStore) Close() {}

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeOTelSpan(traceID, spanID, parentSpanID string, attrs map[string]string) ptrace.Span {
	span := ptrace.NewSpan()
	var tid pcommon.TraceID
	copy(tid[:], []byte(traceID))
	var sid pcommon.SpanID
	copy(sid[:], []byte(spanID))
	span.SetTraceID(tid)
	span.SetSpanID(sid)
	if parentSpanID != "" {
		var pid pcommon.SpanID
		copy(pid[:], []byte(parentSpanID))
		span.SetParentSpanID(pid)
	}
	span.SetName("test-span")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(50 * time.Millisecond)))
	for k, v := range attrs {
		span.Attributes().PutStr(k, v)
	}
	return span
}

func makeTracesFromSpans(projectID string, spans []ptrace.Span) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("agentpulse.project_id", projectID)
	ss := rs.ScopeSpans().AppendEmpty()
	for _, s := range spans {
		dest := ss.Spans().AppendEmpty()
		s.CopyTo(dest)
	}
	return td
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestExporter_ConsumeTraces_PersistsNodesAndEdges(t *testing.T) {
	store := &mockStore{}
	exp := newTopologyExporter(defaultConfig(), zaptest.NewLogger(t), store)

	s1 := makeOTelSpan("trace1", "aaaaaaaa", "", map[string]string{
		"agentpulse.span_kind":  "agent.handoff",
		"agentpulse.agent.name": "Supervisor",
		"agentpulse.run_id":     "run-1",
	})
	s2 := makeOTelSpan("trace1", "bbbbbbbb", "aaaaaaaa", map[string]string{
		"agentpulse.span_kind": "llm.call",
		"agentpulse.model_id":  "gpt-4o",
		"agentpulse.run_id":    "run-1",
	})

	td := makeTracesFromSpans("proj-1", []ptrace.Span{s1, s2})
	require.NoError(t, exp.ConsumeTraces(context.Background(), td))

	assert.Len(t, store.nodes, 2)
	assert.Len(t, store.edges, 1)
}

func TestExporter_ConsumeTraces_EmptyTraces_NoPersistence(t *testing.T) {
	store := &mockStore{}
	exp := newTopologyExporter(defaultConfig(), zaptest.NewLogger(t), store)

	require.NoError(t, exp.ConsumeTraces(context.Background(), ptrace.NewTraces()))
	assert.Empty(t, store.nodes)
	assert.Empty(t, store.edges)
}

func TestExporter_ConsumeTraces_AllUnknownSpans_NoPersistence(t *testing.T) {
	store := &mockStore{}
	exp := newTopologyExporter(defaultConfig(), zaptest.NewLogger(t), store)

	// Spans with unknown kind and no agent name — all skipped
	s := makeOTelSpan("trace1", "aaaaaaaa", "", map[string]string{
		"agentpulse.span_kind": "unknown",
	})
	td := makeTracesFromSpans("proj-1", []ptrace.Span{s})
	require.NoError(t, exp.ConsumeTraces(context.Background(), td))

	assert.Empty(t, store.nodes)
	assert.Empty(t, store.edges)
}

func TestExporter_ConsumeTraces_MultiProject_GroupedCorrectly(t *testing.T) {
	store := &mockStore{}
	exp := newTopologyExporter(defaultConfig(), zaptest.NewLogger(t), store)

	// Two spans with different project IDs in the same batch
	td := ptrace.NewTraces()

	rs1 := td.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("agentpulse.project_id", "proj-A")
	ss1 := rs1.ScopeSpans().AppendEmpty()
	s1 := ss1.Spans().AppendEmpty()
	s1.SetName("span-1")
	s1.Attributes().PutStr("agentpulse.span_kind", "llm.call")
	s1.Attributes().PutStr("agentpulse.model_id", "gpt-4o")
	s1.Attributes().PutStr("agentpulse.run_id", "run-A")

	rs2 := td.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("agentpulse.project_id", "proj-B")
	ss2 := rs2.ScopeSpans().AppendEmpty()
	s2 := ss2.Spans().AppendEmpty()
	s2.SetName("span-2")
	s2.Attributes().PutStr("agentpulse.span_kind", "tool.call")
	s2.Attributes().PutStr("tool.name", "calculator")
	s2.Attributes().PutStr("agentpulse.run_id", "run-B")

	require.NoError(t, exp.ConsumeTraces(context.Background(), td))

	// Both spans become nodes (one per project)
	assert.Len(t, store.nodes, 2)

	projectIDs := make(map[string]bool)
	for _, n := range store.nodes {
		projectIDs[n.ProjectID] = true
	}
	assert.True(t, projectIDs["proj-A"])
	assert.True(t, projectIDs["proj-B"])
}
