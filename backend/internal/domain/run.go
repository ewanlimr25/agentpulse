package domain

import "time"

// RunComparison holds two runs and their associated topology and eval data
// for side-by-side comparison.
type RunComparison struct {
	RunA      *Run
	RunB      *Run
	TopologyA *Topology
	TopologyB *Topology
	EvalsA    []*SpanEval
	EvalsB    []*SpanEval
}

// Run represents a single agent execution run with aggregated metrics.
// It is derived from spans stored in ClickHouse via a materialized view.
type Run struct {
	RunID     string
	ProjectID string
	TraceID   string

	StartTime time.Time
	EndTime   time.Time
	DurationMS float64

	SpanCount    uint64
	LLMCallCount uint64
	ToolCallCount uint64

	TotalInputTokens  uint64
	TotalOutputTokens uint64
	TotalTokens       uint64
	TotalCostUSD      float64

	// ErrorCount is the number of spans with status ERROR.
	ErrorCount uint64
	// Status is "error" if any span errored, otherwise "ok".
	Status string

	// SessionID groups this run into a multi-turn session (empty if not set).
	SessionID string

	// UserID is the end-user identifier extracted from spans via anyLast().
	// Best-effort for display purposes — use user_agg for authoritative cost attribution.
	UserID string

	// LoopDetected is true when the background loop detector has flagged this run.
	LoopDetected bool

	// Streaming span metrics — zero when no streaming spans exist in the run.
	TtftP50Ms          float64
	TtftP95Ms          float64
	StreamingSpanCount uint64

	// IsActive is true when the run has had span activity within the last 30 seconds.
	// Populated at the API layer; not stored in ClickHouse.
	IsActive bool

	// Tags is the list of tag strings attached to this run.
	// Populated at the API layer from Postgres; never stored in ClickHouse.
	Tags []string `json:"tags,omitempty"`

	// Annotation is the free-text note attached to this run, or nil if none exists.
	// Populated at the API layer from Postgres; never stored in ClickHouse.
	Annotation *string `json:"annotation,omitempty"`
}
