package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// BudgetStore implements store.BudgetStore against SQLite.
type BudgetStore struct {
	db *sql.DB
}

func NewBudgetStore(db *sql.DB) *BudgetStore {
	return &BudgetStore{db: db}
}

// scanBudgetRule reads one row of the canonical budget_rules SELECT into r.
// Used by both ListRules and GetRule to keep nullable handling in one place.
func scanBudgetRule(scanner interface {
	Scan(dest ...any) error
}, r *domain.BudgetRule) error {
	var (
		action        string
		scope         string
		windowSeconds sql.NullInt64
		webhookURL    sql.NullString
	)
	if err := scanner.Scan(
		&r.ID, &r.ProjectID, &r.Name, &r.ThresholdUSD, &action,
		&scope, &windowSeconds, &webhookURL, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return err
	}
	r.Action = domain.BudgetAction(action)
	r.Scope = domain.BudgetScope(scope)
	if windowSeconds.Valid {
		v := int(windowSeconds.Int64)
		r.WindowSeconds = &v
	}
	if webhookURL.Valid {
		v := webhookURL.String
		r.WebhookURL = &v
	}
	return nil
}

func (s *BudgetStore) ListRules(ctx context.Context, projectID string) ([]*domain.BudgetRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, threshold_usd, action,
		       scope, window_seconds, webhook_url, enabled, created_at, updated_at
		FROM budget_rules
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("budget_store list_rules: %w", err)
	}
	defer rows.Close()

	var out []*domain.BudgetRule
	for rows.Next() {
		r := &domain.BudgetRule{}
		if err := scanBudgetRule(rows, r); err != nil {
			return nil, fmt.Errorf("budget_store rule scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *BudgetStore) GetRule(ctx context.Context, id string) (*domain.BudgetRule, error) {
	r := &domain.BudgetRule{}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, threshold_usd, action,
		       scope, window_seconds, webhook_url, enabled, created_at, updated_at
		FROM budget_rules WHERE id = ?
	`, id)
	if err := scanBudgetRule(row, r); err != nil {
		return nil, fmt.Errorf("budget_store get_rule %s: %w", id, err)
	}
	return r, nil
}

func (s *BudgetStore) CreateRule(ctx context.Context, r *domain.BudgetRule) error {
	var (
		windowSeconds any
		webhookURL    any
	)
	if r.WindowSeconds != nil {
		windowSeconds = *r.WindowSeconds
	}
	if r.WebhookURL != nil {
		webhookURL = *r.WebhookURL
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budget_rules
		  (id, project_id, name, threshold_usd, action, scope, window_seconds, webhook_url, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.ProjectID, r.Name, r.ThresholdUSD, string(r.Action),
		string(r.Scope), windowSeconds, webhookURL, r.Enabled)
	if err != nil {
		return fmt.Errorf("budget_store create_rule: %w", err)
	}
	return nil
}

func (s *BudgetStore) UpdateRule(ctx context.Context, r *domain.BudgetRule) error {
	var (
		windowSeconds any
		webhookURL    any
	)
	if r.WindowSeconds != nil {
		windowSeconds = *r.WindowSeconds
	}
	if r.WebhookURL != nil {
		webhookURL = *r.WebhookURL
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE budget_rules
		SET name = ?, threshold_usd = ?, action = ?, scope = ?,
		    window_seconds = ?, webhook_url = ?, enabled = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?
	`, r.Name, r.ThresholdUSD, string(r.Action), string(r.Scope),
		windowSeconds, webhookURL, r.Enabled, r.ID)
	if err != nil {
		return fmt.Errorf("budget_store update_rule: %w", err)
	}
	return nil
}

func (s *BudgetStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM budget_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("budget_store delete_rule %s: %w", id, err)
	}
	return nil
}

func (s *BudgetStore) ListRecentAlerts(ctx context.Context, projectID string, limit int) ([]*domain.RecentBudgetAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ba.id, ba.rule_id, ba.project_id, ba.run_id,
		       ba.triggered_at, ba.current_cost, ba.threshold_usd, ba.action_taken,
		       p.name  AS project_name,
		       br.name AS rule_name
		FROM budget_alerts ba
		JOIN projects p     ON p.id  = ba.project_id
		JOIN budget_rules br ON br.id = ba.rule_id
		WHERE ba.project_id = ?
		ORDER BY ba.triggered_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("budget_store list_recent_alerts: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecentBudgetAlert
	for rows.Next() {
		a := &domain.RecentBudgetAlert{}
		var runID sql.NullString
		if err := rows.Scan(
			&a.ID, &a.RuleID, &a.ProjectID, &runID,
			&a.TriggeredAt, &a.CurrentCost, &a.ThresholdUSD, &a.ActionTaken,
			&a.ProjectName, &a.RuleName,
		); err != nil {
			return nil, fmt.Errorf("budget_store recent alert scan: %w", err)
		}
		if runID.Valid {
			v := runID.String
			a.RunID = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *BudgetStore) ListAlerts(ctx context.Context, projectID string, limit int) ([]*domain.BudgetAlert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_id, project_id, run_id,
		       triggered_at, current_cost, threshold_usd, action_taken, metadata
		FROM budget_alerts
		WHERE project_id = ?
		ORDER BY triggered_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("budget_store list_alerts: %w", err)
	}
	defer rows.Close()

	var out []*domain.BudgetAlert
	for rows.Next() {
		a := &domain.BudgetAlert{}
		var (
			runID   sql.NullString
			metaRaw sql.NullString
		)
		if err := rows.Scan(
			&a.ID, &a.RuleID, &a.ProjectID, &runID,
			&a.TriggeredAt, &a.CurrentCost, &a.ThresholdUSD, &a.ActionTaken, &metaRaw,
		); err != nil {
			return nil, fmt.Errorf("budget_store alert scan: %w", err)
		}
		if runID.Valid {
			v := runID.String
			a.RunID = &v
		}
		if metaRaw.Valid && metaRaw.String != "" {
			if err := json.Unmarshal([]byte(metaRaw.String), &a.Metadata); err != nil {
				return nil, fmt.Errorf("budget_store alert metadata: %w", err)
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
