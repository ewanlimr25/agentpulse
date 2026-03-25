package domain

import "time"

// BudgetAction defines what happens when a threshold is breached.
type BudgetAction string

const (
	BudgetActionNotify BudgetAction = "notify"
	BudgetActionHalt   BudgetAction = "halt"
)

// BudgetScope defines what the cost accumulator applies to.
type BudgetScope string

const (
	BudgetScopeRun    BudgetScope = "run"
	BudgetScopeAgent  BudgetScope = "agent"
	BudgetScopeWindow BudgetScope = "window"
	BudgetScopeUser   BudgetScope = "user"
)

// BudgetRule defines a cost threshold for a project.
type BudgetRule struct {
	ID        string
	ProjectID string

	Name         string
	ThresholdUSD float64
	Action       BudgetAction

	Scope         BudgetScope
	WindowSeconds *int

	WebhookURL *string
	Enabled    bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// RecentBudgetAlert is a cross-project alert enriched with project and rule names.
type RecentBudgetAlert struct {
	BudgetAlert
	ProjectName string
	RuleName    string
}

// BudgetAlert records a threshold breach event.
type BudgetAlert struct {
	ID        string
	RuleID    string
	ProjectID string
	RunID     *string

	TriggeredAt  time.Time
	CurrentCost  float64
	ThresholdUSD float64
	ActionTaken  string

	Metadata map[string]any
}
