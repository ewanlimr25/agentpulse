package topologyexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// topologyExporter converts OTel span batches to topology graphs in Postgres.
type topologyExporter struct {
	cfg    *Config
	logger *zap.Logger
	store  topologyStore
}

func newTopologyExporter(cfg *Config, logger *zap.Logger, store topologyStore) *topologyExporter {
	return &topologyExporter{cfg: cfg, logger: logger, store: store}
}

func (e *topologyExporter) Start(_ context.Context, _ component.Host) error { return nil }

func (e *topologyExporter) Shutdown(_ context.Context) error {
	e.store.Close()
	return nil
}

// ConsumeTraces extracts topology from a span batch and persists it to Postgres.
func (e *topologyExporter) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	spans := extractSpans(td)
	if len(spans) == 0 {
		return nil
	}

	// Group by project_id for multi-tenant correctness.
	byProject := groupByProject(spans)

	for projectID, projectSpans := range byProject {
		nodes, edges := InferTopology(projectSpans)
		if len(nodes) == 0 {
			continue
		}

		nodeIDs, err := e.store.UpsertNodes(ctx, projectID, nodes)
		if err != nil {
			e.logger.Error("failed to upsert topology nodes",
				zap.String("project_id", projectID),
				zap.Int("nodes", len(nodes)),
				zap.Error(err),
			)
			continue
		}

		if err := e.store.UpsertEdges(ctx, projectID, edges, nodeIDs); err != nil {
			e.logger.Error("failed to upsert topology edges",
				zap.String("project_id", projectID),
				zap.Int("edges", len(edges)),
				zap.Error(err),
			)
		}
	}
	return nil
}

// extractSpans converts ptrace.Traces into a flat slice of enrichedSpan.
func extractSpans(td ptrace.Traces) []enrichedSpan {
	var out []enrichedSpan
	for i := range td.ResourceSpans().Len() {
		rs := td.ResourceSpans().At(i)
		resourceProjectID := firstNonEmpty(
			strAttr(rs.Resource().Attributes(), "agentpulse.project_id"),
			strAttr(rs.Resource().Attributes(), "agentpulse.project.id"),
		)

		for j := range rs.ScopeSpans().Len() {
			ss := rs.ScopeSpans().At(j)
			for k := range ss.Spans().Len() {
				span := ss.Spans().At(k)
				out = append(out, spanToEnriched(span, rs.Resource(), resourceProjectID))
			}
		}
	}
	return out
}

// spanToEnriched maps an OTel span to the enrichedSpan type used by inference.
func spanToEnriched(span ptrace.Span, resource pcommon.Resource, projectID string) enrichedSpan {
	attrs := span.Attributes()

	// project_id: prefer span attribute (written by agentsemanticproc), fall back to resource
	projectID = firstNonEmpty(
		strAttr(attrs, "agentpulse.project_id"),
		strAttr(attrs, "agentpulse.project.id"),
		projectID,
	)

	return enrichedSpan{
		SpanID:        span.SpanID().String(),
		ParentSpanID:  span.ParentSpanID().String(),
		SpanName:      span.Name(),
		RunID:         firstNonEmpty(strAttr(attrs, "agentpulse.run_id"), strAttr(attrs, "agentpulse.run.id")),
		ProjectID:     projectID,
		AgentSpanKind: firstNonEmpty(strAttr(attrs, "agentpulse.span_kind"), strAttr(attrs, "agentpulse.span.kind")),
		AgentName:     strAttr(attrs, "agentpulse.agent.name"),
		ModelID:       strAttr(attrs, "agentpulse.model_id"),
		ToolName:      strAttr(attrs, "tool.name"),
		MCPServerName: strAttr(attrs, "agentpulse.mcp.server_name"),
		CostUSD:       floatAttr(attrs, "agentpulse.cost_usd"),
		InputTokens:   uint32(intAttr(attrs, "agentpulse.input_tokens")),
		OutputTokens:  uint32(intAttr(attrs, "agentpulse.output_tokens")),
		StatusCode:    span.Status().Code().String(),
		StartTime:     span.StartTimestamp().AsTime(),
		EndTime:       span.EndTimestamp().AsTime(),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// groupByProject partitions spans by project_id, dropping spans with no project_id.
func groupByProject(spans []enrichedSpan) map[string][]enrichedSpan {
	out := make(map[string][]enrichedSpan)
	for _, s := range spans {
		if s.ProjectID == "" {
			continue
		}
		out[s.ProjectID] = append(out[s.ProjectID], s)
	}
	return out
}

// ── Attribute helpers (mirrored from clickhouseexporter to avoid coupling) ───

func strAttr(m pcommon.Map, key string) string {
	if v, ok := m.Get(key); ok {
		return v.AsString()
	}
	return ""
}

func intAttr(m pcommon.Map, key string) int64 {
	if v, ok := m.Get(key); ok {
		switch v.Type() {
		case pcommon.ValueTypeInt:
			return v.Int()
		case pcommon.ValueTypeDouble:
			return int64(v.Double())
		}
	}
	return 0
}

func floatAttr(m pcommon.Map, key string) float64 {
	if v, ok := m.Get(key); ok {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			return v.Double()
		case pcommon.ValueTypeInt:
			return float64(v.Int())
		}
	}
	return 0
}
