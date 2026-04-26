//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// SessionStore implements store.SessionStore against DuckDB by aggregating
// the spans table at query time. The team-mode equivalent reads from a
// ClickHouse AggregatingMergeTree MV (session_agg); DuckDB has no MVs so we
// run the same aggregation directly against `spans`. At indie scale this is
// well within budget.
type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore { return &SessionStore{db: db} }

// sessionAggCols computes per-session aggregates from spans grouped by
// (session_id, project_id). Mirrors the columns produced by session_agg.
const sessionAggCols = `
    session_id,
    any_value(project_id)             AS project_id,
    count(DISTINCT run_id)            AS run_count,
    sum(cost_usd)                     AS total_cost_usd,
    sum(input_tokens + output_tokens) AS total_tokens,
    sum(input_tokens)                 AS input_tokens,
    sum(output_tokens)                AS output_tokens,
    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
    min(start_time)                   AS first_run_at,
    max(start_time)                   AS last_run_at
`

func (s *SessionStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionAggCols+`
		 FROM spans
		 WHERE project_id = ? AND session_id != ''
		 GROUP BY session_id
		 ORDER BY last_run_at DESC
		 LIMIT ? OFFSET ?`,
		projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("session_store list: %w", err)
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *SessionStore) Count(ctx context.Context, projectID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(DISTINCT session_id)
		 FROM spans
		 WHERE project_id = ? AND session_id != ''`,
		projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("session_store count: %w", err)
	}
	return n, nil
}

func (s *SessionStore) Get(ctx context.Context, projectID, sessionID string) (*domain.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionAggCols+`
		 FROM spans
		 WHERE project_id = ? AND session_id = ?
		 GROUP BY session_id
		 LIMIT 1`,
		projectID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session_store get: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("session %q not found in project %q", sessionID, projectID)
	}
	return scanSession(rows)
}

func scanSession(rows *sql.Rows) (*domain.Session, error) {
	s := &domain.Session{}
	var firstRunAt, lastRunAt time.Time
	if err := rows.Scan(
		&s.SessionID, &s.ProjectID,
		&s.RunCount, &s.TotalCostUSD,
		&s.TotalTokens, &s.InputTokens, &s.OutputTokens,
		&s.ErrorCount,
		&firstRunAt, &lastRunAt,
	); err != nil {
		return nil, fmt.Errorf("session_store scan: %w", err)
	}
	s.FirstRunAt = firstRunAt.UTC()
	s.LastRunAt = lastRunAt.UTC()
	return s, nil
}
