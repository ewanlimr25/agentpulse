//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// UserStore implements store.UserStore against DuckDB by aggregating spans
// at query time. The team-mode equivalent reads from the ClickHouse user_agg
// MV via -Merge combinators; DuckDB has no MVs so we group spans directly.
type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{db: db} }

const userAggCols = `
    user_id,
    any_value(project_id)                  AS project_id,
    count(DISTINCT run_id)                 AS run_count,
    sum(cost_usd)                          AS total_cost_usd,
    sum(input_tokens + output_tokens)      AS total_tokens,
    sum(input_tokens)                      AS input_tokens,
    sum(output_tokens)                     AS output_tokens,
    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
    min(start_time)                        AS first_seen_at,
    max(start_time)                        AS last_seen_at
`

func (s *UserStore) List(ctx context.Context, projectID string, limit, offset int) ([]*domain.UserStats, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userAggCols+`
		 FROM spans
		 WHERE project_id = ? AND user_id != ''
		 GROUP BY user_id
		 ORDER BY total_cost_usd DESC
		 LIMIT ? OFFSET ?`,
		projectID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("user_store list: %w", err)
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

	if totalCost > 0 {
		for _, u := range users {
			u.CostPercent = (u.TotalCostUSD / totalCost) * 100
		}
	}
	return users, nil
}

func (s *UserStore) Count(ctx context.Context, projectID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(DISTINCT user_id)
		 FROM spans
		 WHERE project_id = ? AND user_id != ''`, projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("user_store count: %w", err)
	}
	return n, nil
}

func scanUser(rows *sql.Rows) (*domain.UserStats, error) {
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
