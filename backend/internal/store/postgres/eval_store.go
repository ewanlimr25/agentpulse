package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

type EvalJobStore struct {
	pool *pgxpool.Pool
}

func NewEvalJobStore(pool *pgxpool.Pool) *EvalJobStore {
	return &EvalJobStore{pool: pool}
}

func (s *EvalJobStore) Enqueue(ctx context.Context, jobs []*domain.EvalJob) error {
	if len(jobs) == 0 {
		return nil
	}
	for _, j := range jobs {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO eval_jobs (span_id, run_id, project_id, eval_name)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (span_id, eval_name) DO NOTHING`,
			j.SpanID, j.RunID, j.ProjectID, j.EvalName,
		)
		if err != nil {
			return fmt.Errorf("eval_job_store enqueue: %w", err)
		}
	}
	return nil
}

func (s *EvalJobStore) Claim(ctx context.Context, n int) ([]*domain.EvalJob, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE eval_jobs
		SET status = 'running', attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM eval_jobs
			WHERE status IN ('pending', 'failed') AND attempts < 3
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, span_id, run_id, project_id, eval_name`,
		n,
	)
	if err != nil {
		return nil, fmt.Errorf("eval_job_store claim: %w", err)
	}
	defer rows.Close()

	var jobs []*domain.EvalJob
	for rows.Next() {
		j := &domain.EvalJob{}
		if err := rows.Scan(&j.ID, &j.SpanID, &j.RunID, &j.ProjectID, &j.EvalName); err != nil {
			return nil, fmt.Errorf("eval_job_store scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *EvalJobStore) MarkDone(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE eval_jobs SET status = 'done', updated_at = now() WHERE id = $1`, id)
	return err
}

func (s *EvalJobStore) MarkFailed(ctx context.Context, id, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE eval_jobs SET status = 'failed', error = $2, updated_at = now() WHERE id = $1`,
		id, errMsg)
	return err
}
