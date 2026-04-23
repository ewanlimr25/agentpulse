// Package postgres contains Postgres-backed store implementations.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// EmailDigestStore implements store.EmailDigestStore against Postgres.
type EmailDigestStore struct {
	pool *pgxpool.Pool
}

// NewEmailDigestStore returns a new EmailDigestStore backed by pool.
func NewEmailDigestStore(pool *pgxpool.Pool) *EmailDigestStore {
	return &EmailDigestStore{pool: pool}
}

// Get returns the email digest config for a project, or nil if none exists.
func (s *EmailDigestStore) Get(ctx context.Context, projectID string) (*domain.EmailDigestConfig, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, enabled, recipient_email, schedule,
		       last_sent_at, last_error, created_at, updated_at
		FROM email_digest_configs
		WHERE project_id = $1
	`, projectID)

	cfg := &domain.EmailDigestConfig{}
	err := row.Scan(
		&cfg.ID, &cfg.ProjectID, &cfg.Enabled, &cfg.RecipientEmail, &cfg.Schedule,
		&cfg.LastSentAt, &cfg.LastError, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("email_digest_store get: %w", err)
	}
	return cfg, nil
}

// Upsert creates or updates an email digest config keyed on project_id.
func (s *EmailDigestStore) Upsert(ctx context.Context, cfg *domain.EmailDigestConfig) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO email_digest_configs
		  (project_id, enabled, recipient_email, schedule)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (project_id) DO UPDATE
		  SET enabled         = EXCLUDED.enabled,
		      recipient_email = EXCLUDED.recipient_email,
		      schedule        = EXCLUDED.schedule,
		      updated_at      = now()
	`, cfg.ProjectID, cfg.Enabled, cfg.RecipientEmail, cfg.Schedule)
	if err != nil {
		return fmt.Errorf("email_digest_store upsert: %w", err)
	}
	return nil
}

// ListDue returns configs that are due for digest delivery.
// Uses SELECT FOR UPDATE SKIP LOCKED for multi-replica safety.
func (s *EmailDigestStore) ListDue(ctx context.Context) ([]*domain.EmailDigestConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, enabled, recipient_email, schedule,
		       last_sent_at, last_error, created_at, updated_at
		FROM email_digest_configs
		WHERE enabled = true
		  AND recipient_email != ''
		  AND (
		    last_sent_at IS NULL
		    OR (schedule = 'daily'  AND last_sent_at < now() - interval '24 hours')
		    OR (schedule = 'hourly' AND last_sent_at < now() - interval '1 hour')
		  )
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		return nil, fmt.Errorf("email_digest_store list_due: %w", err)
	}
	defer rows.Close()

	var out []*domain.EmailDigestConfig
	for rows.Next() {
		cfg := &domain.EmailDigestConfig{}
		if err := rows.Scan(
			&cfg.ID, &cfg.ProjectID, &cfg.Enabled, &cfg.RecipientEmail, &cfg.Schedule,
			&cfg.LastSentAt, &cfg.LastError, &cfg.CreatedAt, &cfg.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("email_digest_store list_due scan: %w", err)
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// UpdateLastSent sets last_sent_at to now() for a project's digest config.
func (s *EmailDigestStore) UpdateLastSent(ctx context.Context, projectID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE email_digest_configs
		SET last_sent_at = now(), last_error = NULL, updated_at = now()
		WHERE project_id = $1
	`, projectID)
	if err != nil {
		return fmt.Errorf("email_digest_store update_last_sent: %w", err)
	}
	return nil
}

// UpdateLastError records a delivery failure for a project's digest config.
func (s *EmailDigestStore) UpdateLastError(ctx context.Context, projectID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE email_digest_configs
		SET last_error = $2, updated_at = now()
		WHERE project_id = $1
	`, projectID, errMsg)
	if err != nil {
		return fmt.Errorf("email_digest_store update_last_error: %w", err)
	}
	return nil
}
