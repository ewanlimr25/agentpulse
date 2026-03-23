package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// SessionStore reads session aggregates from the session_agg AggregatingMergeTree MV.
// All queries enforce project_id scoping to prevent cross-project data leakage.
type SessionStore struct {
	conn driver.Conn
}

func NewSessionStore(conn driver.Conn) *SessionStore {
	return &SessionStore{conn: conn}
}

// listSessionsQuery reads from the session_agg MV using -Merge combinators.
// Orders by last_run (maxMerge of last_run_at_state) descending.
const listSessionsQuery = `
SELECT
    session_id,
    project_id,
    uniqMerge(run_count_state)          AS run_count,
    sumMerge(total_cost_state)          AS total_cost_usd,
    sumMerge(total_tokens_state)        AS total_tokens,
    sumMerge(input_tokens_state)        AS input_tokens,
    sumMerge(output_tokens_state)       AS output_tokens,
    countMerge(error_count_state)       AS error_count,
    minMerge(first_run_at_state)        AS first_run_at,
    maxMerge(last_run_at_state)         AS last_run_at
FROM session_agg
WHERE project_id = ?
GROUP BY session_id, project_id
ORDER BY last_run_at DESC
LIMIT ? OFFSET ?
`

const countSessionsQuery = `
SELECT count() FROM (
    SELECT session_id
    FROM session_agg
    WHERE project_id = ?
    GROUP BY session_id
)
`

const getSessionQuery = `
SELECT
    session_id,
    project_id,
    uniqMerge(run_count_state)          AS run_count,
    sumMerge(total_cost_state)          AS total_cost_usd,
    sumMerge(total_tokens_state)        AS total_tokens,
    sumMerge(input_tokens_state)        AS input_tokens,
    sumMerge(output_tokens_state)       AS output_tokens,
    countMerge(error_count_state)       AS error_count,
    minMerge(first_run_at_state)        AS first_run_at,
    maxMerge(last_run_at_state)         AS last_run_at
FROM session_agg
WHERE project_id = ? AND session_id = ?
GROUP BY session_id, project_id
LIMIT 1
`

func (s *SessionStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.Session, error) {
	rows, err := s.conn.Query(ctx, listSessionsQuery, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("session_store list query: %w", err)
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
	row := s.conn.QueryRow(ctx, countSessionsQuery, projectID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("session_store count query: %w", err)
	}
	return int(n), nil
}

func (s *SessionStore) Get(ctx context.Context, projectID, sessionID string) (*domain.Session, error) {
	rows, err := s.conn.Query(ctx, getSessionQuery, projectID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session_store get query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("session %q not found in project %q", sessionID, projectID)
	}
	sess, err := scanSession(rows)
	if err != nil {
		return nil, err
	}
	return sess, rows.Err()
}

func scanSession(rows driver.Rows) (*domain.Session, error) {
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
