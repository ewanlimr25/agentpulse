package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// EmailDigestStore implements store.EmailDigestStore against SQLite.
type EmailDigestStore struct {
	db *sql.DB
}

// NewEmailDigestStore returns a new EmailDigestStore backed by db.
func NewEmailDigestStore(db *sql.DB) *EmailDigestStore {
	return &EmailDigestStore{db: db}
}

// Get returns the email digest config for a project, or nil if none exists.
func (s *EmailDigestStore) Get(ctx context.Context, projectID string) (*domain.EmailDigestConfig, error) {
	cfg := &domain.EmailDigestConfig{}
	var (
		lastSent  sql.NullTime
		lastError sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, enabled, recipient_email, schedule,
		       last_sent_at, last_error, created_at, updated_at
		FROM email_digest_configs
		WHERE project_id = ?
	`, projectID).Scan(
		&cfg.ID, &cfg.ProjectID, &cfg.Enabled, &cfg.RecipientEmail, &cfg.Schedule,
		&lastSent, &lastError, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("email_digest_store get: %w", err)
	}
	if lastSent.Valid {
		t := lastSent.Time
		cfg.LastSentAt = &t
	}
	if lastError.Valid {
		s := lastError.String
		cfg.LastError = &s
	}
	return cfg, nil
}

// Upsert creates or updates an email digest config keyed on project_id.
func (s *EmailDigestStore) Upsert(ctx context.Context, cfg *domain.EmailDigestConfig) error {
	id := cfg.ID
	if id == "" {
		id = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO email_digest_configs
		  (id, project_id, enabled, recipient_email, schedule)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (project_id) DO UPDATE
		  SET enabled         = excluded.enabled,
		      recipient_email = excluded.recipient_email,
		      schedule        = excluded.schedule,
		      updated_at      = strftime('%Y-%m-%dT%H:%M:%fZ','now')
	`, id, cfg.ProjectID, cfg.Enabled, cfg.RecipientEmail, cfg.Schedule)
	if err != nil {
		return fmt.Errorf("email_digest_store upsert: %w", err)
	}
	return nil
}

// ListDue returns configs that are due for digest delivery.
//
// The Postgres version uses SELECT ... FOR UPDATE SKIP LOCKED for multi-replica
// safety; SQLite is single-writer so plain SELECT is sufficient (the writer
// serialization replaces the explicit lock semantics).
//
// Interval arithmetic ("now() - interval '24 hours'") is computed in Go and
// passed as a parameter, which is cleaner than embedding SQLite datetime modifiers.
func (s *EmailDigestStore) ListDue(ctx context.Context) ([]*domain.EmailDigestConfig, error) {
	now := time.Now().UTC()
	dailyCutoff := now.Add(-24 * time.Hour)
	hourlyCutoff := now.Add(-1 * time.Hour)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, enabled, recipient_email, schedule,
		       last_sent_at, last_error, created_at, updated_at
		FROM email_digest_configs
		WHERE enabled = 1
		  AND recipient_email != ''
		  AND (
		    last_sent_at IS NULL
		    OR (schedule = 'daily'  AND last_sent_at < ?)
		    OR (schedule = 'hourly' AND last_sent_at < ?)
		  )
	`, dailyCutoff, hourlyCutoff)
	if err != nil {
		return nil, fmt.Errorf("email_digest_store list_due: %w", err)
	}
	defer rows.Close()

	var out []*domain.EmailDigestConfig
	for rows.Next() {
		cfg := &domain.EmailDigestConfig{}
		var (
			lastSent  sql.NullTime
			lastError sql.NullString
		)
		if err := rows.Scan(
			&cfg.ID, &cfg.ProjectID, &cfg.Enabled, &cfg.RecipientEmail, &cfg.Schedule,
			&lastSent, &lastError, &cfg.CreatedAt, &cfg.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("email_digest_store list_due scan: %w", err)
		}
		if lastSent.Valid {
			t := lastSent.Time
			cfg.LastSentAt = &t
		}
		if lastError.Valid {
			s := lastError.String
			cfg.LastError = &s
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// UpdateLastSent sets last_sent_at to now() for a project's digest config.
func (s *EmailDigestStore) UpdateLastSent(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE email_digest_configs
		SET last_sent_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
		    last_error   = NULL,
		    updated_at   = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE project_id = ?
	`, projectID)
	if err != nil {
		return fmt.Errorf("email_digest_store update_last_sent: %w", err)
	}
	return nil
}

// UpdateLastError records a delivery failure for a project's digest config.
func (s *EmailDigestStore) UpdateLastError(ctx context.Context, projectID, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE email_digest_configs
		SET last_error = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE project_id = ?
	`, errMsg, projectID)
	if err != nil {
		return fmt.Errorf("email_digest_store update_last_error: %w", err)
	}
	return nil
}
