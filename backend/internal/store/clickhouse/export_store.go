package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ExportStore implements store.ExportStore against ClickHouse.
type ExportStore struct {
	conn driver.Conn
}

func NewExportStore(conn driver.Conn) *ExportStore {
	return &ExportStore{conn: conn}
}

// buildSpanWhere constructs the WHERE clause and args for span export queries.
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

// buildRunWhere constructs the WHERE clause and args for run export queries.
func buildRunWhere(params *domain.ExportParams) (string, []any) {
	where := "project_id = ? AND min_start >= ? AND min_start < ?"
	args := []any{params.ProjectID, params.From, params.To}
	return where, args
}

func (s *ExportStore) CountSpans(ctx context.Context, params *domain.ExportParams) (int64, error) {
	where, args := buildSpanWhere(params)
	q := fmt.Sprintf("SELECT count() FROM spans WHERE %s", where)

	row := s.conn.QueryRow(ctx, q, args...)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("export count spans: %w", err)
	}
	return int64(n), nil
}

func (s *ExportStore) ExportSpans(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportSpanRow) error) error {
	where, args := buildSpanWhere(params)
	q := fmt.Sprintf(`
SELECT
    trace_id, span_id, parent_span_id, run_id, agent_span_kind, agent_name, model_id,
    span_name, service_name, status_code, status_message,
    start_time, end_time, duration_ns / 1e6 AS duration_ms,
    input_tokens, output_tokens, total_tokens, cost_usd
FROM spans
WHERE %s
ORDER BY start_time ASC
`, where)

	rows, err := s.conn.Query(ctx, q, args...)
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
	where, args := buildRunWhere(params)
	q := fmt.Sprintf("SELECT count() FROM run_metrics WHERE %s", where)

	row := s.conn.QueryRow(ctx, q, args...)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("export count runs: %w", err)
	}
	return int64(n), nil
}

func (s *ExportStore) ExportRuns(ctx context.Context, params *domain.ExportParams, fn func(*domain.ExportRunRow) error) error {
	where, args := buildRunWhere(params)
	q := fmt.Sprintf(`
SELECT
    run_id, trace_id, session_id, user_id,
    min_start, max_end,
    span_count, llm_calls, tool_calls,
    input_tokens, output_tokens, total_tokens, total_cost_usd,
    error_count
FROM run_metrics
WHERE %s
ORDER BY min_start DESC
`, where)

	rows, err := s.conn.Query(ctx, q, args...)
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
