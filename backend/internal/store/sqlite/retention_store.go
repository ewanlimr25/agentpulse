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

const defaultRetentionDays = 30

// ── RetentionStore ────────────────────────────────────────────────────────────

// RetentionStore implements store.RetentionStore against SQLite.
type RetentionStore struct {
	db *sql.DB
}

// NewRetentionStore returns a *RetentionStore backed by db.
func NewRetentionStore(db *sql.DB) *RetentionStore {
	return &RetentionStore{db: db}
}

// Get returns the retention config for projectID.
// If no row exists, a default (30 days) is returned — never an error.
func (s *RetentionStore) Get(ctx context.Context, projectID string) (*domain.RetentionConfig, error) {
	cfg := &domain.RetentionConfig{
		ProjectID:     projectID,
		RetentionDays: defaultRetentionDays,
		UpdatedAt:     time.Now(),
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT retention_days, updated_at
		FROM project_retention_config
		WHERE project_id = ?
	`, projectID).Scan(&cfg.RetentionDays, &cfg.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cfg, nil
		}
		return nil, fmt.Errorf("retention_store get %s: %w", projectID, err)
	}
	return cfg, nil
}

// Upsert creates or updates the retention config for a project.
func (s *RetentionStore) Upsert(ctx context.Context, cfg *domain.RetentionConfig) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_retention_config (project_id, retention_days)
		VALUES (?, ?)
		ON CONFLICT (project_id) DO UPDATE SET
		  retention_days = excluded.retention_days,
		  updated_at     = strftime('%Y-%m-%dT%H:%M:%fZ','now')
	`, cfg.ProjectID, cfg.RetentionDays)
	if err != nil {
		return fmt.Errorf("retention_store upsert %s: %w", cfg.ProjectID, err)
	}
	return nil
}

// ListAll returns retention configs for all projects that have an explicit config.
func (s *RetentionStore) ListAll(ctx context.Context) ([]*domain.RetentionConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, retention_days, updated_at
		FROM project_retention_config
	`)
	if err != nil {
		return nil, fmt.Errorf("retention_store list_all: %w", err)
	}
	defer rows.Close()

	var out []*domain.RetentionConfig
	for rows.Next() {
		cfg := &domain.RetentionConfig{}
		if err := rows.Scan(&cfg.ProjectID, &cfg.RetentionDays, &cfg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("retention_store list_all scan: %w", err)
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// ── PurgeJobStore ─────────────────────────────────────────────────────────────

// PurgeJobStore implements store.PurgeJobStore against SQLite.
type PurgeJobStore struct {
	db *sql.DB
}

// NewPurgeJobStore returns a *PurgeJobStore backed by db.
func NewPurgeJobStore(db *sql.DB) *PurgeJobStore {
	return &PurgeJobStore{db: db}
}

// Create inserts a new purge job record. If job.ID is empty a new UUID is generated.
func (s *PurgeJobStore) Create(ctx context.Context, job *domain.PurgeJob) error {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	now := time.Now().UTC()

	// Mirror the Postgres NULLIF($2, '') behavior — store NULL when run_id is empty.
	var runID any
	if job.RunID != "" {
		runID = job.RunID
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO purge_jobs (id, project_id, run_id, cutoff_date, include_evals, status, started_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?)
	`, job.ID, job.ProjectID, runID, job.CutoffDate, job.IncludeEvals, now)
	if err != nil {
		return fmt.Errorf("purge_job_store create: %w", err)
	}
	job.StartedAt = now
	return nil
}

// Get returns the purge job with the given id.
func (s *PurgeJobStore) Get(ctx context.Context, id string) (*domain.PurgeJob, error) {
	job := &domain.PurgeJob{}
	var (
		runID    sql.NullString
		errorMsg sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, run_id, cutoff_date, status, include_evals,
		       spans_deleted, s3_keys_deleted, pg_rows_deleted,
		       partial_failure, error_msg, started_at, completed_at
		FROM purge_jobs
		WHERE id = ?
	`, id).Scan(
		&job.ID, &job.ProjectID, &runID, &job.CutoffDate,
		&job.Status, &job.IncludeEvals,
		&job.SpansDeleted, &job.S3KeysDeleted, &job.PGRowsDeleted,
		&job.PartialFailure, &errorMsg,
		&job.StartedAt, &job.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("purge_job_store get %s: %w", id, sql.ErrNoRows)
		}
		return nil, fmt.Errorf("purge_job_store get %s: %w", id, err)
	}
	if runID.Valid {
		job.RunID = runID.String
	}
	if errorMsg.Valid {
		job.ErrorMsg = errorMsg.String
	}
	return job, nil
}

// UpdateStatus sets the status field for the given job ID.
func (s *PurgeJobStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE purge_jobs SET status = ? WHERE id = ?
	`, status, id)
	if err != nil {
		return fmt.Errorf("purge_job_store update_status %s: %w", id, err)
	}
	return nil
}

// Complete marks a purge job as completed (or failed) with final counts and metadata.
func (s *PurgeJobStore) Complete(ctx context.Context, id string, result *domain.PurgeJob) error {
	// Mirror Postgres NULLIF behavior — NULL when error_msg is empty.
	var errMsg any
	if result.ErrorMsg != "" {
		errMsg = result.ErrorMsg
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE purge_jobs SET
		  status          = ?,
		  spans_deleted   = ?,
		  s3_keys_deleted = ?,
		  pg_rows_deleted = ?,
		  partial_failure = ?,
		  error_msg       = ?,
		  completed_at    = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?
	`, result.Status, result.SpansDeleted, result.S3KeysDeleted, result.PGRowsDeleted,
		result.PartialFailure, errMsg, id)
	if err != nil {
		return fmt.Errorf("purge_job_store complete %s: %w", id, err)
	}
	return nil
}
