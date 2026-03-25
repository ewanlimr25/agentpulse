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

// evalConfigRef bundles the eval name and scope filter for a single config.
type evalConfigRef struct {
	evalName    string
	scopeFilter map[string][]string
}

// projectEvalMap groups enabled configs by project_id → span_kind → []evalConfigRef.
type projectEvalMap map[string]map[string][]evalConfigRef

func buildEvalMap(configs []*domain.EvalConfig) projectEvalMap {
	m := make(projectEvalMap)
	for _, cfg := range configs {
		if m[cfg.ProjectID] == nil {
			m[cfg.ProjectID] = make(map[string][]evalConfigRef)
		}
		m[cfg.ProjectID][cfg.SpanKind] = append(m[cfg.ProjectID][cfg.SpanKind], evalConfigRef{
			evalName:    cfg.EvalName,
			scopeFilter: cfg.ScopeFilter,
		})
	}
	return m
}

// evalNamesForSpan returns the eval types to run for a given (projectID, spanKind, agentName).
// scope_filter is applied per config: a config with {"agent_name": ["x"]} only runs for agent x.
// Falls back to ["relevance"] for llm.call spans on projects with no configs.
func (m projectEvalMap) evalNamesForSpan(projectID, spanKind, agentName string) []string {
	byKind, ok := m[projectID]
	if !ok || len(byKind[spanKind]) == 0 {
		if spanKind == "llm.call" {
			return []string{"relevance"} // default
		}
		return nil
	}
	var names []string
	for _, ref := range byKind[spanKind] {
		if matchesScopeFilter(ref.scopeFilter, agentName) {
			names = append(names, ref.evalName)
		}
	}
	return names
}

// matchesScopeFilter returns true if the span's agentName satisfies the scope filter.
// An empty/nil filter matches all spans.
func matchesScopeFilter(filter map[string][]string, agentName string) bool {
	if len(filter) == 0 {
		return true
	}
	agents, ok := filter["agent_name"]
	if !ok || len(agents) == 0 {
		return true
	}
	for _, a := range agents {
		if a == agentName {
			return true
		}
	}
	return false
}

type spanRef struct {
	SpanID    string
	RunID     string
	ProjectID string
	AgentName string
}

// querySpans fetches recent spans of the given kind that have scorable content.
// llm.call spans require a non-empty gen_ai.prompt.
// tool.call spans require tool.input or tool.output.
func (e *Enqueuer) querySpans(ctx context.Context, spanKind string) []spanRef {
	var contentFilter string
	switch spanKind {
	case "llm.call":
		contentFilter = "AND attributes['gen_ai.prompt'] != ''"
	case "tool.call":
		contentFilter = "AND (attributes['tool.input'] != '' OR attributes['tool.output'] != '')"
	}

	rows, err := e.chConn.Query(ctx, `
		SELECT span_id, run_id, project_id, agent_name
		FROM spans
		WHERE agent_span_kind = ?
		  AND start_time >= now() - INTERVAL 24 HOUR
		  `+contentFilter+`
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
		if err := rows.Scan(&s.SpanID, &s.RunID, &s.ProjectID, &s.AgentName); err != nil {
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
		for _, evalName := range evalMap.evalNamesForSpan(s.ProjectID, "llm.call", s.AgentName) {
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
			for _, evalName := range evalMap.evalNamesForSpan(s.ProjectID, "tool.call", s.AgentName) {
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
