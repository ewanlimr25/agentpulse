package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// PushSubscriptionStore implements store.PushSubscriptionStore against SQLite.
type PushSubscriptionStore struct {
	db *sql.DB
}

// NewPushSubscriptionStore returns a new PushSubscriptionStore backed by db.
func NewPushSubscriptionStore(db *sql.DB) *PushSubscriptionStore {
	return &PushSubscriptionStore{db: db}
}

// Upsert inserts or updates a push subscription keyed on (project_id, endpoint).
func (s *PushSubscriptionStore) Upsert(ctx context.Context, sub *domain.PushSubscription) error {
	id := sub.ID
	if id == "" {
		id = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions
		  (id, project_id, endpoint, p256dh_key, auth_key, vapid_public_key, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, endpoint) DO UPDATE
		  SET p256dh_key       = excluded.p256dh_key,
		      auth_key         = excluded.auth_key,
		      vapid_public_key = excluded.vapid_public_key,
		      user_agent       = excluded.user_agent
	`, id, sub.ProjectID, sub.Endpoint, sub.P256DHKey, sub.AuthKey, sub.VAPIDPublicKey, sub.UserAgent)
	if err != nil {
		return fmt.Errorf("push_subscription_store upsert: %w", err)
	}
	return nil
}

// ListByProject returns all push subscriptions for a project.
func (s *PushSubscriptionStore) ListByProject(ctx context.Context, projectID string) ([]*domain.PushSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, endpoint, p256dh_key, auth_key, vapid_public_key,
		       COALESCE(user_agent, ''), created_at
		FROM push_subscriptions
		WHERE project_id = ?
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
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM push_subscriptions WHERE project_id = ? AND endpoint = ?
	`, projectID, endpoint)
	if err != nil {
		return fmt.Errorf("push_subscription_store delete: %w", err)
	}
	return nil
}
