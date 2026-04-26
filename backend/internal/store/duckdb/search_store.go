//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// SearchStore implements store.SearchStore against DuckDB. The team-mode
// equivalent uses ClickHouse with `tokenbf_v1` skip indexes on materialized
// search columns; in indie mode we use plain LIKE on the same precomputed
// search_* columns. DuckDB's optional FTS extension is fragile to load on
// minimal Linux runtimes, so the LIKE path is the always-on default —
// correct, simple, and fast at indie scale.
//
// The search_prompt / search_completion / search_tool_input / search_tool_output
// columns are populated by the SpanStore at insert time (extracted from the
// `attributes` JSON via json_extract_string and lower()).
type SearchStore struct {
	db *sql.DB
}

func NewSearchStore(db *sql.DB) *SearchStore { return &SearchStore{db: db} }

// escapeLike escapes LIKE metacharacters so user input is matched as a literal.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func extractSnippet(text, query string) string {
	runes := []rune(text)
	lowerRunes := []rune(strings.ToLower(text))
	lowerQuery := []rune(strings.ToLower(query))

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
	out := string(runes[start:end])
	if start > 0 {
		out = "..." + out
	}
	if end < len(runes) {
		out = out + "..."
	}
	return out
}

func buildWhereClause(params *domain.SearchParams, escaped string) (string, []any) {
	args := []any{params.ProjectID, escaped, escaped, escaped, escaped}
	where := `project_id = ?
  AND payload_s3_key = ''
  AND (
       search_prompt      LIKE '%' || lower(?) || '%' ESCAPE '\'
    OR search_completion  LIKE '%' || lower(?) || '%' ESCAPE '\'
    OR search_tool_input  LIKE '%' || lower(?) || '%' ESCAPE '\'
    OR search_tool_output LIKE '%' || lower(?) || '%' ESCAPE '\'
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

func (s *SearchStore) Search(ctx context.Context, params *domain.SearchParams) ([]*domain.SearchResult, error) {
	escaped := escapeLike(params.Query)
	where, whereArgs := buildWhereClause(params, escaped)

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
    date_diff('nanosecond', start_time, end_time) AS duration_ns,
    input_tokens,
    output_tokens,
    (input_tokens + output_tokens) AS total_tokens,
    cost_usd,
    coalesce(json_extract_string(attributes, '$."gen_ai.prompt"'),     '') AS prompt_raw,
    coalesce(json_extract_string(attributes, '$."gen_ai.completion"'), '') AS completion_raw,
    coalesce(json_extract_string(attributes, '$."tool.input"'),        '') AS tool_input_raw,
    coalesce(json_extract_string(attributes, '$."tool.output"'),       '') AS tool_output_raw,
    CASE
        WHEN search_prompt      LIKE '%%' || lower(?) || '%%' ESCAPE '\' THEN 'gen_ai.prompt'
        WHEN search_completion  LIKE '%%' || lower(?) || '%%' ESCAPE '\' THEN 'gen_ai.completion'
        WHEN search_tool_input  LIKE '%%' || lower(?) || '%%' ESCAPE '\' THEN 'tool.input'
        WHEN search_tool_output LIKE '%%' || lower(?) || '%%' ESCAPE '\' THEN 'tool.output'
        ELSE ''
    END AS matched_field
FROM spans
WHERE %s
ORDER BY start_time DESC
LIMIT ? OFFSET ?
`, where)

	allArgs := make([]any, 0, len(matchedFieldArgs)+len(whereArgs)+2)
	allArgs = append(allArgs, matchedFieldArgs...)
	allArgs = append(allArgs, whereArgs...)
	allArgs = append(allArgs, params.Limit, params.Offset)

	rows, err := s.db.QueryContext(ctx, q, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("search_store query: %w", err)
	}
	defer rows.Close()

	var results []*domain.SearchResult
	for rows.Next() {
		var (
			sr        domain.SearchResult
			startTime time.Time
			promptRaw string
			complRaw  string
			toolIn    string
			toolOut   string
		)
		if err := rows.Scan(
			&sr.TraceID, &sr.SpanID, &sr.RunID, &sr.ProjectID,
			&sr.SpanName, &sr.AgentSpanKind, &sr.AgentName, &sr.ModelID,
			&sr.StatusCode, &startTime, &sr.DurationNS,
			&sr.InputTokens, &sr.OutputTokens, &sr.TotalTokens, &sr.CostUSD,
			&promptRaw, &complRaw, &toolIn, &toolOut,
			&sr.MatchedField,
		); err != nil {
			return nil, fmt.Errorf("search_store scan: %w", err)
		}
		sr.StartTime = startTime.UTC()
		switch sr.MatchedField {
		case "gen_ai.prompt":
			sr.Snippet = extractSnippet(promptRaw, params.Query)
		case "gen_ai.completion":
			sr.Snippet = extractSnippet(complRaw, params.Query)
		case "tool.input":
			sr.Snippet = extractSnippet(toolIn, params.Query)
		case "tool.output":
			sr.Snippet = extractSnippet(toolOut, params.Query)
		}
		results = append(results, &sr)
	}
	return results, rows.Err()
}

func (s *SearchStore) SearchCount(ctx context.Context, params *domain.SearchParams) (int, error) {
	escaped := escapeLike(params.Query)
	where, whereArgs := buildWhereClause(params, escaped)
	q := fmt.Sprintf(`SELECT count(*) FROM spans WHERE %s`, where)
	var n int
	if err := s.db.QueryRowContext(ctx, q, whereArgs...).Scan(&n); err != nil {
		return 0, fmt.Errorf("search_store count: %w", err)
	}
	return n, nil
}
