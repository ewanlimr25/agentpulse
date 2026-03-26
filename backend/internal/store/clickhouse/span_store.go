package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

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
    ttft_ms
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
