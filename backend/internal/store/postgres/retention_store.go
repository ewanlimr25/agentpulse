package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

const defaultRetentionDays = 30

// ── RetentionStore ────────────────────────────────────────────────────────────

// RetentionPGStore implements store.RetentionStore against Postgres.
type RetentionPGStore struct {
	pool *pgxpool.Pool
}

// NewRetentionStore returns a *RetentionPGStore.
func NewRetentionStore(pool *pgxpool.Pool) *RetentionPGStore {
	return &RetentionPGStore{pool: pool}
}

// Get returns the retention config for projectID.
// If no row exists, a default (30 days) is returned — never an error.
func (s *RetentionPGStore) Get(ctx context.Context, projectID string) (*domain.RetentionConfig, error) {
	cfg := &domain.RetentionConfig{
		ProjectID:     projectID,
		RetentionDays: defaultRetentionDays,
		UpdatedAt:     time.Now(),
	}

	err := s.pool.QueryRow(ctx, `
		SELECT retention_days, updated_at
		FROM project_retention_config
		WHERE project_id = $1
	`, projectID).Scan(&cfg.RetentionDays, &cfg.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cfg, nil
		}
		return nil, fmt.Errorf("retention_store get %s: %w", projectID, err)
	}
	return cfg, nil
}

// Upsert creates or updates the retention config for a project.
func (s *RetentionPGStore) Upsert(ctx context.Context, cfg *domain.RetentionConfig) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_retention_config (project_id, retention_days)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET
		  retention_days = EXCLUDED.retention_days,
		  updated_at     = now()
	`, cfg.ProjectID, cfg.RetentionDays)
	if err != nil {
		return fmt.Errorf("retention_store upsert %s: %w", cfg.ProjectID, err)
	}
	return nil
}

// ListAll returns retention configs for all projects that have an explicit config.
func (s *RetentionPGStore) ListAll(ctx context.Context) ([]*domain.RetentionConfig, error) {
	rows, err := s.pool.Query(ctx, `
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

// PurgeJobPGStore implements store.PurgeJobStore against Postgres.
type PurgeJobPGStore struct {
	pool *pgxpool.Pool
}

// NewPurgeJobStore returns a *PurgeJobPGStore.
func NewPurgeJobStore(pool *pgxpool.Pool) *PurgeJobPGStore {
	return &PurgeJobPGStore{pool: pool}
}

// Create inserts a new purge job record.
func (s *PurgeJobPGStore) Create(ctx context.Context, job *domain.PurgeJob) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO purge_jobs (project_id, run_id, cutoff_date, include_evals, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, 'pending')
		RETURNING id, started_at
	`, job.ProjectID, job.RunID, job.CutoffDate, job.IncludeEvals).Scan(&job.ID, &job.StartedAt)
	if err != nil {
		return fmt.Errorf("purge_job_store create: %w", err)
	}
	return nil
}

// Get returns the purge job with the given id.
func (s *PurgeJobPGStore) Get(ctx context.Context, id string) (*domain.PurgeJob, error) {
	job := &domain.PurgeJob{}
	var runID *string
	var errorMsg *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, run_id, cutoff_date, status, include_evals,
		       spans_deleted, s3_keys_deleted, pg_rows_deleted,
		       partial_failure, error_msg, started_at, completed_at
		FROM purge_jobs
		WHERE id = $1
	`, id).Scan(
		&job.ID, &job.ProjectID, &runID, &job.CutoffDate,
		&job.Status, &job.IncludeEvals,
		&job.SpansDeleted, &job.S3KeysDeleted, &job.PGRowsDeleted,
		&job.PartialFailure, &errorMsg,
		&job.StartedAt, &job.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("purge_job_store get %s: %w", id, pgx.ErrNoRows)
		}
		return nil, fmt.Errorf("purge_job_store get %s: %w", id, err)
	}
	if runID != nil {
		job.RunID = *runID
	}
	if errorMsg != nil {
		job.ErrorMsg = *errorMsg
	}
	return job, nil
}

// UpdateStatus sets the status field for the given job ID.
func (s *PurgeJobPGStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE purge_jobs SET status = $2 WHERE id = $1
	`, id, status)
	if err != nil {
		return fmt.Errorf("purge_job_store update_status %s: %w", id, err)
	}
	return nil
}

// Complete marks a purge job as completed (or failed) with final counts and metadata.
func (s *PurgeJobPGStore) Complete(ctx context.Context, id string, result *domain.PurgeJob) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE purge_jobs SET
		  status          = $2,
		  spans_deleted   = $3,
		  s3_keys_deleted = $4,
		  pg_rows_deleted = $5,
		  partial_failure = $6,
		  error_msg       = NULLIF($7, ''),
		  completed_at    = now()
		WHERE id = $1
	`, id, result.Status, result.SpansDeleted, result.S3KeysDeleted, result.PGRowsDeleted,
		result.PartialFailure, result.ErrorMsg)
	if err != nil {
		return fmt.Errorf("purge_job_store complete %s: %w", id, err)
	}
	return nil
}
