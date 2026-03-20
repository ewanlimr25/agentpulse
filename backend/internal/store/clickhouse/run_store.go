package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// RunStore implements store.RunStore against ClickHouse.
// It queries the run_metrics materialized view created in 002_metrics_agg.sql.
type RunStore struct {
	conn driver.Conn
}

func NewRunStore(conn driver.Conn) *RunStore {
	return &RunStore{conn: conn}
}

// listRunsQuery reads from the run_metrics_mv materialized view.
// The view has columns: run_id, project_id, trace_id, min_start, max_end,
// span_count, llm_calls, tool_calls, input_tokens, output_tokens,
// total_tokens, total_cost_usd, error_count.
const listRunsQuery = `
SELECT
    run_id, project_id, trace_id,
    min_start, max_end,
    span_count, llm_calls, tool_calls,
    input_tokens, output_tokens, total_tokens, total_cost_usd,
    error_count
FROM run_metrics
WHERE project_id = ?
ORDER BY min_start DESC
LIMIT ? OFFSET ?
`

const getRunQuery = `
SELECT
    run_id, project_id, trace_id,
    min_start, max_end,
    span_count, llm_calls, tool_calls,
    input_tokens, output_tokens, total_tokens, total_cost_usd,
    error_count
FROM run_metrics
WHERE run_id = ?
LIMIT 1
`

func (s *RunStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Run, error) {
	rows, err := s.conn.Query(ctx, listRunsQuery, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("run_store list query: %w", err)
	}
	defer rows.Close()

	var runs []*domain.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *RunStore) Get(ctx context.Context, runID string) (*domain.Run, error) {
	rows, err := s.conn.Query(ctx, getRunQuery, runID)
	if err != nil {
		return nil, fmt.Errorf("run_store get query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	r, err := scanRun(rows)
	if err != nil {
		return nil, err
	}
	return r, rows.Err()
}

func scanRun(rows driver.Rows) (*domain.Run, error) {
	r := &domain.Run{}
	var startTime, endTime time.Time
	if err := rows.Scan(
		&r.RunID, &r.ProjectID, &r.TraceID,
		&startTime, &endTime,
		&r.SpanCount, &r.LLMCallCount, &r.ToolCallCount,
		&r.TotalInputTokens, &r.TotalOutputTokens, &r.TotalTokens, &r.TotalCostUSD,
		&r.ErrorCount,
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
