package domain

import "time"

// Session represents a group of runs belonging to the same conversation.
// Aggregates are computed from ClickHouse by the session_agg materialized view.
type Session struct {
	SessionID   string
	ProjectID   string

	RunCount    uint64
	TotalCostUSD float64
	TotalTokens  uint64
	InputTokens  uint64
	OutputTokens uint64
	ErrorCount   uint64

	FirstRunAt time.Time
	LastRunAt  time.Time
}
