package domain

import "time"

// ProjectHealth reports whether a project's collector pipeline is actively receiving spans.
type ProjectHealth struct {
	// CollectorReachable is true when a span was received within the last 5 minutes.
	CollectorReachable bool
	// LastSpanAt is the timestamp of the most recent span, or nil if no spans exist.
	LastSpanAt *time.Time
	// SpanCount is the total number of spans received for this project.
	SpanCount int64
	// SpansPerMinute is the count of spans received in the last 60 seconds.
	SpansPerMinute int64
}
