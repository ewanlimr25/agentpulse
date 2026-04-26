package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// EvalJobStore implements store.EvalJobStore against SQLite.
type EvalJobStore struct {
	db *sql.DB
}

func NewEvalJobStore(db *sql.DB) *EvalJobStore {
	return &EvalJobStore{db: db}
}

const defaultEvalJobJudgeModel = "claude-haiku-4-5"

// Enqueue inserts pending jobs, ignoring duplicates on (span_id, eval_name, judge_model).
func (s *EvalJobStore) Enqueue(ctx context.Context, jobs []*domain.EvalJob) error {
	if len(jobs) == 0 {
		return nil
	}
	for _, j := range jobs {
		model := j.JudgeModel
		if model == "" {
			model = defaultEvalJobJudgeModel
		}
		id := j.ID
		if id == "" {
			id = uuid.NewString()
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO eval_jobs (id, span_id, run_id, project_id, eval_name, judge_model)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (span_id, eval_name, judge_model) DO NOTHING`,
			id, j.SpanID, j.RunID, j.ProjectID, j.EvalName, model,
		)
		if err != nil {
			return fmt.Errorf("eval_job_store enqueue: %w", err)
		}
	}
	return nil
}

// Claim atomically claims up to n pending/failed jobs.
//
// SQLite is single-writer so we don't need FOR UPDATE SKIP LOCKED. We wrap
// the SELECT → UPDATE → SELECT in a single transaction so the row state
// transition is atomic w.r.t. concurrent claims.
func (s *EvalJobStore) Claim(ctx context.Context, n int) ([]*domain.EvalJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("eval_job_store claim begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM eval_jobs
		WHERE status IN ('pending', 'failed') AND attempts < 3
		ORDER BY created_at ASC
		LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("eval_job_store claim select: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("eval_job_store claim scan_ids: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eval_job_store claim iter: %w", err)
	}
	if len(ids) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("eval_job_store claim commit: %w", err)
		}
		return nil, nil
	}

	// Build placeholder list "(?, ?, ?, ...)".
	placeholders := make([]byte, 0, len(ids)*2+1)
	placeholders = append(placeholders, '(')
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	placeholders = append(placeholders, ')')

	updateSQL := `UPDATE eval_jobs
		SET status = 'running', attempts = attempts + 1,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id IN ` + string(placeholders)
	if _, err := tx.ExecContext(ctx, updateSQL, args...); err != nil {
		return nil, fmt.Errorf("eval_job_store claim update: %w", err)
	}

	selectSQL := `SELECT id, span_id, run_id, project_id, eval_name, judge_model
		FROM eval_jobs WHERE id IN ` + string(placeholders) + ` ORDER BY created_at ASC`
	rows2, err := tx.QueryContext(ctx, selectSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("eval_job_store claim reselect: %w", err)
	}
	defer rows2.Close()

	var jobs []*domain.EvalJob
	for rows2.Next() {
		j := &domain.EvalJob{}
		if err := rows2.Scan(&j.ID, &j.SpanID, &j.RunID, &j.ProjectID, &j.EvalName, &j.JudgeModel); err != nil {
			return nil, fmt.Errorf("eval_job_store claim scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows2.Err(); err != nil {
		return nil, fmt.Errorf("eval_job_store claim iter2: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("eval_job_store claim commit: %w", err)
	}
	return jobs, nil
}

// MarkDone marks a job successfully completed.
func (s *EvalJobStore) MarkDone(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE eval_jobs SET status = 'done',
		   updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("eval_job_store mark_done: %w", err)
	}
	return nil
}

// MarkFailed marks a job failed with an error message.
func (s *EvalJobStore) MarkFailed(ctx context.Context, id, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE eval_jobs SET status = 'failed', error = ?,
		   updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ?`, errMsg, id)
	if err != nil {
		return fmt.Errorf("eval_job_store mark_failed: %w", err)
	}
	return nil
}
