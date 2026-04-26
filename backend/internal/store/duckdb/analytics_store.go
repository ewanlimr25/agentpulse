//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
)

import "github.com/agentpulse/agentpulse/backend/internal/domain"

// AnalyticsStore implements store.AnalyticsStore against DuckDB.
// Mirrors the ClickHouse impl but adapts:
//   - quantile(0.95)(...) → quantile_cont(..., 0.95)
//   - countIf(cond)       → count(*) FILTER (WHERE cond)
//   - duration_ns         → date_diff('millisecond', start_time, end_time)
//   - now() - INTERVAL ? SECOND → now() - to_seconds(?::INTEGER)
type AnalyticsStore struct {
	db *sql.DB
}

func NewAnalyticsStore(db *sql.DB) *AnalyticsStore { return &AnalyticsStore{db: db} }

func (s *AnalyticsStore) ToolStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.ToolStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		    span_name AS tool_name,
		    count(*) AS call_count,
		    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
		    avg(date_diff('millisecond', start_time, end_time)) AS avg_latency_ms,
		    coalesce(quantile_cont(date_diff('millisecond', start_time, end_time), 0.95), 0) AS p95_latency_ms,
		    sum(cost_usd) AS total_cost_usd
		FROM spans
		WHERE project_id = ?
		  AND agent_span_kind = 'tool.call'
		  AND start_time >= now() - to_seconds(?::INTEGER)
		GROUP BY span_name
		ORDER BY call_count DESC`, projectID, windowSeconds)
	if err != nil {
		return nil, fmt.Errorf("analytics tool_stats: %w", err)
	}
	defer rows.Close()

	var results []*domain.ToolStats
	for rows.Next() {
		t := &domain.ToolStats{}
		if err := rows.Scan(
			&t.ToolName, &t.CallCount, &t.ErrorCount,
			&t.AvgLatencyMS, &t.P95LatencyMS, &t.TotalCostUSD,
		); err != nil {
			return nil, fmt.Errorf("analytics tool_stats scan: %w", err)
		}
		if t.CallCount > 0 {
			t.ErrorRate = float64(t.ErrorCount) / float64(t.CallCount) * 100
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

func (s *AnalyticsStore) AgentCostStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.AgentCostStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		    agent_name,
		    sum(cost_usd) AS total_cost_usd,
		    count(*)      AS call_count
		FROM spans
		WHERE project_id = ?
		  AND agent_name != ''
		  AND start_time >= now() - to_seconds(?::INTEGER)
		GROUP BY agent_name
		ORDER BY total_cost_usd DESC`, projectID, windowSeconds)
	if err != nil {
		return nil, fmt.Errorf("analytics agent_cost: %w", err)
	}
	defer rows.Close()

	var results []*domain.AgentCostStats
	var grandTotal float64
	for rows.Next() {
		a := &domain.AgentCostStats{}
		if err := rows.Scan(&a.AgentName, &a.TotalCostUSD, &a.CallCount); err != nil {
			return nil, fmt.Errorf("analytics agent_cost scan: %w", err)
		}
		grandTotal += a.TotalCostUSD
		results = append(results, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, a := range results {
		if grandTotal > 0 {
			a.CostPercent = a.TotalCostUSD / grandTotal * 100
		}
		if a.CallCount > 0 {
			a.AvgCostPerCall = a.TotalCostUSD / float64(a.CallCount)
		}
	}
	return results, nil
}

func (s *AnalyticsStore) ModelStats(ctx context.Context, projectID string, windowSeconds int) ([]*domain.ModelStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		    model_id,
		    count(*) AS call_count,
		    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
		    avg(date_diff('millisecond', start_time, end_time)) AS avg_latency_ms,
		    coalesce(quantile_cont(date_diff('millisecond', start_time, end_time), 0.95), 0) AS p95_latency_ms,
		    sum(cost_usd) AS total_cost_usd,
		    sum(input_tokens) AS input_tokens,
		    sum(output_tokens) AS output_tokens
		FROM spans
		WHERE project_id = ?
		  AND agent_span_kind = 'llm.call'
		  AND model_id != ''
		  AND start_time >= now() - to_seconds(?::INTEGER)
		GROUP BY model_id
		ORDER BY total_cost_usd DESC`, projectID, windowSeconds)
	if err != nil {
		return nil, fmt.Errorf("analytics model_stats: %w", err)
	}
	defer rows.Close()

	var results []*domain.ModelStats
	for rows.Next() {
		m := &domain.ModelStats{}
		if err := rows.Scan(
			&m.ModelID, &m.CallCount, &m.ErrorCount,
			&m.AvgLatencyMS, &m.P95LatencyMS, &m.TotalCostUSD,
			&m.InputTokens, &m.OutputTokens,
		); err != nil {
			return nil, fmt.Errorf("analytics model_stats scan: %w", err)
		}
		if m.CallCount > 0 {
			m.ErrorRate = float64(m.ErrorCount) / float64(m.CallCount) * 100
			m.AvgCostPerCall = m.TotalCostUSD / float64(m.CallCount)
		}
		m.TotalTokens = m.InputTokens + m.OutputTokens
		if m.TotalTokens > 0 {
			m.CostPerMillionTokens = m.TotalCostUSD / float64(m.TotalTokens) * 1_000_000
		}
		results = append(results, m)
	}
	return results, rows.Err()
}
