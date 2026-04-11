package domain

import "time"

// PlaygroundMessage represents a single message in a playground variant.
type PlaygroundMessage struct {
	Role    string `json:"role"`    // "system", "user", or "assistant"
	Content string `json:"content"`
}

// PlaygroundSession groups one or more prompt variants for side-by-side comparison.
type PlaygroundSession struct {
	ID           string
	ProjectID    string
	Name         string
	SourceSpanID *string
	SourceRunID  *string
	Variants     []*PlaygroundVariant  // populated on Get, nil on List
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PlaygroundVariant is a single prompt configuration within a session.
type PlaygroundVariant struct {
	ID          string
	SessionID   string
	Label       string
	ModelID     string
	System      string // system prompt text
	Messages    []PlaygroundMessage
	Temperature *float32
	MaxTokens   *int
	Executions  []*PlaygroundExecution // most recent N, populated on Get
	UpdatedAt   time.Time
}

// PlaygroundExecution records one invocation of a variant against a provider.
type PlaygroundExecution struct {
	ID           string
	VariantID    string
	Output       *string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	LatencyMS    int
	Error        *string
	CreatedAt    time.Time
}
