package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// AlertRuleStore implements store.AlertRuleStore against SQLite.
type AlertRuleStore struct {
	db *sql.DB
}

func NewAlertRuleStore(db *sql.DB) *AlertRuleStore { return &AlertRuleStore{db: db} }

const alertRuleColumns = `id, project_id, name, signal_type, threshold, compare_op,
	window_seconds, scope_filter, webhook_url, webhook_secret, enabled, created_at, updated_at,
	slack_webhook_url, discord_webhook_url, last_channel_error, last_channel_error_at`

// scanAlertRule reads a single alert_rules row, handling nullable text and timestamp columns.
func scanAlertRule(row interface {
	Scan(...any) error
}) (*domain.AlertRule, error) {
	r := &domain.AlertRule{}
	var (
		scopeFilter        sql.NullString
		webhookURL         sql.NullString
		webhookSecret      sql.NullString
		slackWebhookURL    sql.NullString
		discordWebhookURL  sql.NullString
		lastChannelError   sql.NullString
		lastChannelErrorAt sql.NullTime
	)
	err := row.Scan(
		&r.ID, &r.ProjectID, &r.Name, &r.SignalType, &r.Threshold, &r.CompareOp,
		&r.WindowSeconds, &scopeFilter, &webhookURL, &webhookSecret, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
		&slackWebhookURL, &discordWebhookURL, &lastChannelError, &lastChannelErrorAt,
	)
	if err != nil {
		return nil, err
	}
	if scopeFilter.Valid {
		v := scopeFilter.String
		r.ScopeFilter = &v
	}
	if webhookURL.Valid {
		v := webhookURL.String
		r.WebhookURL = &v
	}
	if webhookSecret.Valid {
		v := webhookSecret.String
		r.WebhookSecret = &v
	}
	if slackWebhookURL.Valid {
		v := slackWebhookURL.String
		r.SlackWebhookURL = &v
	}
	if discordWebhookURL.Valid {
		v := discordWebhookURL.String
		r.DiscordWebhookURL = &v
	}
	if lastChannelError.Valid {
		v := lastChannelError.String
		r.LastChannelError = &v
	}
	if lastChannelErrorAt.Valid {
		t := lastChannelErrorAt.Time
		r.LastChannelErrorAt = &t
	}
	return r, nil
}

// generateWebhookSecret creates a cryptographically random 32-byte hex secret.
func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate_webhook_secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *AlertRuleStore) ListRules(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+alertRuleColumns+`
		FROM alert_rules WHERE project_id = ? ORDER BY created_at DESC
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
	row := s.db.QueryRowContext(ctx, `
		SELECT `+alertRuleColumns+` FROM alert_rules WHERE id = ?
	`, id)
	r, err := scanAlertRule(row)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store get_rule %s: %w", id, err)
	}
	return r, nil
}

func (s *AlertRuleStore) CreateRule(ctx context.Context, r *domain.AlertRule) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	// Auto-generate a signing secret for rules with a webhook URL.
	if r.WebhookURL != nil && *r.WebhookURL != "" && r.WebhookSecret == nil {
		secret, err := generateWebhookSecret()
		if err != nil {
			return err
		}
		r.WebhookSecret = &secret
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules
		  (id, project_id, name, signal_type, threshold, compare_op,
		   window_seconds, scope_filter, webhook_url, webhook_secret, enabled,
		   slack_webhook_url, discord_webhook_url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
	`, r.ID, r.ProjectID, r.Name, r.SignalType, r.Threshold, r.CompareOp,
		r.WindowSeconds, r.ScopeFilter, r.WebhookURL, r.WebhookSecret, r.Enabled,
		r.SlackWebhookURL, r.DiscordWebhookURL)
	if err != nil {
		return fmt.Errorf("alert_rule_store create_rule: %w", err)
	}
	return nil
}

func (s *AlertRuleStore) UpdateRule(ctx context.Context, r *domain.AlertRule) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules
		SET name=?, signal_type=?, threshold=?, compare_op=?,
		    window_seconds=?, scope_filter=?, webhook_url=?, enabled=?,
		    slack_webhook_url=?, discord_webhook_url=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=?
	`, r.Name, r.SignalType, r.Threshold, r.CompareOp,
		r.WindowSeconds, r.ScopeFilter, r.WebhookURL, r.Enabled,
		r.SlackWebhookURL, r.DiscordWebhookURL, r.ID)
	if err != nil {
		return fmt.Errorf("alert_rule_store update_rule: %w", err)
	}
	return nil
}

// UpdateChannelError records a delivery failure for a rule. Pass empty errMsg to clear.
func (s *AlertRuleStore) UpdateChannelError(ctx context.Context, ruleID, errMsg string) error {
	if errMsg == "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE alert_rules
			SET last_channel_error=NULL,
			    last_channel_error_at=NULL,
			    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id=?
		`, ruleID)
		if err != nil {
			return fmt.Errorf("alert_rule_store update_channel_error: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules
		SET last_channel_error=?,
		    last_channel_error_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),
		    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=?
	`, errMsg, ruleID)
	if err != nil {
		return fmt.Errorf("alert_rule_store update_channel_error: %w", err)
	}
	return nil
}

func (s *AlertRuleStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("alert_rule_store delete_rule %s: %w", id, err)
	}
	return nil
}

func (s *AlertRuleStore) ListEnabledRules(ctx context.Context) ([]*domain.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+alertRuleColumns+`
		FROM alert_rules WHERE enabled = 1 ORDER BY project_id, created_at
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_id, project_id, triggered_at, signal_type,
		       current_value, threshold, compare_op, action_taken, metadata
		FROM alert_events
		WHERE project_id = ?
		ORDER BY triggered_at DESC
		LIMIT ?
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
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		meta = []byte("{}")
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO alert_events
		  (id, rule_id, project_id, triggered_at, signal_type,
		   current_value, threshold, compare_op, action_taken, metadata)
		VALUES (?,?,?,?,?,?,?,?,?,?)
	`, e.ID, e.RuleID, e.ProjectID, e.TriggeredAt, e.SignalType,
		e.CurrentValue, e.Threshold, e.CompareOp, e.ActionTaken, string(meta))
	if err != nil {
		return fmt.Errorf("alert_rule_store create_event: %w", err)
	}
	return nil
}

func (s *AlertRuleStore) LastEventForRule(ctx context.Context, ruleID string) (*domain.AlertEvent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, rule_id, project_id, triggered_at, signal_type,
		       current_value, threshold, compare_op, action_taken, metadata
		FROM alert_events
		WHERE rule_id = ?
		ORDER BY triggered_at DESC
		LIMIT 1
	`, ruleID)
	e, err := scanAlertEvent(row)
	if err != nil {
		return nil, nil // no prior event — not an error
	}
	return e, nil
}

func (s *AlertRuleStore) ListRecentEvents(ctx context.Context, projectID string, limit int) ([]*domain.RecentAlertEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ae.id, ae.rule_id, ae.project_id, ae.triggered_at, ae.signal_type,
		       ae.current_value, ae.threshold, ae.compare_op, ae.action_taken, ae.metadata,
		       p.name  AS project_name,
		       ar.name AS rule_name
		FROM alert_events ae
		JOIN projects    p  ON p.id  = ae.project_id
		JOIN alert_rules ar ON ar.id = ae.rule_id
		WHERE ae.project_id = ?
		ORDER BY ae.triggered_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("alert_rule_store list_recent: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecentAlertEvent
	for rows.Next() {
		r := &domain.RecentAlertEvent{}
		var metaRaw sql.NullString
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.ProjectID, &r.TriggeredAt, &r.SignalType,
			&r.CurrentValue, &r.Threshold, &r.CompareOp, &r.ActionTaken, &metaRaw,
			&r.ProjectName, &r.RuleName,
		); err != nil {
			return nil, fmt.Errorf("alert_rule_store recent scan: %w", err)
		}
		if metaRaw.Valid && len(metaRaw.String) > 0 {
			_ = json.Unmarshal([]byte(metaRaw.String), &r.Metadata)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanAlertEvent(row interface {
	Scan(...any) error
}) (*domain.AlertEvent, error) {
	e := &domain.AlertEvent{}
	var metaRaw sql.NullString
	err := row.Scan(
		&e.ID, &e.RuleID, &e.ProjectID, &e.TriggeredAt, &e.SignalType,
		&e.CurrentValue, &e.Threshold, &e.CompareOp, &e.ActionTaken, &metaRaw,
	)
	if err != nil {
		return nil, err
	}
	if metaRaw.Valid && len(metaRaw.String) > 0 {
		_ = json.Unmarshal([]byte(metaRaw.String), &e.Metadata)
	}
	return e, nil
}
