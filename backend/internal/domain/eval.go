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
	EvalName       string             // built-in name or "custom:<name>"
	Enabled        bool
	SpanKind       string             // "llm.call" or "tool.call"
	PromptTemplate *string            // nil = use built-in Go implementation
	PromptVersion  int
	ScopeFilter    map[string][]string // nil = match all spans; {"agent_name": ["researcher"]} = only that agent
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// EvalTypeBaseline holds per-eval-type baseline stats across the last N runs.
type EvalTypeBaseline struct {
	EvalName  string  `json:"eval_name"`
	AvgScore  float32 `json:"avg_score"`
	SpanCount int     `json:"span_count"`
	RunCount  int     `json:"run_count"` // how many of the N runs contributed scores for this type
}

// EvalBaseline is the response for GET /projects/{id}/evals/baseline.
// OverallScore is the unweighted average of per-type averages — informational only;
// CI gates should use per-type thresholds via the CLI --eval-type flag.
type EvalBaseline struct {
	ProjectID      string             `json:"project_id"`
	RunsConsidered int                `json:"runs_considered"`
	Types          []EvalTypeBaseline `json:"types"`
	OverallScore   float32            `json:"overall_score"`
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
