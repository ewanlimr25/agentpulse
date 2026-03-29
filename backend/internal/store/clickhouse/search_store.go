package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// SearchStore implements store.SearchStore against ClickHouse.
// It uses pre-lowercased materialized columns (search_prompt, search_completion,
// search_tool_input, search_tool_output) backed by tokenbf_v1 skip indexes.
type SearchStore struct {
	conn driver.Conn
}

func NewSearchStore(conn driver.Conn) *SearchStore {
	return &SearchStore{conn: conn}
}

// escapeLike escapes LIKE metacharacters in the search term so user input is
// treated as a literal substring, not a pattern.
// ClickHouse's LIKE supports bracket expressions ([abc]) in addition to the
// standard SQL metacharacters (%, _, \), so we escape all five.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	return s
}

// extractSnippet returns a ~200-rune window of text centred on the first
// occurrence of query (case-insensitive). If query is not found the first
// 200 runes are returned instead.
// All arithmetic is in rune offsets to avoid splitting multibyte UTF-8 characters.
func extractSnippet(text, query string) string {
	runes := []rune(text)
	lowerRunes := []rune(strings.ToLower(text))
	lowerQuery := []rune(strings.ToLower(query))

	// Find the first rune-offset occurrence of lowerQuery in lowerRunes.
	pos := -1
	for i := 0; i <= len(lowerRunes)-len(lowerQuery); i++ {
		match := true
		for j, r := range lowerQuery {
			if lowerRunes[i+j] != r {
				match = false
				break
			}
		}
		if match {
			pos = i
			break
		}
	}

	if pos < 0 {
		if len(runes) > 200 {
			return string(runes[:200]) + "..."
		}
		return text
	}

	start := pos - 100
	if start < 0 {
		start = 0
	}
	end := pos + len(lowerQuery) + 100
	if end > len(runes) {
		end = len(runes)
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet = snippet + "..."
	}
	return snippet
}

// buildWhereClause constructs the WHERE clause and corresponding args for the
// search query. The escaped term is passed as a separate parameter for each of
// the four LIKE predicates.
func buildWhereClause(params *domain.SearchParams, escaped string) (string, []any) {
	args := []any{params.ProjectID}

	// The four field predicates — one LIKE per materialized column.
	args = append(args, escaped, escaped, escaped, escaped)

	where := `project_id = ?
  AND (
      search_prompt      LIKE '%' || lower(?) || '%'
   OR search_completion  LIKE '%' || lower(?) || '%'
   OR search_tool_input  LIKE '%' || lower(?) || '%'
   OR search_tool_output LIKE '%' || lower(?) || '%'
  )`

	if params.SpanKind != "" {
		where += "\n  AND agent_span_kind = ?"
		args = append(args, string(params.SpanKind))
	}
	if !params.From.IsZero() {
		where += "\n  AND start_time >= ?"
		args = append(args, params.From)
	}
	if !params.To.IsZero() {
		where += "\n  AND start_time <= ?"
		args = append(args, params.To)
	}

	return where, args
}

// Search returns spans matching the query, with snippet extraction done in Go.
func (s *SearchStore) Search(ctx context.Context, params *domain.SearchParams) ([]*domain.SearchResult, error) {
	escaped := escapeLike(params.Query)

	where, whereArgs := buildWhereClause(params, escaped)

	// The matched_field expression uses the same escaped term 4 more times.
	matchedFieldArgs := []any{escaped, escaped, escaped, escaped}

	q := fmt.Sprintf(`
SELECT
    trace_id,
    span_id,
    run_id,
    project_id,
    span_name,
    agent_span_kind,
    agent_name,
    model_id,
    status_code,
    start_time,
    duration_ns,
    input_tokens,
    output_tokens,
    total_tokens,
    cost_usd,
    attributes['gen_ai.prompt']     AS prompt_raw,
    attributes['gen_ai.completion'] AS completion_raw,
    attributes['tool.input']        AS tool_input_raw,
    attributes['tool.output']       AS tool_output_raw,
    multiIf(
        search_prompt      LIKE '%%' || lower(?) || '%%', 'gen_ai.prompt',
        search_completion  LIKE '%%' || lower(?) || '%%', 'gen_ai.completion',
        search_tool_input  LIKE '%%' || lower(?) || '%%', 'tool.input',
        search_tool_output LIKE '%%' || lower(?) || '%%', 'tool.output',
        ''
    ) AS matched_field
FROM spans
WHERE %s
ORDER BY start_time DESC
LIMIT ? OFFSET ?
`, where)

	// Combine: matchedField args first, then where args, then limit/offset.
	allArgs := append(matchedFieldArgs, whereArgs...)
	allArgs = append(allArgs, params.Limit, params.Offset)

	rows, err := s.conn.Query(ctx, q, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("search_store query: %w", err)
	}
	defer rows.Close()

	var results []*domain.SearchResult
	for rows.Next() {
		var (
			sr            domain.SearchResult
			startTime     time.Time
			promptRaw     string
			completionRaw string
			toolInputRaw  string
			toolOutputRaw string
		)
		if err := rows.Scan(
			&sr.TraceID,
			&sr.SpanID,
			&sr.RunID,
			&sr.ProjectID,
			&sr.SpanName,
			&sr.AgentSpanKind,
			&sr.AgentName,
			&sr.ModelID,
			&sr.StatusCode,
			&startTime,
			&sr.DurationNS,
			&sr.InputTokens,
			&sr.OutputTokens,
			&sr.TotalTokens,
			&sr.CostUSD,
			&promptRaw,
			&completionRaw,
			&toolInputRaw,
			&toolOutputRaw,
			&sr.MatchedField,
		); err != nil {
			return nil, fmt.Errorf("search_store scan: %w", err)
		}
		sr.StartTime = startTime.UTC()

		// Extract snippet from the matched field's raw value.
		switch sr.MatchedField {
		case "gen_ai.prompt":
			sr.Snippet = extractSnippet(promptRaw, params.Query)
		case "gen_ai.completion":
			sr.Snippet = extractSnippet(completionRaw, params.Query)
		case "tool.input":
			sr.Snippet = extractSnippet(toolInputRaw, params.Query)
		case "tool.output":
			sr.Snippet = extractSnippet(toolOutputRaw, params.Query)
		}

		results = append(results, &sr)
	}
	return results, rows.Err()
}

// SearchCount returns the total number of spans matching the query.
func (s *SearchStore) SearchCount(ctx context.Context, params *domain.SearchParams) (int, error) {
	escaped := escapeLike(params.Query)
	where, whereArgs := buildWhereClause(params, escaped)

	q := fmt.Sprintf(`SELECT count() FROM spans WHERE %s`, where)

	row := s.conn.QueryRow(ctx, q, whereArgs...)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("search_store count: %w", err)
	}
	return int(n), nil
}
