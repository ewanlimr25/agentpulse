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

// EvalJob is a pending unit of work in the eval queue.
type EvalJob struct {
	ID        string
	SpanID    string
	RunID     string
	ProjectID string
	EvalName  string
	Attempts  int
}
