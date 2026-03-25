package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// UserStore reads user cost aggregates from the user_agg AggregatingMergeTree MV.
// All queries enforce project_id scoping to prevent cross-project data leakage.
type UserStore struct {
	conn driver.Conn
}

func NewUserStore(conn driver.Conn) *UserStore {
	return &UserStore{conn: conn}
}

// listUsersQuery reads from the user_agg MV using -Merge combinators.
// Orders by total cost descending (most expensive users first).
const listUsersQuery = `
SELECT
    user_id,
    project_id,
    uniqMerge(run_count_state)          AS run_count,
    sumMerge(total_cost_state)          AS total_cost_usd,
    sumMerge(total_tokens_state)        AS total_tokens,
    sumMerge(input_tokens_state)        AS input_tokens,
    sumMerge(output_tokens_state)       AS output_tokens,
    countMerge(error_count_state)       AS error_count,
    minMerge(first_run_at_state)        AS first_seen_at,
    maxMerge(last_run_at_state)         AS last_seen_at
FROM user_agg
WHERE project_id = ?
GROUP BY user_id, project_id
ORDER BY total_cost_usd DESC
LIMIT ? OFFSET ?
`

const countUsersQuery = `
SELECT count() FROM (
    SELECT user_id
    FROM user_agg
    WHERE project_id = ?
    GROUP BY user_id
)
`

func (s *UserStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.UserStats, error) {
	rows, err := s.conn.Query(ctx, listUsersQuery, projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("user_store list query: %w", err)
	}
	defer rows.Close()

	var users []*domain.UserStats
	var totalCost float64
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
		totalCost += u.TotalCostUSD
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute cost percentages after we know the total.
	if totalCost > 0 {
		for _, u := range users {
			u.CostPercent = (u.TotalCostUSD / totalCost) * 100
		}
	}
	return users, nil
}

func (s *UserStore) Count(ctx context.Context, projectID string) (int, error) {
	row := s.conn.QueryRow(ctx, countUsersQuery, projectID)
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("user_store count query: %w", err)
	}
	return int(n), nil
}

func scanUser(rows driver.Rows) (*domain.UserStats, error) {
	u := &domain.UserStats{}
	var firstSeen, lastSeen time.Time
	if err := rows.Scan(
		&u.UserID, &u.ProjectID,
		&u.RunCount, &u.TotalCostUSD,
		&u.TotalTokens, &u.InputTokens, &u.OutputTokens,
		&u.ErrorCount,
		&firstSeen, &lastSeen,
	); err != nil {
		return nil, fmt.Errorf("user_store scan: %w", err)
	}
	u.FirstSeenAt = firstSeen.UTC()
	u.LastSeenAt = lastSeen.UTC()
	return u, nil
}
