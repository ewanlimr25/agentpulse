package domain

import "time"

// ExportParams holds the filters for a data export request.
type ExportParams struct {
	ProjectID string
	From      time.Time
	To        time.Time
	AgentName string // optional filter
	Model     string // optional filter
}

// ExportSpanRow represents a single span row in an export.
type ExportSpanRow struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	RunID         string
	AgentSpanKind string
	AgentName     string
	ModelID       string
	SpanName      string
	ServiceName   string
	StatusCode    string
	StatusMessage string
	StartTime     time.Time
	EndTime       time.Time
	DurationMS    float64
	InputTokens   uint32
	OutputTokens  uint32
	TotalTokens   uint32
	CostUSD       float64
}

// ExportRunRow represents a single run row in an export.
type ExportRunRow struct {
	RunID        string
	TraceID      string
	SessionID    string
	UserID       string
	StartTime    time.Time
	EndTime      time.Time
	SpanCount    uint64
	LLMCalls     uint64
	ToolCalls    uint64
	InputTokens  uint64
	OutputTokens uint64
	TotalTokens  uint64
	TotalCostUSD float64
	ErrorCount   uint64
}
