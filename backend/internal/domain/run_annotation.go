package domain

import "time"

// RunAnnotation represents a free-text note attached to a run.
// At most one annotation exists per (project_id, run_id).
type RunAnnotation struct {
	ID        string
	ProjectID string
	RunID     string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
