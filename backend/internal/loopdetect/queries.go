package loopdetect

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// QueryRepeatedToolCalls detects agent loops via two-tier ClickHouse analysis.
// Tier 1 (high confidence): same span_name + input + output, count >= cfg.Tier1MinCount
// Tier 2 (low confidence): same span_name + input only, count >= cfg.Tier2MinCount, avg interval < cfg.Tier2MaxIntervalMs
// Only spans with non-empty input attributes are considered.
// Results are deduplicated: if a (run_id, span_name, input_hash) matches both tiers,
// only the Tier 1 (high confidence) result is returned.
func QueryRepeatedToolCalls(ctx context.Context, conn driver.Conn, projectID string, since time.Time, cfg domain.LoopConfig) ([]domain.RepeatedToolCall, error) {
	// Run both tier queries
	tier1, err := queryTier1(ctx, conn, projectID, since, cfg.Tier1MinCount)
	if err != nil {
		return nil, err
	}
	tier2, err := queryTier2(ctx, conn, projectID, since, cfg.Tier2MinCount, cfg.Tier2MaxIntervalMs)
	if err != nil {
		return nil, err
	}

	// Build dedup map keyed on (run_id, span_name, input_hash)
	// Tier 1 takes priority
	type key struct{ runID, spanName, inputHash string }
	seen := make(map[key]bool)
	var results []domain.RepeatedToolCall

	for _, r := range tier1 {
		k := key{r.RunID, r.SpanName, r.InputHash}
		seen[k] = true
		results = append(results, r)
	}
	for _, r := range tier2 {
		k := key{r.RunID, r.SpanName, r.InputHash}
		if !seen[k] {
			results = append(results, r)
		}
	}
	return results, nil
}

const tier1QueryTemplate = `
SELECT
    run_id,
    span_name,
    toString(cityHash64(
        ifNull(attributes['gen_ai.prompt'], '') ||
        ifNull(attributes['tool.input'], '')
    )) AS input_hash,
    toString(cityHash64(
        ifNull(attributes['gen_ai.completion'], '') ||
        ifNull(attributes['tool.output'], '')
    )) AS output_hash,
    count() AS cnt
FROM spans
WHERE project_id = ?
  AND agent_span_kind IN ('tool.call', 'llm.call')
  AND start_time >= ?
  AND payload_s3_key = ''
  AND (attributes['gen_ai.prompt'] != '' OR attributes['tool.input'] != '')
GROUP BY run_id, span_name, input_hash, output_hash
HAVING cnt >= %d
ORDER BY cnt DESC
LIMIT 1000
`

const tier2QueryTemplate = `
SELECT
    run_id,
    span_name,
    toString(cityHash64(
        ifNull(attributes['gen_ai.prompt'], '') ||
        ifNull(attributes['tool.input'], '')
    )) AS input_hash,
    count() AS cnt,
    if(count() > 1,
        (toUnixTimestamp64Milli(max(start_time)) - toUnixTimestamp64Milli(min(start_time))) / (count() - 1),
        0
    ) AS avg_interval_ms
FROM spans
WHERE project_id = ?
  AND agent_span_kind IN ('tool.call', 'llm.call')
  AND start_time >= ?
  AND payload_s3_key = ''
  AND (attributes['gen_ai.prompt'] != '' OR attributes['tool.input'] != '')
GROUP BY run_id, span_name, input_hash
HAVING cnt >= %d AND avg_interval_ms < %d
ORDER BY cnt DESC
LIMIT 1000
`

func queryTier1(ctx context.Context, conn driver.Conn, projectID string, since time.Time, minCount int) ([]domain.RepeatedToolCall, error) {
	q := fmt.Sprintf(tier1QueryTemplate, minCount)
	rows, err := conn.Query(ctx, q, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("loopdetect tier1 query: %w", err)
	}
	defer rows.Close()

	var results []domain.RepeatedToolCall
	for rows.Next() {
		var r domain.RepeatedToolCall
		var cnt uint64
		if err := rows.Scan(&r.RunID, &r.SpanName, &r.InputHash, &r.OutputHash, &cnt); err != nil {
			return nil, fmt.Errorf("loopdetect tier1 scan: %w", err)
		}
		r.Count = int(cnt)
		r.Confidence = "high"
		results = append(results, r)
	}
	return results, rows.Err()
}

func queryTier2(ctx context.Context, conn driver.Conn, projectID string, since time.Time, minCount, maxIntervalMs int) ([]domain.RepeatedToolCall, error) {
	q := fmt.Sprintf(tier2QueryTemplate, minCount, maxIntervalMs)
	rows, err := conn.Query(ctx, q, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("loopdetect tier2 query: %w", err)
	}
	defer rows.Close()

	var results []domain.RepeatedToolCall
	for rows.Next() {
		var r domain.RepeatedToolCall
		var cnt uint64
		var avgInterval float64
		if err := rows.Scan(&r.RunID, &r.SpanName, &r.InputHash, &cnt, &avgInterval); err != nil {
			return nil, fmt.Errorf("loopdetect tier2 scan: %w", err)
		}
		r.Count = int(cnt)
		r.Confidence = "low"
		r.OutputHash = "0" // not tracked for tier 2
		results = append(results, r)
	}
	return results, rows.Err()
}
