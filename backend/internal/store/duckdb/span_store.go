//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ErrSpanNotFound is returned by GetByID when no span matches the project + span IDs.
var ErrSpanNotFound = errors.New("span not found")

// SpanStore implements store.SpanStore against DuckDB. It also exposes Insert
// for the embedded OTLP receiver — the team-mode equivalent lives in the
// collector exporter.
type SpanStore struct {
	db *sql.DB
}

func NewSpanStore(db *sql.DB) *SpanStore { return &SpanStore{db: db} }

// SpanRow is the row layout for direct inserts from the embedded OTLP receiver.
// Mirrors collector/exporter/clickhouseexporter spanRow but uses Go-native types
// throughout (no driver-specific Map binding).
type SpanRow struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	RunID         string
	ProjectID     string
	SessionID     string
	UserID        string
	AgentSpanKind string
	AgentName     string
	ModelID       string
	SpanName      string
	ServiceName   string
	StatusCode    string
	StatusMessage string
	StartTime     time.Time
	EndTime       time.Time
	InputTokens   uint32
	OutputTokens  uint32
	CostUSD       float64
	TtftMs        float64
	Attributes    map[string]string
	ResourceAttrs map[string]string
	Events        []domain.SpanEvent
	PayloadS3Key  string
}

// Insert appends rows to the spans table. Rows is a single transaction —
// either all rows commit or none do.
func (s *SpanStore) Insert(ctx context.Context, rows []SpanRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("span_store insert begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // safe no-op after Commit
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO spans (
			trace_id, span_id, parent_span_id,
			run_id, project_id, session_id, user_id,
			agent_span_kind, agent_name, model_id,
			span_name, service_name, status_code, status_message,
			start_time, end_time,
			input_tokens, output_tokens, cost_usd, ttft_ms,
			attributes, resource_attrs, events,
			payload_s3_key
		) VALUES (
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?
		)`)
	if err != nil {
		return fmt.Errorf("span_store insert prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		attrsJSON, _ := json.Marshal(r.Attributes)
		resAttrsJSON, _ := json.Marshal(r.ResourceAttrs)
		eventsJSON, _ := json.Marshal(r.Events)

		if _, err := stmt.ExecContext(ctx,
			r.TraceID, r.SpanID, r.ParentSpanID,
			r.RunID, r.ProjectID, r.SessionID, r.UserID,
			r.AgentSpanKind, r.AgentName, r.ModelID,
			r.SpanName, r.ServiceName, r.StatusCode, r.StatusMessage,
			r.StartTime.UTC(), r.EndTime.UTC(),
			int32(r.InputTokens), int32(r.OutputTokens), r.CostUSD, r.TtftMs,
			string(attrsJSON), string(resAttrsJSON), string(eventsJSON),
			r.PayloadS3Key,
		); err != nil {
			return fmt.Errorf("span_store insert exec: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("span_store insert commit: %w", err)
	}
	return nil
}

const selectSpanCols = `
trace_id, span_id, parent_span_id,
run_id, project_id,
agent_span_kind, agent_name, model_id,
span_name, service_name, status_code, status_message,
start_time, end_time,
input_tokens, output_tokens, cost_usd, ttft_ms,
attributes, resource_attrs,
payload_s3_key
`

func (s *SpanStore) ListByRun(ctx context.Context, runID string) ([]*domain.Span, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectSpanCols+` FROM spans WHERE run_id = ? ORDER BY start_time ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("span_store list_by_run: %w", err)
	}
	return scanSpans(rows)
}

func (s *SpanStore) ListLLMSpansByRun(ctx context.Context, runID string) ([]*domain.Span, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectSpanCols+` FROM spans WHERE run_id = ? AND agent_span_kind = 'llm.call' ORDER BY start_time ASC, span_id ASC`,
		runID)
	if err != nil {
		return nil, fmt.Errorf("span_store list_llm: %w", err)
	}
	return scanSpans(rows)
}

func (s *SpanStore) GetByID(ctx context.Context, projectID, spanID string) (*domain.Span, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectSpanCols+` FROM spans WHERE project_id = ? AND span_id = ? LIMIT 1`,
		projectID, spanID)
	if err != nil {
		return nil, fmt.Errorf("span_store get_by_id: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrSpanNotFound
	}
	return scanSpan(rows)
}

func (s *SpanStore) LatestSpanTime(ctx context.Context, projectID string) (*time.Time, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT max(start_time), count(*) FROM spans WHERE project_id = ? AND start_time > now() - INTERVAL 24 HOUR`,
		projectID)
	var maxTime sql.NullTime
	var count uint64
	if err := row.Scan(&maxTime, &count); err != nil {
		return nil, fmt.Errorf("span_store latest: %w", err)
	}
	if count == 0 || !maxTime.Valid {
		return nil, nil
	}
	t := maxTime.Time.UTC()
	return &t, nil
}

func (s *SpanStore) ListByRunSince(ctx context.Context, runID string, since time.Time) ([]*domain.Span, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectSpanCols+` FROM spans WHERE run_id = ? AND start_time > ? ORDER BY start_time ASC`,
		runID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("span_store list_by_run_since: %w", err)
	}
	return scanSpans(rows)
}

func (s *SpanStore) ListByProjectSince(ctx context.Context, projectID string, since time.Time) ([]*domain.Span, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectSpanCols+` FROM spans WHERE project_id = ? AND start_time > ? ORDER BY start_time ASC`,
		projectID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("span_store list_by_project_since: %w", err)
	}
	return scanSpans(rows)
}

func (s *SpanStore) CountSince(ctx context.Context, projectID string, window time.Duration) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM spans WHERE project_id = ? AND start_time > now() - INTERVAL '`+
			fmt.Sprintf("%d", int64(window.Seconds()))+` second'`, projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("span_store count_since: %w", err)
	}
	return n, nil
}

func scanSpans(rows *sql.Rows) ([]*domain.Span, error) {
	defer rows.Close()
	var out []*domain.Span
	for rows.Next() {
		sp, err := scanSpan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func scanSpan(rows *sql.Rows) (*domain.Span, error) {
	sp := &domain.Span{}
	var startTime, endTime time.Time
	var agentSpanKind, statusCode string
	var attrsJSON, resAttrsJSON string

	if err := rows.Scan(
		&sp.TraceID, &sp.SpanID, &sp.ParentSpanID,
		&sp.RunID, &sp.ProjectID,
		&agentSpanKind, &sp.AgentName, &sp.ModelID,
		&sp.SpanName, &sp.ServiceName, &statusCode, &sp.StatusMessage,
		&startTime, &endTime,
		&sp.InputTokens, &sp.OutputTokens, &sp.CostUSD, &sp.TtftMs,
		&attrsJSON, &resAttrsJSON,
		&sp.PayloadS3Key,
	); err != nil {
		return nil, fmt.Errorf("span_store scan: %w", err)
	}

	sp.AgentSpanKind = domain.AgentSpanKind(agentSpanKind)
	sp.StatusCode = domain.StatusCode(statusCode)
	sp.StartTime = startTime.UTC()
	sp.EndTime = endTime.UTC()
	sp.DurationNS = uint64(endTime.Sub(startTime).Nanoseconds())
	sp.TotalTokens = sp.InputTokens + sp.OutputTokens

	sp.Attributes = decodeStringMap(attrsJSON)
	sp.ResourceAttrs = decodeStringMap(resAttrsJSON)

	return sp, nil
}

func decodeStringMap(raw string) map[string]string {
	if raw == "" || raw == "null" {
		return nil
	}
	m := make(map[string]string)
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
