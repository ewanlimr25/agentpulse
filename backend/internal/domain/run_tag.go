package domain

import "time"

// RunTag represents a single tag attached to a run.
type RunTag struct {
	ID        string
	ProjectID string
	RunID     string
	Tag       string
	CreatedAt time.Time
}
