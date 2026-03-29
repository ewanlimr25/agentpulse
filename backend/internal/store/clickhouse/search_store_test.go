package clickhouse

// Unit tests for the pure-Go helpers in search_store.go:
//   - escapeLike
//   - extractSnippet
//   - buildWhereClause
//
// No ClickHouse connection is required; these are all in-process tests.

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// escapeLike
// ---------------------------------------------------------------------------

func TestEscapeLike_Backslash(t *testing.T) {
	got := escapeLike(`a\b`)
	want := `a\\b`
	if got != want {
		t.Errorf("escapeLike backslash: got %q, want %q", got, want)
	}
}

func TestEscapeLike_Percent(t *testing.T) {
	got := escapeLike(`50%`)
	want := `50\%`
	if got != want {
		t.Errorf("escapeLike percent: got %q, want %q", got, want)
	}
}

func TestEscapeLike_Underscore(t *testing.T) {
	got := escapeLike(`foo_bar`)
	want := `foo\_bar`
	if got != want {
		t.Errorf("escapeLike underscore: got %q, want %q", got, want)
	}
}

func TestEscapeLike_LeftBracket(t *testing.T) {
	got := escapeLike(`foo[bar`)
	want := `foo\[bar`
	if got != want {
		t.Errorf("escapeLike left bracket: got %q, want %q", got, want)
	}
}

func TestEscapeLike_RightBracket(t *testing.T) {
	got := escapeLike(`foo]bar`)
	want := `foo\]bar`
	if got != want {
		t.Errorf("escapeLike right bracket: got %q, want %q", got, want)
	}
}

func TestEscapeLike_AdversarialCombined(t *testing.T) {
	// "100% [complete]" — contains percent, space, brackets
	got := escapeLike(`100% [complete]`)
	want := `100\% \[complete\]`
	if got != want {
		t.Errorf("escapeLike adversarial: got %q, want %q", got, want)
	}
}

func TestEscapeLike_PlainAlphanumeric(t *testing.T) {
	input := `HelloWorld123`
	got := escapeLike(input)
	if got != input {
		t.Errorf("escapeLike plain alphanumeric: expected no change, got %q", got)
	}
}

func TestEscapeLike_AllMetacharsTogether(t *testing.T) {
	// Input has all five metacharacters in one string.
	input := `\%_[]`
	got := escapeLike(input)
	want := `\\\%\_\[\]`
	if got != want {
		t.Errorf("escapeLike all metas: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// extractSnippet
// ---------------------------------------------------------------------------

func TestExtractSnippet_MatchInMiddle(t *testing.T) {
	// Build a string long enough that a match in the middle triggers ellipsis
	// on both sides: 120 chars before the keyword, keyword, 120 chars after.
	prefix := strings.Repeat("a", 120)
	suffix := strings.Repeat("z", 120)
	text := prefix + "KEYWORD" + suffix
	query := "KEYWORD"

	got := extractSnippet(text, query)

	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected '...' prefix for mid-text match, got: %q", got[:min(20, len(got))])
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected '...' suffix for mid-text match, got: %q", got[max(0, len(got)-20):])
	}
	if !strings.Contains(got, "KEYWORD") {
		t.Errorf("snippet does not contain the matched keyword")
	}
}

func TestExtractSnippet_MatchAtStart(t *testing.T) {
	// Match is at position 0 — no leading ellipsis.
	text := "rate limit exceeded. " + strings.Repeat("x", 300)
	got := extractSnippet(text, "rate")

	if strings.HasPrefix(got, "...") {
		t.Errorf("expected no leading '...' when match is at start, got: %q", got[:min(20, len(got))])
	}
	if !strings.Contains(got, "rate") {
		t.Errorf("snippet should contain 'rate'")
	}
}

func TestExtractSnippet_MatchAtEnd(t *testing.T) {
	// Match is within the last 100 runes — no trailing ellipsis.
	text := strings.Repeat("a", 200) + "ENDPOINT"
	got := extractSnippet(text, "ENDPOINT")

	if strings.HasSuffix(got, "...") {
		t.Errorf("expected no trailing '...' when match reaches end of text, got suffix: %q", got[max(0, len(got)-20):])
	}
	if !strings.Contains(got, "ENDPOINT") {
		t.Errorf("snippet should contain 'ENDPOINT'")
	}
}

func TestExtractSnippet_NoMatch_LongText(t *testing.T) {
	// When query is absent, the first 200 runes are returned with "..." appended.
	text := strings.Repeat("b", 300)
	got := extractSnippet(text, "NOTFOUND")

	runes := []rune(got)
	// Strip trailing "..." before counting runes.
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected '...' suffix when no match and text > 200 runes")
	}
	// Content before "..." must be exactly 200 runes.
	content := strings.TrimSuffix(got, "...")
	if len([]rune(content)) != 200 {
		t.Errorf("expected 200-rune content before '...', got %d runes; snippet len=%d", len([]rune(content)), len(runes))
	}
}

func TestExtractSnippet_NoMatch_ShortText(t *testing.T) {
	// When query is absent and text <= 200 runes, return verbatim — no ellipsis.
	text := "short text"
	got := extractSnippet(text, "NOTFOUND")

	if got != text {
		t.Errorf("expected verbatim text for short no-match, got %q", got)
	}
	if strings.Contains(got, "...") {
		t.Errorf("expected no ellipsis for short no-match text, got %q", got)
	}
}

func TestExtractSnippet_TextShorterThanWindow(t *testing.T) {
	// Text is shorter than the snippet window (200 chars). The full text is
	// returned without any ellipsis even when the match is present.
	text := "the quick brown fox"
	got := extractSnippet(text, "quick")

	if strings.Contains(got, "...") {
		t.Errorf("expected no ellipsis when full text fits in window, got %q", got)
	}
	if !strings.Contains(got, "quick") {
		t.Errorf("snippet should contain 'quick'")
	}
}

func TestExtractSnippet_MultibyteChinese(t *testing.T) {
	// 200 Chinese characters (each is 3 bytes in UTF-8).
	// Match sits in the middle. The function must not panic and must return
	// valid UTF-8.
	prefix := strings.Repeat("中", 120)
	suffix := strings.Repeat("文", 120)
	text := prefix + "关键词" + suffix
	query := "关键词"

	got := extractSnippet(text, query)

	if !utf8.ValidString(got) {
		t.Error("extractSnippet returned invalid UTF-8 for Chinese input")
	}
	if !strings.Contains(got, "关键词") {
		t.Errorf("snippet should contain the Chinese keyword")
	}
}

func TestExtractSnippet_MultibytEmoji(t *testing.T) {
	// Emoji are 4-byte UTF-8. The function must handle rune boundaries correctly.
	prefix := strings.Repeat("😀", 50)
	suffix := strings.Repeat("🎉", 50)
	text := prefix + "🔥" + suffix
	query := "🔥"

	got := extractSnippet(text, query)

	if !utf8.ValidString(got) {
		t.Error("extractSnippet returned invalid UTF-8 for emoji input")
	}
	if !strings.Contains(got, "🔥") {
		t.Errorf("snippet should contain the fire emoji")
	}
}

func TestExtractSnippet_CaseInsensitiveMatch(t *testing.T) {
	// Query "RATE" must match "rate" in the text.
	text := "the current rate limit has been exceeded by the client"
	got := extractSnippet(text, "RATE")

	if !strings.Contains(got, "rate") {
		t.Errorf("case-insensitive: snippet should contain 'rate', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// buildWhereClause
// ---------------------------------------------------------------------------

func TestBuildWhereClause_NoOptionalFilters(t *testing.T) {
	params := &domain.SearchParams{
		ProjectID: "proj-abc",
	}
	escaped := "search_term"
	where, args := buildWhereClause(params, escaped)

	// Must contain the project_id predicate.
	if !strings.Contains(where, "project_id = ?") {
		t.Errorf("WHERE clause missing project_id predicate: %s", where)
	}
	// Must contain all four LIKE predicates.
	for _, field := range []string{"search_prompt", "search_completion", "search_tool_input", "search_tool_output"} {
		if !strings.Contains(where, field) {
			t.Errorf("WHERE clause missing LIKE predicate for %s", field)
		}
	}
	// Must NOT contain optional filter predicates.
	if strings.Contains(where, "agent_span_kind") {
		t.Error("WHERE clause should not contain agent_span_kind when SpanKind is empty")
	}
	if strings.Contains(where, "start_time") {
		t.Error("WHERE clause should not contain start_time when From/To are zero")
	}
	// args: 1 (project_id) + 4 (LIKE terms) = 5
	if len(args) != 5 {
		t.Errorf("expected 5 args with no optional filters, got %d", len(args))
	}
}

func TestBuildWhereClause_SpanKindFilter(t *testing.T) {
	params := &domain.SearchParams{
		ProjectID: "proj-abc",
		SpanKind:  domain.SpanKindLLMCall,
	}
	where, args := buildWhereClause(params, "term")

	if !strings.Contains(where, "agent_span_kind = ?") {
		t.Errorf("WHERE clause missing agent_span_kind predicate: %s", where)
	}
	// args: 5 base + 1 span_kind = 6
	if len(args) != 6 {
		t.Errorf("expected 6 args with SpanKind filter, got %d", len(args))
	}
	// The last arg must be the span kind string.
	if args[5] != string(domain.SpanKindLLMCall) {
		t.Errorf("expected args[5]=%q, got %q", string(domain.SpanKindLLMCall), args[5])
	}
}

func TestBuildWhereClause_FromFilter(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	params := &domain.SearchParams{
		ProjectID: "proj-abc",
		From:      from,
	}
	where, args := buildWhereClause(params, "term")

	if !strings.Contains(where, "start_time >= ?") {
		t.Errorf("WHERE clause missing start_time >= predicate: %s", where)
	}
	// args: 5 base + 1 from = 6
	if len(args) != 6 {
		t.Errorf("expected 6 args with From filter, got %d", len(args))
	}
	if args[5] != from {
		t.Errorf("expected args[5]=%v, got %v", from, args[5])
	}
}

func TestBuildWhereClause_ToFilter(t *testing.T) {
	to := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	params := &domain.SearchParams{
		ProjectID: "proj-abc",
		To:        to,
	}
	where, args := buildWhereClause(params, "term")

	if !strings.Contains(where, "start_time <= ?") {
		t.Errorf("WHERE clause missing start_time <= predicate: %s", where)
	}
	// args: 5 base + 1 to = 6
	if len(args) != 6 {
		t.Errorf("expected 6 args with To filter, got %d", len(args))
	}
	if args[5] != to {
		t.Errorf("expected args[5]=%v, got %v", to, args[5])
	}
}

func TestBuildWhereClause_AllOptionalFilters(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	params := &domain.SearchParams{
		ProjectID: "proj-abc",
		SpanKind:  domain.SpanKindToolCall,
		From:      from,
		To:        to,
	}
	where, args := buildWhereClause(params, "term")

	for _, fragment := range []string{"agent_span_kind = ?", "start_time >= ?", "start_time <= ?"} {
		if !strings.Contains(where, fragment) {
			t.Errorf("WHERE clause missing %q with all filters: %s", fragment, where)
		}
	}
	// args: 5 base + 3 optional = 8
	if len(args) != 8 {
		t.Errorf("expected 8 args with all three optional filters, got %d", len(args))
	}
}

func TestBuildWhereClause_ArgsOrderWithAllFilters(t *testing.T) {
	// Verify arg positions: [0]=projectID [1..4]=escaped term [5]=spanKind [6]=from [7]=to
	from := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)
	params := &domain.SearchParams{
		ProjectID: "proj-order",
		SpanKind:  domain.SpanKindMemoryRead,
		From:      from,
		To:        to,
	}
	escaped := "escaped_term"
	_, args := buildWhereClause(params, escaped)

	if args[0] != "proj-order" {
		t.Errorf("args[0] should be projectID, got %v", args[0])
	}
	for i := 1; i <= 4; i++ {
		if args[i] != escaped {
			t.Errorf("args[%d] should be escaped term %q, got %v", i, escaped, args[i])
		}
	}
	if args[5] != string(domain.SpanKindMemoryRead) {
		t.Errorf("args[5] should be span kind, got %v", args[5])
	}
	if args[6] != from {
		t.Errorf("args[6] should be from time, got %v", args[6])
	}
	if args[7] != to {
		t.Errorf("args[7] should be to time, got %v", args[7])
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
