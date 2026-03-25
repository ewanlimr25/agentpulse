package clickhouseexporter

import (
	"encoding/json"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// spanRow is the flattened representation of a span ready for ClickHouse insertion.
type spanRow struct {
	TraceID      string
	SpanID       string
	ParentSpanID string

	RunID     string
	ProjectID string
	SessionID string
	UserID    string

	AgentSpanKind string
	AgentName     string
	ModelID       string

	SpanName      string
	ServiceName   string
	StatusCode    string
	StatusMessage string

	StartTime time.Time
	EndTime   time.Time

	InputTokens  uint32
	OutputTokens uint32
	CostUSD      float64

	Attributes    map[string]string
	ResourceAttrs map[string]string
	Events        string // JSON
}

// spanRowFromOTel converts an OTel span + resource attributes into a spanRow.
func spanRowFromOTel(span ptrace.Span, resource pcommon.Resource, projectID string) spanRow {
	attrs := span.Attributes()
	resourceAttrs := resource.Attributes()

	row := spanRow{
		TraceID:       span.TraceID().String(),
		SpanID:        span.SpanID().String(),
		ParentSpanID:  span.ParentSpanID().String(),
		ProjectID:     projectID,
		SpanName:      span.Name(),
		StatusCode:    span.Status().Code().String(),
		StatusMessage: span.Status().Message(),
		StartTime:     span.StartTimestamp().AsTime(),
		EndTime:       span.EndTimestamp().AsTime(),
		Attributes:    attrsToMap(attrs),
		ResourceAttrs: attrsToMap(resourceAttrs),
	}

	// Fields written by agentsemanticproc
	row.RunID = strAttr(attrs, "agentpulse.run_id")
	row.SessionID = strAttr(attrs, "agentpulse.session_id")
	row.UserID = strAttr(attrs, "agentpulse.user_id")
	row.AgentSpanKind = strAttr(attrs, "agentpulse.span_kind")
	row.AgentName = strAttr(attrs, "agentpulse.agent.name")
	row.ModelID = strAttr(attrs, "agentpulse.model_id")
	row.InputTokens = uint32(intAttr(attrs, "agentpulse.input_tokens"))
	row.OutputTokens = uint32(intAttr(attrs, "agentpulse.output_tokens"))
	row.CostUSD = floatAttr(attrs, "agentpulse.cost_usd")

	// service.name lives in resource attributes
	row.ServiceName = strAttr(resourceAttrs, "service.name")

	// project_id can also be supplied as a resource attribute (takes precedence)
	if pid := strAttr(resourceAttrs, "agentpulse.project_id"); pid != "" {
		row.ProjectID = pid
	}

	row.Events = eventsToJSON(span.Events())

	return row
}

// attrsToMap converts a pcommon.Map to a plain map[string]string.
func attrsToMap(m pcommon.Map) map[string]string {
	out := make(map[string]string, m.Len())
	m.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsString()
		return true
	})
	return out
}

// eventsToJSON serialises span events to a JSON string for storage.
func eventsToJSON(events ptrace.SpanEventSlice) string {
	type event struct {
		Name       string            `json:"name"`
		Timestamp  string            `json:"timestamp"`
		Attributes map[string]string `json:"attributes,omitempty"`
	}
	out := make([]event, 0, events.Len())
	for i := range events.Len() {
		e := events.At(i)
		out = append(out, event{
			Name:       e.Name(),
			Timestamp:  e.Timestamp().AsTime().UTC().Format(time.RFC3339Nano),
			Attributes: attrsToMap(e.Attributes()),
		})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

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
