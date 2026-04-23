// Package postgres contains Postgres-backed store implementations.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// PushSubscriptionStore implements store.PushSubscriptionStore against Postgres.
type PushSubscriptionStore struct {
	pool *pgxpool.Pool
}

// NewPushSubscriptionStore returns a new PushSubscriptionStore backed by pool.
func NewPushSubscriptionStore(pool *pgxpool.Pool) *PushSubscriptionStore {
	return &PushSubscriptionStore{pool: pool}
}

// Upsert inserts or updates a push subscription keyed on (project_id, endpoint).
func (s *PushSubscriptionStore) Upsert(ctx context.Context, sub *domain.PushSubscription) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_subscriptions
		  (project_id, endpoint, p256dh_key, auth_key, vapid_public_key, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, endpoint) DO UPDATE
		  SET p256dh_key       = EXCLUDED.p256dh_key,
		      auth_key         = EXCLUDED.auth_key,
		      vapid_public_key = EXCLUDED.vapid_public_key,
		      user_agent       = EXCLUDED.user_agent
	`, sub.ProjectID, sub.Endpoint, sub.P256DHKey, sub.AuthKey, sub.VAPIDPublicKey, sub.UserAgent)
	if err != nil {
		return fmt.Errorf("push_subscription_store upsert: %w", err)
	}
	return nil
}

// ListByProject returns all push subscriptions for a project.
func (s *PushSubscriptionStore) ListByProject(ctx context.Context, projectID string) ([]*domain.PushSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, endpoint, p256dh_key, auth_key, vapid_public_key,
		       COALESCE(user_agent, ''), created_at
		FROM push_subscriptions
		WHERE project_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("push_subscription_store list_by_project: %w", err)
	}
	defer rows.Close()

	var out []*domain.PushSubscription
	for rows.Next() {
		sub := &domain.PushSubscription{}
		if err := rows.Scan(
			&sub.ID, &sub.ProjectID, &sub.Endpoint, &sub.P256DHKey, &sub.AuthKey,
			&sub.VAPIDPublicKey, &sub.UserAgent, &sub.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("push_subscription_store scan: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// Delete removes a push subscription identified by (project_id, endpoint).
func (s *PushSubscriptionStore) Delete(ctx context.Context, projectID, endpoint string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM push_subscriptions WHERE project_id = $1 AND endpoint = $2
	`, projectID, endpoint)
	if err != nil {
		return fmt.Errorf("push_subscription_store delete: %w", err)
	}
	return nil
}
