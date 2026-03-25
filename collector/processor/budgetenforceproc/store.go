package budgetenforceproc

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// budgetScope mirrors domain.BudgetScope without importing the backend module.
type budgetScope string

const (
	scopeRun    budgetScope = "run"
	scopeAgent  budgetScope = "agent"
	scopeWindow budgetScope = "window"
	scopeUser   budgetScope = "user"
)

// budgetAction mirrors domain.BudgetAction.
type budgetAction string

const (
	actionNotify budgetAction = "notify"
	actionHalt   budgetAction = "halt"
)

// budgetRule is a local copy of a rule row from Postgres.
type budgetRule struct {
	id           string
	projectID    string
	name         string
	thresholdUSD float64
	action       budgetAction
	scope        budgetScope
	windowSeconds *int
	webhookURL   *string
	enabled      bool
}

// budgetStore handles Postgres reads/writes for the processor.
type budgetStore struct {
	pool *pgxpool.Pool
}

func newBudgetStore(ctx context.Context, dsn string) (*budgetStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("budget store: pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("budget store: ping: %w", err)
	}
	return &budgetStore{pool: pool}, nil
}

func (s *budgetStore) close() {
	s.pool.Close()
}

// loadRules reads all enabled rules from Postgres.
func (s *budgetStore) loadRules(ctx context.Context) ([]budgetRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, name, threshold_usd, action,
		       scope, window_seconds, webhook_url
		FROM budget_rules
		WHERE enabled = true
	`)
	if err != nil {
		return nil, fmt.Errorf("budget store load rules: %w", err)
	}
	defer rows.Close()

	var rules []budgetRule
	for rows.Next() {
		var r budgetRule
		if err := rows.Scan(
			&r.id, &r.projectID, &r.name, &r.thresholdUSD, &r.action,
			&r.scope, &r.windowSeconds, &r.webhookURL,
		); err != nil {
			return nil, fmt.Errorf("budget store scan rule: %w", err)
		}
		r.enabled = true
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// actionToPast converts action verbs ("notify","halt") to the past-tense form
// required by the budget_alerts.action_taken check constraint.
func actionToPast(action string) string {
	switch action {
	case "notify":
		return "notified"
	case "halt":
		return "halted"
	default:
		return action
	}
}

// lastAlertForUser returns the time of the most recent budget alert for a
// (rule, user) pair, or zero time if none. Used for window-based dedup of
// user-scoped budget alerts instead of the in-memory alertDedup map.
func (s *budgetStore) lastAlertForUser(ctx context.Context, ruleID, userID string) (time.Time, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT triggered_at
		FROM budget_alerts
		WHERE rule_id = $1 AND metadata->>'user_id' = $2
		ORDER BY triggered_at DESC
		LIMIT 1
	`, ruleID, userID)
	var t time.Time
	if err := row.Scan(&t); err != nil {
		if err.Error() == "no rows in result set" {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("budget store last alert for user: %w", err)
	}
	return t, nil
}

// writeUserAlert inserts a budget_alerts row for a user-scoped rule, storing user_id in metadata.
func (s *budgetStore) writeUserAlert(ctx context.Context, ruleID, projectID, userID string, cost, threshold float64, action string) error {
	if s.pool == nil {
		return nil // no-op in tests
	}
	metadata := fmt.Sprintf(`{"user_id": %q}`, userID)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budget_alerts
		  (id, rule_id, project_id, run_id, triggered_at, current_cost, threshold_usd, action_taken, metadata)
		VALUES
		  (gen_random_uuid(), $1, $2, NULL, $3, $4, $5, $6, $7::jsonb)
	`, ruleID, projectID, time.Now(), cost, threshold, actionToPast(action), metadata)
	if err != nil {
		return fmt.Errorf("budget store write user alert: %w", err)
	}
	return nil
}

// writeAlert inserts a budget_alerts row.
func (s *budgetStore) writeAlert(ctx context.Context, ruleID, projectID string, runID *string, cost, threshold float64, action string) error {
	if s.pool == nil {
		return nil // no-op in tests
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budget_alerts
		  (id, rule_id, project_id, run_id, triggered_at, current_cost, threshold_usd, action_taken, metadata)
		VALUES
		  (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, '{}')
	`, ruleID, projectID, runID, time.Now(), cost, threshold, actionToPast(action))
	if err != nil {
		return fmt.Errorf("budget store write alert: %w", err)
	}
	return nil
}
