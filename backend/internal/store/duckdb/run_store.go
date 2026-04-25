//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// RunStore implements store.RunStore against DuckDB by aggregating spans
// directly. The team-mode equivalent reads from a ClickHouse materialized
// view; in indie mode we run the aggregation at query time. At indie scale
// (≤1M spans/day) this is well within budget for a one-binary deployment.
type RunStore struct {
	db *sql.DB
}

func NewRunStore(db *sql.DB) *RunStore { return &RunStore{db: db} }

const runAggCols = `
    run_id,
    any_value(project_id) AS project_id,
    any_value(trace_id)   AS trace_id,
    any_value(session_id) AS session_id,
    any_value(user_id)    AS user_id,
    min(start_time)       AS min_start,
    max(end_time)         AS max_end,
    count(*)              AS span_count,
    count(*) FILTER (WHERE agent_span_kind = 'llm.call')  AS llm_calls,
    count(*) FILTER (WHERE agent_span_kind = 'tool.call') AS tool_calls,
    sum(input_tokens)     AS input_tokens,
    sum(output_tokens)    AS output_tokens,
    sum(input_tokens + output_tokens) AS total_tokens,
    sum(cost_usd)         AS total_cost_usd,
    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
    coalesce(quantile_cont(ttft_ms, 0.5)  FILTER (WHERE ttft_ms > 0), 0) AS ttft_p50_ms,
    coalesce(quantile_cont(ttft_ms, 0.95) FILTER (WHERE ttft_ms > 0), 0) AS ttft_p95_ms,
    count(*) FILTER (WHERE ttft_ms > 0) AS streaming_span_count
`

func (s *RunStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+runAggCols+` FROM spans WHERE project_id = ? GROUP BY run_id ORDER BY min_start DESC LIMIT ? OFFSET ?`,
		projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("run_store list: %w", err)
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (s *RunStore) Count(ctx context.Context, projectID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(DISTINCT run_id) FROM spans WHERE project_id = ?`, projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("run_store count: %w", err)
	}
	return n, nil
}

func (s *RunStore) Get(ctx context.Context, runID string) (*domain.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+runAggCols+` FROM spans WHERE run_id = ? GROUP BY run_id LIMIT 1`, runID)
	if err != nil {
		return nil, fmt.Errorf("run_store get: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	return scanRun(rows)
}

func (s *RunStore) GetMulti(ctx context.Context, runIDs []string) ([]*domain.Run, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	out := make([]*domain.Run, 0, len(runIDs))
	for _, id := range runIDs {
		r, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *RunStore) ListBySession(ctx context.Context, projectID, sessionID string) ([]*domain.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+runAggCols+` FROM spans WHERE project_id = ? AND session_id = ? GROUP BY run_id ORDER BY min_start ASC`,
		projectID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("run_store list_by_session: %w", err)
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (s *RunStore) GetProjectID(ctx context.Context, runID string) (string, error) {
	var projectID string
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id FROM spans WHERE run_id = ? LIMIT 1`, runID).Scan(&projectID)
	if err != nil {
		return "", fmt.Errorf("run %q not found", runID)
	}
	return projectID, nil
}

func (s *RunStore) ListActiveRunIDs(ctx context.Context, projectID string, thresholdSeconds int) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT DISTINCT run_id FROM spans WHERE project_id = ? AND end_time >= now() - INTERVAL '%d second'`, thresholdSeconds),
		projectID)
	if err != nil {
		return nil, fmt.Errorf("run_store list_active: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("run_store list_active scan: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

func scanRuns(rows *sql.Rows) ([]*domain.Run, error) {
	var out []*domain.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRun(rows *sql.Rows) (*domain.Run, error) {
	r := &domain.Run{}
	var startTime, endTime time.Time
	if err := rows.Scan(
		&r.RunID, &r.ProjectID, &r.TraceID, &r.SessionID, &r.UserID,
		&startTime, &endTime,
		&r.SpanCount, &r.LLMCallCount, &r.ToolCallCount,
		&r.TotalInputTokens, &r.TotalOutputTokens, &r.TotalTokens, &r.TotalCostUSD,
		&r.ErrorCount,
		&r.TtftP50Ms, &r.TtftP95Ms, &r.StreamingSpanCount,
	); err != nil {
		return nil, fmt.Errorf("run_store scan: %w", err)
	}
	r.StartTime = startTime.UTC()
	r.EndTime = endTime.UTC()
	r.DurationMS = float64(endTime.Sub(startTime).Nanoseconds()) / 1e6
	if r.ErrorCount > 0 {
		r.Status = "error"
	} else {
		r.Status = "ok"
	}
	return r, nil
}
