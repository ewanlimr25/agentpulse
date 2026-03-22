package eval

import (
	"context"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const enqueueInterval = 30 * time.Second

// Enqueuer polls ClickHouse for unprocessed spans and inserts eval_jobs for them.
// Which eval types to run per project is driven by project_eval_configs.
// ON CONFLICT DO NOTHING on (span_id, eval_name) handles duplicates.
type Enqueuer struct {
	chConn      driver.Conn
	jobStore    store.EvalJobStore
	configStore store.EvalConfigStore
}

func NewEnqueuer(chConn driver.Conn, jobStore store.EvalJobStore, configStore store.EvalConfigStore) *Enqueuer {
	return &Enqueuer{chConn: chConn, jobStore: jobStore, configStore: configStore}
}

func (e *Enqueuer) Run(ctx context.Context) {
	e.enqueue(ctx)
	t := time.NewTicker(enqueueInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.enqueue(ctx)
		}
	}
}

// spanKindEvalTypes groups enabled configs by project_id → span_kind → []eval_name.
type projectEvalMap map[string]map[string][]string

func buildEvalMap(configs []*domain.EvalConfig) projectEvalMap {
	m := make(projectEvalMap)
	for _, cfg := range configs {
		if m[cfg.ProjectID] == nil {
			m[cfg.ProjectID] = make(map[string][]string)
		}
		m[cfg.ProjectID][cfg.SpanKind] = append(m[cfg.ProjectID][cfg.SpanKind], cfg.EvalName)
	}
	return m
}

// evalNamesForSpan returns the eval types to run for a given (projectID, spanKind).
// Falls back to ["relevance"] if the project has no configs (backward compatibility).
func (m projectEvalMap) evalNamesForSpan(projectID, spanKind string) []string {
	byKind, ok := m[projectID]
	if !ok || len(byKind[spanKind]) == 0 {
		if spanKind == "llm.call" {
			return []string{"relevance"} // default
		}
		return nil
	}
	return byKind[spanKind]
}

type spanRef struct {
	SpanID    string
	RunID     string
	ProjectID string
}

func (e *Enqueuer) querySpans(ctx context.Context, spanKind string) []spanRef {
	rows, err := e.chConn.Query(ctx, `
		SELECT span_id, run_id, project_id
		FROM spans
		WHERE agent_span_kind = ?
		  AND start_time >= now() - INTERVAL 24 HOUR
		  AND attributes['gen_ai.prompt'] != ''
		ORDER BY start_time DESC
		LIMIT 500
	`, spanKind)
	if err != nil {
		slog.Error("eval enqueuer: query spans", "span_kind", spanKind, "error", err)
		return nil
	}
	defer rows.Close()

	var refs []spanRef
	for rows.Next() {
		var s spanRef
		if err := rows.Scan(&s.SpanID, &s.RunID, &s.ProjectID); err != nil {
			slog.Error("eval enqueuer: scan", "error", err)
			continue
		}
		refs = append(refs, s)
	}
	return refs
}

func (e *Enqueuer) enqueue(ctx context.Context) {
	// Load all enabled configs across all projects.
	configs, err := e.configStore.ListAllEnabled(ctx)
	if err != nil {
		slog.Error("eval enqueuer: load configs", "error", err)
		configs = nil // fall back to default relevance-only behaviour
	}
	evalMap := buildEvalMap(configs)

	var jobs []*domain.EvalJob

	// llm.call spans
	for _, s := range e.querySpans(ctx, "llm.call") {
		for _, evalName := range evalMap.evalNamesForSpan(s.ProjectID, "llm.call") {
			jobs = append(jobs, &domain.EvalJob{
				EvalName:  evalName,
				SpanID:    s.SpanID,
				RunID:     s.RunID,
				ProjectID: s.ProjectID,
			})
		}
	}

	// tool.call spans (only if any project has tool.call evals enabled)
	if hasToolCallConfigs(configs) {
		for _, s := range e.querySpans(ctx, "tool.call") {
			for _, evalName := range evalMap.evalNamesForSpan(s.ProjectID, "tool.call") {
				jobs = append(jobs, &domain.EvalJob{
					EvalName:  evalName,
					SpanID:    s.SpanID,
					RunID:     s.RunID,
					ProjectID: s.ProjectID,
				})
			}
		}
	}

	if len(jobs) == 0 {
		return
	}

	if err := e.jobStore.Enqueue(ctx, jobs); err != nil {
		slog.Error("eval enqueuer: enqueue", "error", err)
		return
	}

	slog.Info("eval enqueuer: enqueued jobs", "count", len(jobs))
}

func hasToolCallConfigs(configs []*domain.EvalConfig) bool {
	for _, c := range configs {
		if c.SpanKind == "tool.call" {
			return true
		}
	}
	return false
}
