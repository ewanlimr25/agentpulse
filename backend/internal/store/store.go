// Package store defines the repository interfaces used by the API handlers.
// Concrete implementations live in the clickhouse/ and postgres/ sub-packages.
package store

import (
	"context"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// SpanStore reads spans from ClickHouse.
type SpanStore interface {
	// ListByRun returns all spans for a given run, ordered by start time.
	ListByRun(ctx context.Context, runID string) ([]*domain.Span, error)
}

// RunStore queries per-run aggregated metrics from ClickHouse.
type RunStore interface {
	// List returns runs for a project, newest first, paginated.
	List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Run, error)
	// Count returns the total number of runs for a project.
	Count(ctx context.Context, projectID string) (int, error)
	// Get returns a single run with its aggregated metrics.
	Get(ctx context.Context, runID string) (*domain.Run, error)
}

// TopologyStore reads topology graphs from Postgres.
type TopologyStore interface {
	// GetByRun returns the full DAG (nodes + edges) for a run.
	GetByRun(ctx context.Context, runID string) (*domain.Topology, error)
}

// ProjectStore manages project records in Postgres.
type ProjectStore interface {
	List(ctx context.Context) ([]*domain.Project, error)
	Get(ctx context.Context, id string) (*domain.Project, error)
	Create(ctx context.Context, p *domain.Project) error
	// GetByAPIKeyHash looks up a project by the SHA-256 hex hash of its API key.
	// Returns an error wrapping pgx.ErrNoRows if not found.
	GetByAPIKeyHash(ctx context.Context, hash string) (*domain.Project, error)
}

// BudgetStore manages budget rules and alerts in Postgres.
type BudgetStore interface {
	ListRules(ctx context.Context, projectID string) ([]*domain.BudgetRule, error)
	GetRule(ctx context.Context, id string) (*domain.BudgetRule, error)
	CreateRule(ctx context.Context, r *domain.BudgetRule) error
	UpdateRule(ctx context.Context, r *domain.BudgetRule) error
	DeleteRule(ctx context.Context, id string) error
	ListAlerts(ctx context.Context, projectID string, limit int) ([]*domain.BudgetAlert, error)
	ListRecentAlerts(ctx context.Context, limit int) ([]*domain.RecentBudgetAlert, error)
}

// EvalStore writes and reads quality scores.
type EvalStore interface {
	// Insert writes a score to ClickHouse.
	Insert(ctx context.Context, e *domain.SpanEval) error
	// ListByRun returns all evals for all spans in a run.
	ListByRun(ctx context.Context, runID string) ([]*domain.SpanEval, error)
	// SummaryByProject returns avg score per run for a project.
	SummaryByProject(ctx context.Context, projectID string) ([]*domain.RunEvalSummary, error)
}

// AlertRuleStore manages signal-based alert rules and events in Postgres.
type AlertRuleStore interface {
	ListRules(ctx context.Context, projectID string) ([]*domain.AlertRule, error)
	GetRule(ctx context.Context, id string) (*domain.AlertRule, error)
	CreateRule(ctx context.Context, r *domain.AlertRule) error
	UpdateRule(ctx context.Context, r *domain.AlertRule) error
	DeleteRule(ctx context.Context, id string) error
	// ListEnabledRules returns all enabled rules across all projects (used by evaluator).
	ListEnabledRules(ctx context.Context) ([]*domain.AlertRule, error)
	ListEvents(ctx context.Context, projectID string, limit int) ([]*domain.AlertEvent, error)
	CreateEvent(ctx context.Context, e *domain.AlertEvent) error
	// LastEventForRule returns the most recent event for a rule, or nil if none.
	LastEventForRule(ctx context.Context, ruleID string) (*domain.AlertEvent, error)
	ListRecentEvents(ctx context.Context, limit int) ([]*domain.RecentAlertEvent, error)
}

// EvalJobStore manages the async eval work queue in Postgres.
type EvalJobStore interface {
	// Enqueue inserts a job; silently ignores duplicates (span_id, eval_name).
	Enqueue(ctx context.Context, jobs []*domain.EvalJob) error
	// Claim atomically claims up to n pending jobs for processing.
	Claim(ctx context.Context, n int) ([]*domain.EvalJob, error)
	// MarkDone marks a job as successfully completed.
	MarkDone(ctx context.Context, id string) error
	// MarkFailed marks a job as failed with an error message.
	MarkFailed(ctx context.Context, id, errMsg string) error
}

// LoopStore manages detected agent loops in Postgres.
type LoopStore interface {
	Upsert(ctx context.Context, loop *domain.RunLoop) error
	ListByRun(ctx context.Context, runID string) ([]*domain.RunLoop, error)
	HasLoops(ctx context.Context, runIDs []string) (map[string]bool, error)
	CountByProject(ctx context.Context, projectID string, windowSeconds int) (int, error)
}

// AnalyticsStore queries per-tool and per-agent aggregates from ClickHouse.
type AnalyticsStore interface {
	// ToolStats returns per-tool aggregates for tool.call spans within windowSeconds.
	ToolStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.ToolStats, error)
	// AgentCostStats returns per-agent cost breakdown within windowSeconds.
	AgentCostStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.AgentCostStats, error)
}
