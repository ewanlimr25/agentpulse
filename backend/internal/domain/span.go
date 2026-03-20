package domain

import "time"

// AgentSpanKind classifies the semantic role of a span in an agent execution.
type AgentSpanKind string

const (
	SpanKindLLMCall      AgentSpanKind = "llm.call"
	SpanKindToolCall     AgentSpanKind = "tool.call"
	SpanKindAgentHandoff AgentSpanKind = "agent.handoff"
	SpanKindMemoryRead   AgentSpanKind = "memory.read"
	SpanKindMemoryWrite  AgentSpanKind = "memory.write"
	SpanKindUnknown      AgentSpanKind = "unknown"
)

// StatusCode mirrors OTel span status codes.
type StatusCode string

const (
	StatusOK    StatusCode = "OK"
	StatusError StatusCode = "ERROR"
	StatusUnset StatusCode = "UNSET"
)

// Span is the core domain entity representing a single OTel span enriched with
// agent semantic fields.
type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string

	RunID     string
	ProjectID string

	AgentSpanKind AgentSpanKind
	AgentName     string
	ModelID       string

	SpanName      string
	ServiceName   string
	StatusCode    StatusCode
	StatusMessage string

	StartTime  time.Time
	EndTime    time.Time
	DurationNS uint64

	InputTokens  uint32
	OutputTokens uint32
	TotalTokens  uint32
	CostUSD      float64

	Attributes    map[string]string
	ResourceAttrs map[string]string
	Events        []SpanEvent
}

// SpanEvent represents a timed annotation on a span.
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]string
}
