package domain

import "time"

// RunLoop records a detected agent loop for a run.
type RunLoop struct {
	ID              string
	RunID           string
	ProjectID       string
	DetectionType   string    // "repeated_call" | "topology_cycle"
	SpanName        string
	InputHash       string
	OutputHash      string
	Confidence      string    // "high" | "low"
	OccurrenceCount int
	DetectedAt      time.Time
}

// RepeatedToolCall is an intermediate result from ClickHouse loop detection.
type RepeatedToolCall struct {
	RunID      string
	SpanName   string
	InputHash  string
	OutputHash string
	Count      int
	Confidence string // "high" | "low"
}
