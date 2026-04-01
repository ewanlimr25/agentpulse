package domain

import "time"

// SpanFeedback records a human rating for an individual span output.
// One feedback record is allowed per (project_id, span_id) — upsert semantics
// so a reviewer can change their mind.
type SpanFeedback struct {
	ID        string
	ProjectID string
	SpanID    string
	RunID     string

	// Rating is "good" or "bad".
	Rating string

	// CorrectedOutput is an optional plain-text ground-truth correction.
	// Nil means no correction was provided. Capped at 10 000 chars.
	CorrectedOutput *string

	CreatedAt time.Time
	UpdatedAt time.Time
}
