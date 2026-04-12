package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ErrSpanNotFound is returned by GetByID when no span matches the given project+span IDs.
var ErrSpanNotFound = errors.New("span not found")

// SpanStore implements store.SpanStore against ClickHouse.
type SpanStore struct {
	conn driver.Conn
}

func NewSpanStore(conn driver.Conn) *SpanStore {
	return &SpanStore{conn: conn}
}

const listByRunQuery = `
SELECT
    trace_id, span_id, parent_span_id,
    run_id, project_id,
    agent_span_kind, agent_name, model_id,
    span_name, service_name, status_code, status_message,
    start_time, end_time, duration_ns,
    input_tokens, output_tokens, total_tokens, cost_usd,
    attributes, resource_attrs,
    ttft_ms,
    payload_s3_key
FROM spans
WHERE run_id = ?
ORDER BY start_time ASC
`

func (s *SpanStore) ListByRun(ctx context.Context, runID string) ([]*domain.Span, error) {
	rows, err := s.conn.Query(ctx, listByRunQuery, runID)
	if err != nil {
		return nil, fmt.Errorf("span_store list_by_run query: %w", err)
	}
	defer rows.Close()

	var spans []*domain.Span
	for rows.Next() {
		sp := &domain.Span{}
		var startTime, endTime time.Time
		var agentSpanKind, statusCode string
		var attrs, resourceAttrs map[string]string

		if err := rows.Scan(
			&sp.TraceID, &sp.SpanID, &sp.ParentSpanID,
			&sp.RunID, &sp.ProjectID,
			&agentSpanKind, &sp.AgentName, &sp.ModelID,
			&sp.SpanName, &sp.ServiceName, &statusCode, &sp.StatusMessage,
			&startTime, &endTime, &sp.DurationNS,
			&sp.InputTokens, &sp.OutputTokens, &sp.TotalTokens, &sp.CostUSD,
			&attrs, &resourceAttrs,
			&sp.TtftMs,
			&sp.PayloadS3Key,
		); err != nil {
			return nil, fmt.Errorf("span_store scan: %w", err)
		}

		sp.AgentSpanKind = domain.AgentSpanKind(agentSpanKind)
		sp.StatusCode = domain.StatusCode(statusCode)
		sp.StartTime = startTime.UTC()
		sp.EndTime = endTime.UTC()
		sp.Attributes = attrs
		sp.ResourceAttrs = resourceAttrs

		spans = append(spans, sp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("span_store rows: %w", err)
	}

	return spans, nil
}

const listByRunSinceQuery = `
SELECT
    trace_id, span_id, parent_span_id,
    run_id, project_id,
    agent_span_kind, agent_name, model_id,
    span_name, service_name, status_code, status_message,
    start_time, end_time, duration_ns,
    input_tokens, output_tokens, total_tokens, cost_usd,
    attributes, resource_attrs,
    ttft_ms,
    payload_s3_key
FROM spans
WHERE run_id = ?
  AND _date >= today() - 1
  AND start_time > ?
ORDER BY start_time ASC
`

func (s *SpanStore) ListByRunSince(ctx context.Context, runID string, since time.Time) ([]*domain.Span, error) {
	rows, err := s.conn.Query(ctx, listByRunSinceQuery, runID, since)
	if err != nil {
		return nil, fmt.Errorf("span_store list_by_run_since query: %w", err)
	}
	defer rows.Close()

	var spans []*domain.Span
	for rows.Next() {
		sp := &domain.Span{}
		var startTime, endTime time.Time
		var agentSpanKind, statusCode string
		var attrs, resourceAttrs map[string]string

		if err := rows.Scan(
			&sp.TraceID, &sp.SpanID, &sp.ParentSpanID,
			&sp.RunID, &sp.ProjectID,
			&agentSpanKind, &sp.AgentName, &sp.ModelID,
			&sp.SpanName, &sp.ServiceName, &statusCode, &sp.StatusMessage,
			&startTime, &endTime, &sp.DurationNS,
			&sp.InputTokens, &sp.OutputTokens, &sp.TotalTokens, &sp.CostUSD,
			&attrs, &resourceAttrs,
			&sp.TtftMs,
			&sp.PayloadS3Key,
		); err != nil {
			return nil, fmt.Errorf("span_store list_by_run_since scan: %w", err)
		}

		sp.AgentSpanKind = domain.AgentSpanKind(agentSpanKind)
		sp.StatusCode = domain.StatusCode(statusCode)
		sp.StartTime = startTime.UTC()
		sp.EndTime = endTime.UTC()
		sp.Attributes = attrs
		sp.ResourceAttrs = resourceAttrs

		spans = append(spans, sp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("span_store list_by_run_since rows: %w", err)
	}

	return spans, nil
}

const getByIDQuery = `
SELECT
    trace_id, span_id, parent_span_id,
    run_id, project_id,
    agent_span_kind, agent_name, model_id,
    span_name, service_name, status_code, status_message,
    start_time, end_time, duration_ns,
    input_tokens, output_tokens, total_tokens, cost_usd,
    attributes, resource_attrs,
    ttft_ms,
    payload_s3_key
FROM spans
WHERE project_id = ? AND span_id = ?
LIMIT 1
`

const latestSpanTimeQuery = `
SELECT max(start_time), count()
FROM spans
WHERE project_id = ?
  AND start_time > now() - INTERVAL 24 HOUR
`

// LatestSpanTime returns the timestamp of the most recent span for a project within the
// last 24 hours, or nil if no spans exist. Returns nil, nil when count is 0.
func (s *SpanStore) LatestSpanTime(ctx context.Context, projectID string) (*time.Time, error) {
	row := s.conn.QueryRow(ctx, latestSpanTimeQuery, projectID)

	var maxTime time.Time
	var count uint64
	if err := row.Scan(&maxTime, &count); err != nil {
		return nil, fmt.Errorf("span_store latest_span_time: %w", err)
	}
	if count == 0 {
		return nil, nil
	}
	t := maxTime.UTC()
	return &t, nil
}

// GetByID returns a single span filtered by both project_id and span_id.
// Returns ErrSpanNotFound if no span matches.
func (s *SpanStore) GetByID(ctx context.Context, projectID, spanID string) (*domain.Span, error) {
	rows, err := s.conn.Query(ctx, getByIDQuery, projectID, spanID)
	if err != nil {
		return nil, fmt.Errorf("span_store get_by_id query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("span_store get_by_id rows: %w", err)
		}
		return nil, ErrSpanNotFound
	}

	sp := &domain.Span{}
	var startTime, endTime time.Time
	var agentSpanKind, statusCode string
	var attrs, resourceAttrs map[string]string

	if err := rows.Scan(
		&sp.TraceID, &sp.SpanID, &sp.ParentSpanID,
		&sp.RunID, &sp.ProjectID,
		&agentSpanKind, &sp.AgentName, &sp.ModelID,
		&sp.SpanName, &sp.ServiceName, &statusCode, &sp.StatusMessage,
		&startTime, &endTime, &sp.DurationNS,
		&sp.InputTokens, &sp.OutputTokens, &sp.TotalTokens, &sp.CostUSD,
		&attrs, &resourceAttrs,
		&sp.TtftMs,
		&sp.PayloadS3Key,
	); err != nil {
		return nil, fmt.Errorf("span_store get_by_id scan: %w", err)
	}

	sp.AgentSpanKind = domain.AgentSpanKind(agentSpanKind)
	sp.StatusCode = domain.StatusCode(statusCode)
	sp.StartTime = startTime.UTC()
	sp.EndTime = endTime.UTC()
	sp.Attributes = attrs
	sp.ResourceAttrs = resourceAttrs

	return sp, nil
}
