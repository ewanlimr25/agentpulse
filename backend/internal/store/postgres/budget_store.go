package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// BudgetStore implements store.BudgetStore against Postgres.
type BudgetStore struct {
	pool *pgxpool.Pool
}

func NewBudgetStore(pool *pgxpool.Pool) *BudgetStore {
	return &BudgetStore{pool: pool}
}

func (s *BudgetStore) ListRules(ctx context.Context, projectID string) ([]*domain.BudgetRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, name, threshold_usd, action,
		       scope, window_seconds, webhook_url, enabled, created_at, updated_at
		FROM budget_rules
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("budget_store list_rules: %w", err)
	}
	defer rows.Close()

	var out []*domain.BudgetRule
	for rows.Next() {
		r := &domain.BudgetRule{}
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.Name, &r.ThresholdUSD, &r.Action,
			&r.Scope, &r.WindowSeconds, &r.WebhookURL, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("budget_store rule scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *BudgetStore) GetRule(ctx context.Context, id string) (*domain.BudgetRule, error) {
	r := &domain.BudgetRule{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, name, threshold_usd, action,
		       scope, window_seconds, webhook_url, enabled, created_at, updated_at
		FROM budget_rules WHERE id = $1
	`, id).Scan(
		&r.ID, &r.ProjectID, &r.Name, &r.ThresholdUSD, &r.Action,
		&r.Scope, &r.WindowSeconds, &r.WebhookURL, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("budget_store get_rule %s: %w", id, err)
	}
	return r, nil
}

func (s *BudgetStore) CreateRule(ctx context.Context, r *domain.BudgetRule) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budget_rules
		  (id, project_id, name, threshold_usd, action, scope, window_seconds, webhook_url, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, r.ID, r.ProjectID, r.Name, r.ThresholdUSD, r.Action,
		r.Scope, r.WindowSeconds, r.WebhookURL, r.Enabled)
	if err != nil {
		return fmt.Errorf("budget_store create_rule: %w", err)
	}
	return nil
}

func (s *BudgetStore) UpdateRule(ctx context.Context, r *domain.BudgetRule) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE budget_rules
		SET name=$2, threshold_usd=$3, action=$4, scope=$5,
		    window_seconds=$6, webhook_url=$7, enabled=$8, updated_at=now()
		WHERE id=$1
	`, r.ID, r.Name, r.ThresholdUSD, r.Action, r.Scope,
		r.WindowSeconds, r.WebhookURL, r.Enabled)
	if err != nil {
		return fmt.Errorf("budget_store update_rule: %w", err)
	}
	return nil
}

func (s *BudgetStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM budget_rules WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("budget_store delete_rule %s: %w", id, err)
	}
	return nil
}

func (s *BudgetStore) ListRecentAlerts(ctx context.Context, projectID string, limit int) ([]*domain.RecentBudgetAlert, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ba.id, ba.rule_id, ba.project_id, ba.run_id,
		       ba.triggered_at, ba.current_cost, ba.threshold_usd, ba.action_taken,
		       p.name  AS project_name,
		       br.name AS rule_name
		FROM budget_alerts ba
		JOIN projects p    ON p.id  = ba.project_id
		JOIN budget_rules br ON br.id = ba.rule_id
		WHERE ba.project_id = $1
		ORDER BY ba.triggered_at DESC
		LIMIT $2
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("budget_store list_recent_alerts: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecentBudgetAlert
	for rows.Next() {
		a := &domain.RecentBudgetAlert{}
		if err := rows.Scan(
			&a.ID, &a.RuleID, &a.ProjectID, &a.RunID,
			&a.TriggeredAt, &a.CurrentCost, &a.ThresholdUSD, &a.ActionTaken,
			&a.ProjectName, &a.RuleName,
		); err != nil {
			return nil, fmt.Errorf("budget_store recent alert scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *BudgetStore) ListAlerts(ctx context.Context, projectID string, limit int) ([]*domain.BudgetAlert, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule_id, project_id, run_id,
		       triggered_at, current_cost, threshold_usd, action_taken, metadata
		FROM budget_alerts
		WHERE project_id = $1
		ORDER BY triggered_at DESC
		LIMIT $2
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("budget_store list_alerts: %w", err)
	}
	defer rows.Close()

	var out []*domain.BudgetAlert
	for rows.Next() {
		a := &domain.BudgetAlert{}
		var metaRaw []byte
		if err := rows.Scan(
			&a.ID, &a.RuleID, &a.ProjectID, &a.RunID,
			&a.TriggeredAt, &a.CurrentCost, &a.ThresholdUSD, &a.ActionTaken, &metaRaw,
		); err != nil {
			return nil, fmt.Errorf("budget_store alert scan: %w", err)
		}
		if len(metaRaw) > 0 {
			if err := json.Unmarshal(metaRaw, &a.Metadata); err != nil {
				return nil, fmt.Errorf("budget_store alert metadata: %w", err)
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
