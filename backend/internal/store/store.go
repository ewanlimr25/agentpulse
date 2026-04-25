// Package store defines the repository interfaces used by the API handlers.
// Concrete implementations live in the clickhouse/ and postgres/ sub-packages.
package store

import (
	"context"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// SpanStore reads spans from ClickHouse.
type SpanStore interface {
	// ListByRun returns all spans for a given run, ordered by start time.
	ListByRun(ctx context.Context, runID string) ([]*domain.Span, error)
	// ListLLMSpansByRun returns only LLM call spans for a run, ordered by start time.
	ListLLMSpansByRun(ctx context.Context, runID string) ([]*domain.Span, error)
	// GetByID returns a single span filtered by both project_id and span_id.
	// Returns a not-found error (wrapping a sentinel) if no span matches.
	// The projectID parameter is required for security — prevents cross-project access.
	GetByID(ctx context.Context, projectID, spanID string) (*domain.Span, error)
	// LatestSpanTime returns the timestamp of the most recent span for a project,
	// or nil if no spans exist. The query is bounded to the last 24 hours to avoid
	// full-table scans.
	LatestSpanTime(ctx context.Context, projectID string) (*time.Time, error)
	// ListByRunSince returns spans for a run with start_time > since, ordered by start_time ASC.
	// Bounded to _date >= today()-1 to avoid scanning historical partitions.
	ListByRunSince(ctx context.Context, runID string, since time.Time) ([]*domain.Span, error)
	// ListByProjectSince returns spans for a project with start_time > since, ordered by start_time ASC.
	// Bounded to _date >= today()-1 to avoid scanning historical partitions.
	ListByProjectSince(ctx context.Context, projectID string, since time.Time) ([]*domain.Span, error)
	// CountSince returns the number of spans for a project with start_time > now()-window.
	// Used for spans/min rate calculation in the health endpoint.
	CountSince(ctx context.Context, projectID string, window time.Duration) (int64, error)
}

// RunStore queries per-run aggregated metrics from ClickHouse.
type RunStore interface {
	// List returns runs for a project, newest first, paginated.
	List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Run, error)
	// Count returns the total number of runs for a project.
	Count(ctx context.Context, projectID string) (int, error)
	// Get returns a single run with its aggregated metrics.
	Get(ctx context.Context, runID string) (*domain.Run, error)
	// GetMulti fetches multiple runs by their IDs concurrently.
	GetMulti(ctx context.Context, runIDs []string) ([]*domain.Run, error)
	// ListBySession returns all runs for a session, oldest first.
	// projectID is required to enforce project-scoped access at the store layer.
	ListBySession(ctx context.Context, projectID, sessionID string) ([]*domain.Run, error)
	// GetProjectID returns the project_id for a given run_id.
	// Used by RunAuth middleware to resolve ownership without fetching full run metrics.
	// Returns an error if the run does not exist.
	GetProjectID(ctx context.Context, runID string) (string, error)
	// ListActiveRunIDs returns a set of run IDs that have had span activity within thresholdSeconds.
	// Queries spans directly (not the run_metrics view, which is a plain VIEW that re-aggregates).
	ListActiveRunIDs(ctx context.Context, projectID string, thresholdSeconds int) (map[string]bool, error)
}

// SessionStore reads session aggregates from the ClickHouse session_agg MV.
// All methods require projectID to enforce project-scoped access at the query layer.
type SessionStore interface {
	// List returns sessions for a project ordered by last_run_at DESC, paginated.
	List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Session, error)
	// Count returns the total number of sessions for a project.
	Count(ctx context.Context, projectID string) (int, error)
	// Get returns a single session aggregate.
	Get(ctx context.Context, projectID, sessionID string) (*domain.Session, error)
}

// UserStore reads per-user cost aggregates from the ClickHouse user_agg MV.
// All methods require projectID to enforce project-scoped access at the query layer.
// For authoritative cost attribution, always query user_agg directly — never join
// through run_metrics (which uses anyLast(user_id) as a display-only best-effort).
type UserStore interface {
	// List returns users for a project ordered by total_cost_usd DESC, paginated.
	List(ctx context.Context, projectID string, limit, offset int) ([]*domain.UserStats, error)
	// Count returns the total number of distinct users for a project.
	Count(ctx context.Context, projectID string) (int, error)
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
	// GetByAdminKeyHash looks up a project by the SHA-256 hex hash of its admin key.
	// Returns an error wrapping pgx.ErrNoRows if not found.
	GetByAdminKeyHash(ctx context.Context, hash string) (*domain.Project, error)
	// GetLoopConfig returns the loop-detection config for a project.
	// Returns &DefaultLoopConfig if the project has no custom config (loop_config IS NULL).
	GetLoopConfig(ctx context.Context, projectID string) (*domain.LoopConfig, error)
	// PutLoopConfig saves the loop-detection config for a project.
	PutLoopConfig(ctx context.Context, projectID string, cfg domain.LoopConfig) error
}

// BudgetStore manages budget rules and alerts in Postgres.
type BudgetStore interface {
	ListRules(ctx context.Context, projectID string) ([]*domain.BudgetRule, error)
	GetRule(ctx context.Context, id string) (*domain.BudgetRule, error)
	CreateRule(ctx context.Context, r *domain.BudgetRule) error
	UpdateRule(ctx context.Context, r *domain.BudgetRule) error
	DeleteRule(ctx context.Context, id string) error
	ListAlerts(ctx context.Context, projectID string, limit int) ([]*domain.BudgetAlert, error)
	ListRecentAlerts(ctx context.Context, projectID string, limit int) ([]*domain.RecentBudgetAlert, error)
}

// EvalStore writes and reads quality scores.
type EvalStore interface {
	// Insert writes a score to ClickHouse.
	Insert(ctx context.Context, e *domain.SpanEval) error
	// ListByRun returns all evals for all spans in a run.
	ListByRun(ctx context.Context, runID string) ([]*domain.SpanEval, error)
	// ListByRunGrouped returns evals for all spans in a run, grouped by (span_id, eval_name).
	// Each group carries per-model scores, a consensus mean, and a disagreement flag.
	ListByRunGrouped(ctx context.Context, runID string) ([]*domain.SpanEvalGroup, error)
	// SummaryByProject returns avg score per run for a project.
	SummaryByProject(ctx context.Context, projectID string) ([]*domain.RunEvalSummary, error)
	// BaselineByProject returns per-eval-type avg scores across the last N runs.
	BaselineByProject(ctx context.Context, projectID string, lastNRuns int) (*domain.EvalBaseline, error)
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
	ListRecentEvents(ctx context.Context, projectID string, limit int) ([]*domain.RecentAlertEvent, error)
	// UpdateChannelError records a delivery failure for a rule. Pass empty errMsg to clear.
	UpdateChannelError(ctx context.Context, ruleID, errMsg string) error
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

// EvalConfigStore manages per-project eval type configuration in Postgres.
type EvalConfigStore interface {
	// List returns all eval configs for a project.
	List(ctx context.Context, projectID string) ([]*domain.EvalConfig, error)
	// ListAllEnabled returns all enabled configs across all projects (used by enqueuer).
	ListAllEnabled(ctx context.Context) ([]*domain.EvalConfig, error)
	// Upsert creates or updates a config (keyed on project_id, eval_name).
	Upsert(ctx context.Context, cfg *domain.EvalConfig) error
	// Delete removes a config by eval_name within a project.
	Delete(ctx context.Context, projectID, evalName string) error
}

// ProjectPIIConfigStore manages per-project PII redaction settings in Postgres.
type ProjectPIIConfigStore interface {
	// Get returns the PII config for a project.
	// If no row exists, a default struct is returned (PIIRedactionEnabled: false, empty rules) — never an error.
	Get(ctx context.Context, projectID string) (*domain.ProjectPIIConfig, error)
	// Upsert creates or updates the PII config for a project.
	Upsert(ctx context.Context, cfg *domain.ProjectPIIConfig) error
}

// SearchStore performs full-text search over spans stored in ClickHouse.
type SearchStore interface {
	Search(ctx context.Context, params *domain.SearchParams) ([]*domain.SearchResult, error)
	SearchCount(ctx context.Context, params *domain.SearchParams) (int, error)
}

// SpanFeedbackStore manages human-in-the-loop ratings for individual spans in Postgres.
// All methods require projectID to enforce project-scoped access at the store layer.
type SpanFeedbackStore interface {
	// Upsert creates or replaces feedback for a span (keyed on project_id, span_id).
	Upsert(ctx context.Context, f *domain.SpanFeedback) error
	// GetBySpan returns the current feedback for a span, or nil if none exists.
	GetBySpan(ctx context.Context, projectID, spanID string) (*domain.SpanFeedback, error)
	// ListByRun returns all feedback records for spans in a run.
	ListByRun(ctx context.Context, projectID, runID string) ([]*domain.SpanFeedback, error)
	// Delete removes feedback for a span scoped to the project.
	Delete(ctx context.Context, projectID, spanID string) error
	// CountByProject returns the total number of feedback records for a project.
	CountByProject(ctx context.Context, projectID string) (int, error)
	// ListAllByProject returns all feedback records for a project.
	ListAllByProject(ctx context.Context, projectID string) ([]*domain.SpanFeedback, error)
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
	// ModelStats returns per-model aggregates for llm.call spans within windowSeconds.
	ModelStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.ModelStats, error)
}

// RunTagStore manages run tags in Postgres.
// All methods require projectID to enforce project-scoped access at the store layer.
type RunTagStore interface {
	// List returns all tags for a specific run.
	List(ctx context.Context, projectID, runID string) ([]string, error)
	// ListByRuns returns a map of runID → tags for multiple runs in one query.
	ListByRuns(ctx context.Context, projectID string, runIDs []string) (map[string][]string, error)
	// Add attaches a tag to a run. Silently ignores duplicates.
	Add(ctx context.Context, projectID, runID, tag string) error
	// Delete removes a tag from a run. No-op if the tag does not exist.
	Delete(ctx context.Context, projectID, runID, tag string) error
	// ListRuns returns up to limit run IDs (paginated by offset) that carry the given tag.
	// Results are capped at 500.
	ListRuns(ctx context.Context, projectID, tag string, limit, offset int) ([]string, error)
	// ListAllTags returns all distinct tags used within a project (for filter dropdowns).
	ListAllTags(ctx context.Context, projectID string) ([]string, error)
}

// RunAnnotationStore manages run annotations (free-text notes) in Postgres.
// At most one annotation exists per (project_id, run_id).
type RunAnnotationStore interface {
	// Upsert creates or replaces the annotation for a run.
	Upsert(ctx context.Context, a *domain.RunAnnotation) error
	// Get returns the annotation for a run, or nil if none exists.
	Get(ctx context.Context, projectID, runID string) (*domain.RunAnnotation, error)
	// GetByRuns returns a map of runID → annotation for multiple runs in one query.
	GetByRuns(ctx context.Context, projectID string, runIDs []string) (map[string]*domain.RunAnnotation, error)
	// Delete removes the annotation for a run. No-op if none exists.
	Delete(ctx context.Context, projectID, runID string) error
}

// PushSubscriptionStore manages browser push subscriptions in Postgres.
type PushSubscriptionStore interface {
	Upsert(ctx context.Context, s *domain.PushSubscription) error
	ListByProject(ctx context.Context, projectID string) ([]*domain.PushSubscription, error)
	Delete(ctx context.Context, projectID, endpoint string) error
}

// EmailDigestStore manages per-project email digest configuration in Postgres.
type EmailDigestStore interface {
	Get(ctx context.Context, projectID string) (*domain.EmailDigestConfig, error)
	Upsert(ctx context.Context, cfg *domain.EmailDigestConfig) error
	// ListDue returns configs where it's time to send the next digest.
	// Uses SELECT FOR UPDATE SKIP LOCKED for multi-replica safety.
	ListDue(ctx context.Context) ([]*domain.EmailDigestConfig, error)
	UpdateLastSent(ctx context.Context, projectID string) error
	UpdateLastError(ctx context.Context, projectID, errMsg string) error
}

// IngestTokenStore manages per-project ingest tokens in Postgres.
// These tokens are used by the OTel collector to authenticate span ingestion.
type IngestTokenStore interface {
	// Create inserts a new ingest token for a project.
	// tokenHash must be the SHA-256 hex of the raw token (never store the raw token).
	// Returns the created IngestToken (without the hash).
	Create(ctx context.Context, projectID, tokenHash, label string) (*domain.IngestToken, error)
	// ListByProject returns all ingest tokens for a project (no hashes).
	ListByProject(ctx context.Context, projectID string) ([]*domain.IngestToken, error)
	// Delete removes an ingest token by id, scoped to the project to prevent cross-project deletion.
	Delete(ctx context.Context, id, projectID string) error
	// GetByHash looks up an ingest token by its SHA-256 hash.
	// Used by the collector to validate an incoming ingest token.
	// Returns an error wrapping pgx.ErrNoRows if not found.
	GetByHash(ctx context.Context, tokenHash string) (*domain.IngestToken, error)
}

// ExportStore supports streaming data export from ClickHouse.
type ExportStore interface {
	// CountSpans returns the number of spans matching the export filters.
	CountSpans(ctx context.Context, params *domain.ExportParams) (int64, error)
	// ExportSpans calls fn for each span row matching the filters. Streams results.
	ExportSpans(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportSpanRow) error) error
	// CountRuns returns the number of runs matching the export filters.
	CountRuns(ctx context.Context, params *domain.ExportParams) (int64, error)
	// ExportRuns calls fn for each run row matching the filters.
	ExportRuns(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportRunRow) error) error
}

// PlaygroundStore manages prompt playground sessions, variants, and executions in Postgres.
type PlaygroundStore interface {
	CreateSession(ctx context.Context, s *domain.PlaygroundSession) error
	GetSession(ctx context.Context, id string) (*domain.PlaygroundSession, error)
	ListSessionsByProject(ctx context.Context, projectID string, limit, offset int) ([]*domain.PlaygroundSession, error)
	CountSessionsByProject(ctx context.Context, projectID string) (int, error)
	DeleteSession(ctx context.Context, id string) error
	UpsertVariant(ctx context.Context, v *domain.PlaygroundVariant) error
	ListVariantsBySession(ctx context.Context, sessionID string) ([]*domain.PlaygroundVariant, error)
	RecordExecution(ctx context.Context, e *domain.PlaygroundExecution) error
	ListExecutionsByVariant(ctx context.Context, variantID string, limit int) ([]*domain.PlaygroundExecution, error)
}

// RetentionStore manages per-project retention configuration in Postgres.
type RetentionStore interface {
	Get(ctx context.Context, projectID string) (*domain.RetentionConfig, error)
	Upsert(ctx context.Context, cfg *domain.RetentionConfig) error
	ListAll(ctx context.Context) ([]*domain.RetentionConfig, error)
}

// PurgeJobStore manages async purge job records in Postgres.
type PurgeJobStore interface {
	Create(ctx context.Context, job *domain.PurgeJob) error
	Get(ctx context.Context, id string) (*domain.PurgeJob, error)
	UpdateStatus(ctx context.Context, id, status string) error
	// Complete sets completed_at, spans_deleted, s3_keys_deleted, pg_rows_deleted,
	// partial_failure, error_msg and marks the job as 'completed' or 'failed'.
	Complete(ctx context.Context, id string, result *domain.PurgeJob) error
}

// StorageStatser computes storage usage across all stores for a project.
// Implemented as a composite (not a thin interface over one store) because
// it fans out to ClickHouse, Postgres, and S3.
type StorageStatser interface {
	GetStats(ctx context.Context, projectID string) (*domain.StorageStats, error)
}
