package domain

import "time"

// SpanEval holds a single quality score for an LLM span.
type SpanEval struct {
	ProjectID   string
	RunID       string
	SpanID      string
	EvalName    string
	Score       float32 // 0.0 – 1.0
	Reasoning   string
	JudgeModel  string
	EvalVersion uint16
	CreatedAt   time.Time
}

// RunEvalSummary aggregates quality scores for one eval type across all spans in a run.
type RunEvalSummary struct {
	RunID     string
	EvalName  string  // e.g. "relevance", "hallucination"
	AvgScore  float32
	SpanCount int
}

// EvalConfig holds per-project configuration for a single eval type.
type EvalConfig struct {
	ID             string
	ProjectID      string
	EvalName       string  // built-in name or "custom:<name>"
	Enabled        bool
	SpanKind       string  // "llm.call" or "tool.call"
	PromptTemplate *string // nil = use built-in Go implementation
	PromptVersion  int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// EvalJob is a pending unit of work in the eval queue.
type EvalJob struct {
	ID        string
	SpanID    string
	RunID     string
	ProjectID string
	EvalName  string
	Attempts  int
}
