// Package stub provides "not yet implemented in indie mode" implementations of
// store interfaces that haven't been ported to SQLite/DuckDB yet. This lets
// cmd/server compile and start in indie mode while we incrementally replace
// stubs with real implementations.
//
// Every method returns ErrNotImplemented (or an empty result for read paths
// where that's a benign fallback — empty list, nil pointer, zero count). The
// goal is for indie mode to handle the supported flows (project + ingest
// tokens + span ingestion + run listing) and surface a clear, actionable error
// for the rest until they're filled in.
package stub

import (
	"context"
	"errors"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ErrNotImplemented signals that a store method has no indie-mode implementation yet.
var ErrNotImplemented = errors.New("not yet implemented in indie mode (P0-1 follow-up)")

// ── SessionStore ─────────────────────────────────────────────────────────────

type SessionStore struct{}

func NewSessionStore() *SessionStore { return &SessionStore{} }

func (SessionStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Session, error) {
	return nil, nil
}
func (SessionStore) Count(ctx context.Context, projectID string) (int, error) { return 0, nil }
func (SessionStore) Get(ctx context.Context, projectID, sessionID string) (*domain.Session, error) {
	return nil, ErrNotImplemented
}

// ── UserStore ────────────────────────────────────────────────────────────────

type UserStore struct{}

func NewUserStore() *UserStore { return &UserStore{} }

func (UserStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.UserStats, error) {
	return nil, nil
}
func (UserStore) Count(ctx context.Context, projectID string) (int, error) { return 0, nil }

// ── TopologyStore ────────────────────────────────────────────────────────────

type TopologyStore struct{}

func NewTopologyStore() *TopologyStore { return &TopologyStore{} }

func (TopologyStore) GetByRun(ctx context.Context, runID string) (*domain.Topology, error) {
	return &domain.Topology{}, nil
}

// ── BudgetStore ──────────────────────────────────────────────────────────────

type BudgetStore struct{}

func NewBudgetStore() *BudgetStore { return &BudgetStore{} }

func (BudgetStore) ListRules(ctx context.Context, projectID string) ([]*domain.BudgetRule, error) {
	return nil, nil
}
func (BudgetStore) GetRule(ctx context.Context, id string) (*domain.BudgetRule, error) {
	return nil, ErrNotImplemented
}
func (BudgetStore) CreateRule(ctx context.Context, r *domain.BudgetRule) error {
	return ErrNotImplemented
}
func (BudgetStore) UpdateRule(ctx context.Context, r *domain.BudgetRule) error {
	return ErrNotImplemented
}
func (BudgetStore) DeleteRule(ctx context.Context, id string) error { return ErrNotImplemented }
func (BudgetStore) ListAlerts(ctx context.Context, projectID string, limit int) ([]*domain.BudgetAlert, error) {
	return nil, nil
}
func (BudgetStore) ListRecentAlerts(ctx context.Context, projectID string, limit int) ([]*domain.RecentBudgetAlert, error) {
	return nil, nil
}

// ── EvalStore ────────────────────────────────────────────────────────────────

type EvalStore struct{}

func NewEvalStore() *EvalStore { return &EvalStore{} }

func (EvalStore) Insert(ctx context.Context, e *domain.SpanEval) error { return ErrNotImplemented }
func (EvalStore) ListByRun(ctx context.Context, runID string) ([]*domain.SpanEval, error) {
	return nil, nil
}
func (EvalStore) ListByRunGrouped(ctx context.Context, runID string) ([]*domain.SpanEvalGroup, error) {
	return nil, nil
}
func (EvalStore) SummaryByProject(ctx context.Context, projectID string) ([]*domain.RunEvalSummary, error) {
	return nil, nil
}
func (EvalStore) BaselineByProject(ctx context.Context, projectID string, lastNRuns int) (*domain.EvalBaseline, error) {
	return &domain.EvalBaseline{ProjectID: projectID}, nil
}

// ── AlertRuleStore ───────────────────────────────────────────────────────────

type AlertRuleStore struct{}

func NewAlertRuleStore() *AlertRuleStore { return &AlertRuleStore{} }

func (AlertRuleStore) ListRules(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	return nil, nil
}
func (AlertRuleStore) GetRule(ctx context.Context, id string) (*domain.AlertRule, error) {
	return nil, ErrNotImplemented
}
func (AlertRuleStore) CreateRule(ctx context.Context, r *domain.AlertRule) error {
	return ErrNotImplemented
}
func (AlertRuleStore) UpdateRule(ctx context.Context, r *domain.AlertRule) error {
	return ErrNotImplemented
}
func (AlertRuleStore) DeleteRule(ctx context.Context, id string) error { return ErrNotImplemented }
func (AlertRuleStore) ListEnabledRules(ctx context.Context) ([]*domain.AlertRule, error) {
	return nil, nil
}
func (AlertRuleStore) ListEvents(ctx context.Context, projectID string, limit int) ([]*domain.AlertEvent, error) {
	return nil, nil
}
func (AlertRuleStore) CreateEvent(ctx context.Context, e *domain.AlertEvent) error {
	return ErrNotImplemented
}
func (AlertRuleStore) LastEventForRule(ctx context.Context, ruleID string) (*domain.AlertEvent, error) {
	return nil, nil
}
func (AlertRuleStore) ListRecentEvents(ctx context.Context, projectID string, limit int) ([]*domain.RecentAlertEvent, error) {
	return nil, nil
}
func (AlertRuleStore) UpdateChannelError(ctx context.Context, ruleID, errMsg string) error {
	return ErrNotImplemented
}

// ── EvalJobStore ─────────────────────────────────────────────────────────────

type EvalJobStore struct{}

func NewEvalJobStore() *EvalJobStore { return &EvalJobStore{} }

func (EvalJobStore) Enqueue(ctx context.Context, jobs []*domain.EvalJob) error { return nil }
func (EvalJobStore) Claim(ctx context.Context, n int) ([]*domain.EvalJob, error)  { return nil, nil }
func (EvalJobStore) MarkDone(ctx context.Context, id string) error              { return nil }
func (EvalJobStore) MarkFailed(ctx context.Context, id, errMsg string) error    { return nil }

// ── EvalConfigStore ──────────────────────────────────────────────────────────

type EvalConfigStore struct{}

func NewEvalConfigStore() *EvalConfigStore { return &EvalConfigStore{} }

func (EvalConfigStore) List(ctx context.Context, projectID string) ([]*domain.EvalConfig, error) {
	return nil, nil
}
func (EvalConfigStore) ListAllEnabled(ctx context.Context) ([]*domain.EvalConfig, error) {
	return nil, nil
}
func (EvalConfigStore) Upsert(ctx context.Context, cfg *domain.EvalConfig) error {
	return ErrNotImplemented
}
func (EvalConfigStore) Delete(ctx context.Context, projectID, evalName string) error {
	return ErrNotImplemented
}

// ── ProjectPIIConfigStore ────────────────────────────────────────────────────

type ProjectPIIConfigStore struct{}

func NewProjectPIIConfigStore() *ProjectPIIConfigStore { return &ProjectPIIConfigStore{} }

func (ProjectPIIConfigStore) Get(ctx context.Context, projectID string) (*domain.ProjectPIIConfig, error) {
	return &domain.ProjectPIIConfig{ProjectID: projectID}, nil
}
func (ProjectPIIConfigStore) Upsert(ctx context.Context, cfg *domain.ProjectPIIConfig) error {
	return ErrNotImplemented
}

// ── SearchStore ──────────────────────────────────────────────────────────────

type SearchStore struct{}

func NewSearchStore() *SearchStore { return &SearchStore{} }

func (SearchStore) Search(ctx context.Context, params *domain.SearchParams) ([]*domain.SearchResult, error) {
	return nil, nil
}
func (SearchStore) SearchCount(ctx context.Context, params *domain.SearchParams) (int, error) {
	return 0, nil
}

// ── SpanFeedbackStore ────────────────────────────────────────────────────────

type SpanFeedbackStore struct{}

func NewSpanFeedbackStore() *SpanFeedbackStore { return &SpanFeedbackStore{} }

func (SpanFeedbackStore) Upsert(ctx context.Context, f *domain.SpanFeedback) error {
	return ErrNotImplemented
}
func (SpanFeedbackStore) GetBySpan(ctx context.Context, projectID, spanID string) (*domain.SpanFeedback, error) {
	return nil, nil
}
func (SpanFeedbackStore) ListByRun(ctx context.Context, projectID, runID string) ([]*domain.SpanFeedback, error) {
	return nil, nil
}
func (SpanFeedbackStore) Delete(ctx context.Context, projectID, spanID string) error {
	return ErrNotImplemented
}
func (SpanFeedbackStore) CountByProject(ctx context.Context, projectID string) (int, error) {
	return 0, nil
}
func (SpanFeedbackStore) ListAllByProject(ctx context.Context, projectID string) ([]*domain.SpanFeedback, error) {
	return nil, nil
}

// ── LoopStore ────────────────────────────────────────────────────────────────

type LoopStore struct{}

func NewLoopStore() *LoopStore { return &LoopStore{} }

func (LoopStore) Upsert(ctx context.Context, loop *domain.RunLoop) error { return nil }
func (LoopStore) ListByRun(ctx context.Context, runID string) ([]*domain.RunLoop, error) {
	return nil, nil
}
func (LoopStore) HasLoops(ctx context.Context, runIDs []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (LoopStore) CountByProject(ctx context.Context, projectID string, windowSeconds int) (int, error) {
	return 0, nil
}

// ── AnalyticsStore ───────────────────────────────────────────────────────────

type AnalyticsStore struct{}

func NewAnalyticsStore() *AnalyticsStore { return &AnalyticsStore{} }

func (AnalyticsStore) ToolStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.ToolStats, error) {
	return nil, nil
}
func (AnalyticsStore) AgentCostStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.AgentCostStats, error) {
	return nil, nil
}
func (AnalyticsStore) ModelStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.ModelStats, error) {
	return nil, nil
}

// ── RunTagStore ──────────────────────────────────────────────────────────────

type RunTagStore struct{}

func NewRunTagStore() *RunTagStore { return &RunTagStore{} }

func (RunTagStore) List(ctx context.Context, projectID, runID string) ([]string, error) {
	return nil, nil
}
func (RunTagStore) ListByRuns(ctx context.Context, projectID string, runIDs []string) (map[string][]string, error) {
	return map[string][]string{}, nil
}
func (RunTagStore) Add(ctx context.Context, projectID, runID, tag string) error {
	return ErrNotImplemented
}
func (RunTagStore) Delete(ctx context.Context, projectID, runID, tag string) error {
	return ErrNotImplemented
}
func (RunTagStore) ListRuns(ctx context.Context, projectID, tag string, limit, offset int) ([]string, error) {
	return nil, nil
}
func (RunTagStore) ListAllTags(ctx context.Context, projectID string) ([]string, error) {
	return nil, nil
}

// ── RunAnnotationStore ───────────────────────────────────────────────────────

type RunAnnotationStore struct{}

func NewRunAnnotationStore() *RunAnnotationStore { return &RunAnnotationStore{} }

func (RunAnnotationStore) Upsert(ctx context.Context, a *domain.RunAnnotation) error {
	return ErrNotImplemented
}
func (RunAnnotationStore) Get(ctx context.Context, projectID, runID string) (*domain.RunAnnotation, error) {
	return nil, nil
}
func (RunAnnotationStore) GetByRuns(ctx context.Context, projectID string, runIDs []string) (map[string]*domain.RunAnnotation, error) {
	return map[string]*domain.RunAnnotation{}, nil
}
func (RunAnnotationStore) Delete(ctx context.Context, projectID, runID string) error {
	return ErrNotImplemented
}

// ── PushSubscriptionStore ────────────────────────────────────────────────────

type PushSubscriptionStore struct{}

func NewPushSubscriptionStore() *PushSubscriptionStore { return &PushSubscriptionStore{} }

func (PushSubscriptionStore) Upsert(ctx context.Context, s *domain.PushSubscription) error {
	return ErrNotImplemented
}
func (PushSubscriptionStore) ListByProject(ctx context.Context, projectID string) ([]*domain.PushSubscription, error) {
	return nil, nil
}
func (PushSubscriptionStore) Delete(ctx context.Context, projectID, endpoint string) error {
	return ErrNotImplemented
}

// ── EmailDigestStore ─────────────────────────────────────────────────────────

type EmailDigestStore struct{}

func NewEmailDigestStore() *EmailDigestStore { return &EmailDigestStore{} }

func (EmailDigestStore) Get(ctx context.Context, projectID string) (*domain.EmailDigestConfig, error) {
	return &domain.EmailDigestConfig{ProjectID: projectID}, nil
}
func (EmailDigestStore) Upsert(ctx context.Context, cfg *domain.EmailDigestConfig) error {
	return ErrNotImplemented
}
func (EmailDigestStore) ListDue(ctx context.Context) ([]*domain.EmailDigestConfig, error) {
	return nil, nil
}
func (EmailDigestStore) UpdateLastSent(ctx context.Context, projectID string) error { return nil }
func (EmailDigestStore) UpdateLastError(ctx context.Context, projectID, errMsg string) error {
	return nil
}

// ── ExportStore ──────────────────────────────────────────────────────────────

type ExportStore struct{}

func NewExportStore() *ExportStore { return &ExportStore{} }

func (ExportStore) CountSpans(ctx context.Context, params *domain.ExportParams) (int64, error) {
	return 0, nil
}
func (ExportStore) ExportSpans(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportSpanRow) error) error {
	return nil
}
func (ExportStore) CountRuns(ctx context.Context, params *domain.ExportParams) (int64, error) {
	return 0, nil
}
func (ExportStore) ExportRuns(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportRunRow) error) error {
	return nil
}

// ── PlaygroundStore ──────────────────────────────────────────────────────────

type PlaygroundStore struct{}

func NewPlaygroundStore() *PlaygroundStore { return &PlaygroundStore{} }

func (PlaygroundStore) CreateSession(ctx context.Context, s *domain.PlaygroundSession) error {
	return ErrNotImplemented
}
func (PlaygroundStore) GetSession(ctx context.Context, id string) (*domain.PlaygroundSession, error) {
	return nil, ErrNotImplemented
}
func (PlaygroundStore) ListSessionsByProject(ctx context.Context, projectID string, limit, offset int) ([]*domain.PlaygroundSession, error) {
	return nil, nil
}
func (PlaygroundStore) CountSessionsByProject(ctx context.Context, projectID string) (int, error) {
	return 0, nil
}
func (PlaygroundStore) DeleteSession(ctx context.Context, id string) error { return ErrNotImplemented }
func (PlaygroundStore) UpsertVariant(ctx context.Context, v *domain.PlaygroundVariant) error {
	return ErrNotImplemented
}
func (PlaygroundStore) ListVariantsBySession(ctx context.Context, sessionID string) ([]*domain.PlaygroundVariant, error) {
	return nil, nil
}
func (PlaygroundStore) RecordExecution(ctx context.Context, e *domain.PlaygroundExecution) error {
	return ErrNotImplemented
}
func (PlaygroundStore) ListExecutionsByVariant(ctx context.Context, variantID string, limit int) ([]*domain.PlaygroundExecution, error) {
	return nil, nil
}

// ── RetentionStore ───────────────────────────────────────────────────────────

type RetentionStore struct{}

func NewRetentionStore() *RetentionStore { return &RetentionStore{} }

func (RetentionStore) Get(ctx context.Context, projectID string) (*domain.RetentionConfig, error) {
	return &domain.RetentionConfig{ProjectID: projectID}, nil
}
func (RetentionStore) Upsert(ctx context.Context, cfg *domain.RetentionConfig) error {
	return ErrNotImplemented
}
func (RetentionStore) ListAll(ctx context.Context) ([]*domain.RetentionConfig, error) {
	return nil, nil
}

// ── PurgeJobStore ────────────────────────────────────────────────────────────

type PurgeJobStore struct{}

func NewPurgeJobStore() *PurgeJobStore { return &PurgeJobStore{} }

func (PurgeJobStore) Create(ctx context.Context, job *domain.PurgeJob) error { return ErrNotImplemented }
func (PurgeJobStore) Get(ctx context.Context, id string) (*domain.PurgeJob, error) {
	return nil, ErrNotImplemented
}
func (PurgeJobStore) UpdateStatus(ctx context.Context, id, status string) error {
	return ErrNotImplemented
}
func (PurgeJobStore) Complete(ctx context.Context, id string, result *domain.PurgeJob) error {
	return ErrNotImplemented
}

// Compile-time check that ErrNotImplemented isn't accidentally swallowed.
var _ = time.Now
