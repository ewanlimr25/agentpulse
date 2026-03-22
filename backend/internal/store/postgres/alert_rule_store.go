package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// AlertRuleStore implements store.AlertRuleStore against Postgres.
type AlertRuleStore struct {
	pool *pgxpool.Pool
}

func NewAlertRuleStore(pool *pgxpool.Pool) *AlertRuleStore {
	return &AlertRuleStore{pool: pool}
}

const alertRuleColumns = `id, project_id, name, signal_type, threshold, compare_op,
	window_seconds, scope_filter, webhook_url, enabled, created_at, updated_at`

func scanAlertRule(row interface {
	Scan(...any) error
}) (*domain.AlertRule, error) {
	r := &domain.AlertRule{}
	err := row.Scan(
		&r.ID, &r.ProjectID, &r.Name, &r.SignalType, &r.Threshold, &r.CompareOp,
		&r.WindowSeconds, &r.ScopeFilter, &r.WebhookURL, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

func (s *AlertRuleStore) ListRules(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+alertRuleColumns+`
		FROM alert_rules WHERE project_id = $1 ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store list_rules: %w", err)
	}
	defer rows.Close()

	var out []*domain.AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("alert_rule_store rule scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AlertRuleStore) GetRule(ctx context.Context, id string) (*domain.AlertRule, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+alertRuleColumns+` FROM alert_rules WHERE id = $1
	`, id)
	r, err := scanAlertRule(row)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store get_rule %s: %w", id, err)
	}
	return r, nil
}

func (s *AlertRuleStore) CreateRule(ctx context.Context, r *domain.AlertRule) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO alert_rules
		  (id, project_id, name, signal_type, threshold, compare_op,
		   window_seconds, scope_filter, webhook_url, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, r.ID, r.ProjectID, r.Name, r.SignalType, r.Threshold, r.CompareOp,
		r.WindowSeconds, r.ScopeFilter, r.WebhookURL, r.Enabled)
	if err != nil {
		return fmt.Errorf("alert_rule_store create_rule: %w", err)
	}
	return nil
}

func (s *AlertRuleStore) UpdateRule(ctx context.Context, r *domain.AlertRule) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE alert_rules
		SET name=$2, signal_type=$3, threshold=$4, compare_op=$5,
		    window_seconds=$6, scope_filter=$7, webhook_url=$8, enabled=$9, updated_at=now()
		WHERE id=$1
	`, r.ID, r.Name, r.SignalType, r.Threshold, r.CompareOp,
		r.WindowSeconds, r.ScopeFilter, r.WebhookURL, r.Enabled)
	if err != nil {
		return fmt.Errorf("alert_rule_store update_rule: %w", err)
	}
	return nil
}

func (s *AlertRuleStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("alert_rule_store delete_rule %s: %w", id, err)
	}
	return nil
}

func (s *AlertRuleStore) ListEnabledRules(ctx context.Context) ([]*domain.AlertRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+alertRuleColumns+`
		FROM alert_rules WHERE enabled = true ORDER BY project_id, created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store list_enabled: %w", err)
	}
	defer rows.Close()

	var out []*domain.AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("alert_rule_store enabled scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AlertRuleStore) ListEvents(ctx context.Context, projectID string, limit int) ([]*domain.AlertEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule_id, project_id, triggered_at, signal_type,
		       current_value, threshold, compare_op, action_taken, metadata
		FROM alert_events
		WHERE project_id = $1
		ORDER BY triggered_at DESC
		LIMIT $2
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store list_events: %w", err)
	}
	defer rows.Close()

	var out []*domain.AlertEvent
	for rows.Next() {
		e, err := scanAlertEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("alert_rule_store event scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *AlertRuleStore) CreateEvent(ctx context.Context, e *domain.AlertEvent) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		meta = []byte("{}")
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO alert_events
		  (id, rule_id, project_id, triggered_at, signal_type,
		   current_value, threshold, compare_op, action_taken, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, e.ID, e.RuleID, e.ProjectID, e.TriggeredAt, e.SignalType,
		e.CurrentValue, e.Threshold, e.CompareOp, e.ActionTaken, meta)
	if err != nil {
		return fmt.Errorf("alert_rule_store create_event: %w", err)
	}
	return nil
}

func (s *AlertRuleStore) LastEventForRule(ctx context.Context, ruleID string) (*domain.AlertEvent, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, rule_id, project_id, triggered_at, signal_type,
		       current_value, threshold, compare_op, action_taken, metadata
		FROM alert_events
		WHERE rule_id = $1
		ORDER BY triggered_at DESC
		LIMIT 1
	`, ruleID)
	e, err := scanAlertEvent(row)
	if err != nil {
		return nil, nil // no prior event — not an error
	}
	return e, nil
}

func (s *AlertRuleStore) ListRecentEvents(ctx context.Context, limit int) ([]*domain.RecentAlertEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ae.id, ae.rule_id, ae.project_id, ae.triggered_at, ae.signal_type,
		       ae.current_value, ae.threshold, ae.compare_op, ae.action_taken, ae.metadata,
		       p.name  AS project_name,
		       ar.name AS rule_name
		FROM alert_events ae
		JOIN projects    p  ON p.id  = ae.project_id
		JOIN alert_rules ar ON ar.id = ae.rule_id
		ORDER BY ae.triggered_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store list_recent: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecentAlertEvent
	for rows.Next() {
		r := &domain.RecentAlertEvent{}
		var metaRaw []byte
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.ProjectID, &r.TriggeredAt, &r.SignalType,
			&r.CurrentValue, &r.Threshold, &r.CompareOp, &r.ActionTaken, &metaRaw,
			&r.ProjectName, &r.RuleName,
		); err != nil {
			return nil, fmt.Errorf("alert_rule_store recent scan: %w", err)
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &r.Metadata)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanAlertEvent(row interface {
	Scan(...any) error
}) (*domain.AlertEvent, error) {
	e := &domain.AlertEvent{}
	var metaRaw []byte
	err := row.Scan(
		&e.ID, &e.RuleID, &e.ProjectID, &e.TriggeredAt, &e.SignalType,
		&e.CurrentValue, &e.Threshold, &e.CompareOp, &e.ActionTaken, &metaRaw,
	)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &e.Metadata)
	}
	return e, nil
}
