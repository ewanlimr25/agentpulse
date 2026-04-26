//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ExportStore implements store.ExportStore against DuckDB. Mirrors ClickHouse
// impl but reads from `spans` directly for run export (no run_metrics MV in
// indie mode).
type ExportStore struct {
	db *sql.DB
}

func NewExportStore(db *sql.DB) *ExportStore { return &ExportStore{db: db} }

func buildSpanWhere(params *domain.ExportParams) (string, []any) {
	where := "project_id = ? AND start_time >= ? AND start_time < ?"
	args := []any{params.ProjectID, params.From, params.To}
	if params.AgentName != "" {
		where += " AND agent_name = ?"
		args = append(args, params.AgentName)
	}
	if params.Model != "" {
		where += " AND model_id = ?"
		args = append(args, params.Model)
	}
	return where, args
}

func (s *ExportStore) CountSpans(ctx context.Context, params *domain.ExportParams) (int64, error) {
	where, args := buildSpanWhere(params)
	var n int64
	if err := s.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT count(*) FROM spans WHERE %s", where), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("export count spans: %w", err)
	}
	return n, nil
}

func (s *ExportStore) ExportSpans(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportSpanRow) error) error {
	where, args := buildSpanWhere(params)
	q := fmt.Sprintf(`
SELECT
    trace_id, span_id, parent_span_id, run_id, agent_span_kind, agent_name, model_id,
    span_name, service_name, status_code, status_message,
    start_time, end_time, date_diff('millisecond', start_time, end_time) AS duration_ms,
    input_tokens, output_tokens, (input_tokens + output_tokens) AS total_tokens, cost_usd
FROM spans
WHERE %s
ORDER BY start_time ASC`, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("export spans query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row domain.ExportSpanRow
		var startTime, endTime time.Time
		if err := rows.Scan(
			&row.TraceID, &row.SpanID, &row.ParentSpanID, &row.RunID,
			&row.AgentSpanKind, &row.AgentName, &row.ModelID,
			&row.SpanName, &row.ServiceName, &row.StatusCode, &row.StatusMessage,
			&startTime, &endTime, &row.DurationMS,
			&row.InputTokens, &row.OutputTokens, &row.TotalTokens, &row.CostUSD,
		); err != nil {
			return fmt.Errorf("export spans scan: %w", err)
		}
		row.StartTime = startTime.UTC()
		row.EndTime = endTime.UTC()
		if err := fn(&row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *ExportStore) CountRuns(ctx context.Context, params *domain.ExportParams) (int64, error) {
	where := "project_id = ? AND start_time >= ? AND start_time < ?"
	args := []any{params.ProjectID, params.From, params.To}
	q := fmt.Sprintf(
		`SELECT count(DISTINCT run_id) FROM spans WHERE %s`, where)
	var n int64
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("export count runs: %w", err)
	}
	return n, nil
}

func (s *ExportStore) ExportRuns(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportRunRow) error) error {
	where := "project_id = ? AND start_time >= ? AND start_time < ?"
	args := []any{params.ProjectID, params.From, params.To}
	q := fmt.Sprintf(`
SELECT
    run_id,
    any_value(trace_id) AS trace_id,
    any_value(session_id) AS session_id,
    any_value(user_id) AS user_id,
    min(start_time) AS min_start,
    max(end_time)   AS max_end,
    count(*) AS span_count,
    count(*) FILTER (WHERE agent_span_kind = 'llm.call') AS llm_calls,
    count(*) FILTER (WHERE agent_span_kind = 'tool.call') AS tool_calls,
    sum(input_tokens) AS input_tokens,
    sum(output_tokens) AS output_tokens,
    sum(input_tokens + output_tokens) AS total_tokens,
    sum(cost_usd) AS total_cost_usd,
    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count
FROM spans
WHERE %s
GROUP BY run_id
ORDER BY min_start DESC`, where)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("export runs query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row domain.ExportRunRow
		var startTime, endTime time.Time
		if err := rows.Scan(
			&row.RunID, &row.TraceID, &row.SessionID, &row.UserID,
			&startTime, &endTime,
			&row.SpanCount, &row.LLMCalls, &row.ToolCalls,
			&row.InputTokens, &row.OutputTokens, &row.TotalTokens, &row.TotalCostUSD,
			&row.ErrorCount,
		); err != nil {
			return fmt.Errorf("export runs scan: %w", err)
		}
		row.StartTime = startTime.UTC()
		row.EndTime = endTime.UTC()
		if err := fn(&row); err != nil {
			return err
		}
	}
	return rows.Err()
}
