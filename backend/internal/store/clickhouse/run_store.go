package clickhouse

import (
	"context"
	"fmt"
	"math"
	"sync"
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
    run_id, project_id, trace_id, session_id, user_id,
    min_start, max_end,
    span_count, llm_calls, tool_calls,
    input_tokens, output_tokens, total_tokens, total_cost_usd,
    error_count,
    ttft_p50_ms, ttft_p95_ms, streaming_span_count
FROM run_metrics
WHERE project_id = ?
ORDER BY min_start DESC
LIMIT ? OFFSET ?
`

const countRunsQuery = `
SELECT count() FROM run_metrics WHERE project_id = ?
`

const getRunQuery = `
SELECT
    run_id, project_id, trace_id, session_id, user_id,
    min_start, max_end,
    span_count, llm_calls, tool_calls,
    input_tokens, output_tokens, total_tokens, total_cost_usd,
    error_count,
    ttft_p50_ms, ttft_p95_ms, streaming_span_count
FROM run_metrics
WHERE run_id = ?
LIMIT 1
`

const getRunProjectIDQuery = `
SELECT project_id FROM run_metrics WHERE run_id = ? LIMIT 1
`

const listRunsBySessionQuery = `
SELECT
    run_id, project_id, trace_id, session_id, user_id,
    min_start, max_end,
    span_count, llm_calls, tool_calls,
    input_tokens, output_tokens, total_tokens, total_cost_usd,
    error_count,
    ttft_p50_ms, ttft_p95_ms, streaming_span_count
FROM run_metrics
WHERE project_id = ? AND session_id = ?
ORDER BY min_start ASC
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

func (s *RunStore) GetProjectID(ctx context.Context, runID string) (string, error) {
	row := s.conn.QueryRow(ctx, getRunProjectIDQuery, runID)
	var projectID string
	if err := row.Scan(&projectID); err != nil {
		return "", fmt.Errorf("run %q not found", runID)
	}
	return projectID, nil
}

func (s *RunStore) Count(ctx context.Context, projectID string) (int, error) {
	row := s.conn.QueryRow(ctx, countRunsQuery, projectID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("run_store count query: %w", err)
	}
	return int(n), nil
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

// GetMulti fetches multiple runs concurrently using parallel Get calls.
// It avoids an IN clause because clickhouse-go slice binding can be unreliable.
func (s *RunStore) GetMulti(ctx context.Context, runIDs []string) ([]*domain.Run, error) {
	type result struct {
		idx int
		run *domain.Run
		err error
	}

	ch := make(chan result, len(runIDs))
	var wg sync.WaitGroup
	for i, id := range runIDs {
		wg.Add(1)
		go func(idx int, runID string) {
			defer wg.Done()
			r, err := s.Get(ctx, runID)
			ch <- result{idx: idx, run: r, err: err}
		}(i, id)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	runs := make([]*domain.Run, len(runIDs))
	for res := range ch {
		if res.err != nil {
			return nil, res.err
		}
		runs[res.idx] = res.run
	}
	return runs, nil
}

func (s *RunStore) ListBySession(ctx context.Context, projectID, sessionID string) ([]*domain.Run, error) {
	rows, err := s.conn.Query(ctx, listRunsBySessionQuery, projectID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("run_store list_by_session query: %w", err)
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

func scanRun(rows driver.Rows) (*domain.Run, error) {
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
	// Guard against NaN from quantileIf when no streaming spans exist.
	if math.IsNaN(r.TtftP50Ms) {
		r.TtftP50Ms = 0
	}
	if math.IsNaN(r.TtftP95Ms) {
		r.TtftP95Ms = 0
	}
	return r, nil
}
