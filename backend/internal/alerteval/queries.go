// Package alerteval contains the signal query functions and evaluator worker
// for multi-signal alert rule evaluation.
package alerteval

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// QueryErrorRate returns the percentage of runs (0–100) that have at least one
// error span, within the given rolling window.
// Returns -1 if there is insufficient data (fewer than minSamples runs).
func QueryErrorRate(ctx context.Context, conn driver.Conn, projectID string, windowSeconds int) (float64, error) {
	var total, errored uint64
	err := conn.QueryRow(ctx, `
		SELECT
			count()                             AS total,
			countIf(error_count > 0)            AS errored
		FROM run_metrics
		WHERE project_id = ?
		  AND min_start >= now() - INTERVAL ? SECOND
	`, projectID, windowSeconds).Scan(&total, &errored)
	if err != nil {
		return 0, fmt.Errorf("query_error_rate: %w", err)
	}
	if total < minSamples {
		return -1, nil
	}
	return float64(errored) / float64(total) * 100, nil
}

// QueryLatencyP95 returns the p95 run duration in milliseconds within the window.
// Returns -1 if there is insufficient data.
func QueryLatencyP95(ctx context.Context, conn driver.Conn, projectID string, windowSeconds int) (float64, error) {
	var p95ms float64
	var total uint64
	err := conn.QueryRow(ctx, `
		SELECT
			count()                                                                AS total,
			quantile(0.95)(
				toUnixTimestamp64Milli(max_end) - toUnixTimestamp64Milli(min_start)
			)                                                                      AS p95_ms
		FROM run_metrics
		WHERE project_id = ?
		  AND min_start >= now() - INTERVAL ? SECOND
	`, projectID, windowSeconds).Scan(&total, &p95ms)
	if err != nil {
		return 0, fmt.Errorf("query_latency_p95: %w", err)
	}
	if total < minSamples {
		return -1, nil
	}
	return p95ms, nil
}

// QueryQualityScore returns the average eval score (0.0–1.0) within the window.
// Returns -1 if there is insufficient data.
func QueryQualityScore(ctx context.Context, conn driver.Conn, projectID string, windowSeconds int) (float64, error) {
	var avg float64
	var cnt uint64
	err := conn.QueryRow(ctx, `
		SELECT count(), avg(score)
		FROM span_evals FINAL
		WHERE project_id = ?
		  AND created_at >= now() - INTERVAL ? SECOND
	`, projectID, windowSeconds).Scan(&cnt, &avg)
	if err != nil {
		return 0, fmt.Errorf("query_quality_score: %w", err)
	}
	if cnt < minSamples {
		return -1, nil
	}
	return avg, nil
}

// QueryToolFailureRate returns the percentage (0–100) of tool spans for the
// given toolName that have error status, within the window.
// Returns -1 if there is insufficient data.
func QueryToolFailureRate(ctx context.Context, conn driver.Conn, projectID, toolName string, windowSeconds int) (float64, error) {
	var total, errored uint64
	err := conn.QueryRow(ctx, `
		SELECT
			count()                              AS total,
			countIf(status_code = 'ERROR')       AS errored
		FROM spans
		WHERE project_id = ?
		  AND agent_span_kind = 'tool.call'
		  AND span_name = ?
		  AND start_time >= now() - INTERVAL ? SECOND
	`, projectID, toolName, windowSeconds).Scan(&total, &errored)
	if err != nil {
		return 0, fmt.Errorf("query_tool_failure_rate: %w", err)
	}
	if total < minSamples {
		return -1, nil
	}
	return float64(errored) / float64(total) * 100, nil
}

// minSamples is the minimum number of data points required before evaluating
// a rule. Prevents false alerts from empty or very sparse windows.
const minSamples = 5
