package domain

import "time"

// SearchResult represents a single span that matched a full-text search query.
type SearchResult struct {
	TraceID       string
	SpanID        string
	RunID         string
	ProjectID     string
	SpanName      string
	AgentSpanKind AgentSpanKind
	AgentName     string
	ModelID       string
	StatusCode    StatusCode
	StartTime     time.Time
	DurationNS    uint64
	InputTokens   uint32
	OutputTokens  uint32
	TotalTokens   uint32
	CostUSD       float64
	MatchedField  string // "gen_ai.prompt" | "gen_ai.completion" | "tool.input" | "tool.output"
	Snippet       string // ~200-char window around the match
}

// SearchParams holds the query parameters for a full-text span search.
type SearchParams struct {
	ProjectID string
	Query     string        // raw user input; handler escapes LIKE metacharacters before passing
	SpanKind  AgentSpanKind // optional; empty means all
	From      time.Time     // optional; zero means no lower bound
	To        time.Time     // optional; zero means no upper bound
	Limit     int
	Offset    int
}
