package domain

import "time"

// SignalType identifies which metric an AlertRule monitors.
type SignalType string

const (
	SignalTypeErrorRate    SignalType = "error_rate"    // % of runs with errors
	SignalTypeLatencyP95   SignalType = "latency_p95"   // p95 run duration in ms
	SignalTypeQualityScore SignalType = "quality_score" // avg eval score (0.0–1.0)
	SignalTypeToolFailure  SignalType = "tool_failure"  // % of tool spans with error
	SignalTypeAgentLoop    SignalType = "agent_loop"    // number of looping runs in window
)

// CompareOp is the comparison direction for threshold evaluation.
type CompareOp string

const (
	CompareOpGt CompareOp = "gt" // alert when current > threshold
	CompareOpLt CompareOp = "lt" // alert when current < threshold
)

// AlertRule defines a signal-based threshold for a project.
type AlertRule struct {
	ID        string
	ProjectID string

	Name       string
	SignalType SignalType
	Threshold  float64   // unit depends on SignalType: %, ms, or 0–1 score
	CompareOp  CompareOp

	WindowSeconds int     // rolling window for signal computation
	ScopeFilter   *string // tool name — required when SignalType is tool_failure

	WebhookURL *string
	Enabled    bool

	SlackWebhookURL    *string
	DiscordWebhookURL  *string
	LastChannelError   *string
	LastChannelErrorAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AlertEvent records a threshold breach for an AlertRule.
type AlertEvent struct {
	ID        string
	RuleID    string
	ProjectID string

	TriggeredAt  time.Time
	SignalType   SignalType
	CurrentValue float64   // measured value at trigger time
	Threshold    float64
	CompareOp    CompareOp
	ActionTaken  string

	Metadata map[string]any
}

// RecentAlertEvent is a cross-project alert enriched with project and rule names.
type RecentAlertEvent struct {
	AlertEvent
	ProjectName string
	RuleName    string
}
