// Package storage contains cross-store operations for purging and stats.
package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// s3Store is a narrower interface so purge.go can call DeleteByKeys without
// importing the s3 package directly.
type s3Store interface {
	store.PayloadStore
	DeleteByKeys(ctx context.Context, keys []string) (int64, error)
}

// PurgeExecutor executes cross-store data purge jobs.
type PurgeExecutor struct {
	ch       driver.Conn
	pg       *pgxpool.Pool
	payloads store.PayloadStore
	jobs     store.PurgeJobStore
	runs     store.RunStore
	logger   *zap.Logger
}

// NewPurgeExecutor creates a PurgeExecutor.
func NewPurgeExecutor(
	ch driver.Conn,
	pg *pgxpool.Pool,
	payloads store.PayloadStore,
	jobs store.PurgeJobStore,
	runs store.RunStore,
	logger *zap.Logger,
) *PurgeExecutor {
	return &PurgeExecutor{
		ch:       ch,
		pg:       pg,
		payloads: payloads,
		jobs:     jobs,
		runs:     runs,
		logger:   logger,
	}
}

// ExecuteRunPurge purges all data for a specific run.
// It marks the job failed if the run is still active.
func (e *PurgeExecutor) ExecuteRunPurge(ctx context.Context, job *domain.PurgeJob) error {
	if err := e.jobs.UpdateStatus(ctx, job.ID, "running"); err != nil {
		return fmt.Errorf("purge executor: mark running: %w", err)
	}

	// Guard: refuse to purge an active run.
	active, err := e.runs.ListActiveRunIDs(ctx, job.ProjectID, 120)
	if err != nil {
		return e.failJob(ctx, job, fmt.Sprintf("list active runs: %v", err))
	}
	if active[job.RunID] {
		return e.failJob(ctx, job, "run is active, retry later")
	}

	result := &domain.PurgeJob{Status: "completed"}

	// Step 1: enumerate S3 keys for the run.
	s3Keys, err := e.listRunS3Keys(ctx, job.ProjectID, job.RunID)
	if err != nil {
		return e.failJob(ctx, job, fmt.Sprintf("list s3 keys: %v", err))
	}

	// Step 2: delete S3 objects.
	if len(s3Keys) > 0 {
		deleted, err := e.deleteS3Keys(ctx, s3Keys)
		result.S3KeysDeleted = deleted
		if err != nil {
			e.logger.Warn("purge run: s3 partial failure", zap.String("run_id", job.RunID), zap.Error(err))
			result.PartialFailure = true
			result.ErrorMsg = err.Error()
		}
	}

	// Step 3: count spans before deletion (CH mutations are async).
	spanCount, err := e.countRunSpans(ctx, job.ProjectID, job.RunID)
	if err != nil {
		e.logger.Warn("purge run: count spans", zap.String("run_id", job.RunID), zap.Error(err))
	}
	result.SpansDeleted = spanCount

	// Step 4: issue ClickHouse async mutations.
	if err := e.ch.AsyncInsert(ctx, fmt.Sprintf(
		"ALTER TABLE spans DELETE WHERE project_id = '%s' AND run_id = '%s'",
		job.ProjectID, job.RunID,
	), false); err != nil {
		slog.Warn("purge run: ch spans mutation", "run_id", job.RunID, "error", err)
	}
	if err := e.ch.AsyncInsert(ctx, fmt.Sprintf(
		"ALTER TABLE metrics_agg DELETE WHERE run_id = '%s'",
		job.RunID,
	), false); err != nil {
		slog.Warn("purge run: ch metrics_agg mutation", "run_id", job.RunID, "error", err)
	}
	if job.IncludeEvals {
		if err := e.ch.AsyncInsert(ctx, fmt.Sprintf(
			"ALTER TABLE span_evals DELETE WHERE run_id = '%s'",
			job.RunID,
		), false); err != nil {
			slog.Warn("purge run: ch span_evals mutation", "run_id", job.RunID, "error", err)
		}
	}

	// Step 5: delete Postgres rows.
	pgDeleted, pgErr := e.deleteRunPGRows(ctx, job.ProjectID, job.RunID)
	result.PGRowsDeleted = pgDeleted
	if pgErr != nil {
		result.PartialFailure = true
		if result.ErrorMsg != "" {
			result.ErrorMsg += "; " + pgErr.Error()
		} else {
			result.ErrorMsg = pgErr.Error()
		}
	}

	if result.PartialFailure {
		result.Status = "completed" // still mark completed with partial_failure flag
	}

	return e.jobs.Complete(ctx, job.ID, result)
}

// ExecuteAgePurge purges all data for a project older than the cutoff date.
func (e *PurgeExecutor) ExecuteAgePurge(ctx context.Context, job *domain.PurgeJob) error {
	if job.CutoffDate == nil {
		return e.failJob(ctx, job, "cutoff_date is required for age purge")
	}
	if err := e.jobs.UpdateStatus(ctx, job.ID, "running"); err != nil {
		return fmt.Errorf("purge executor: mark running: %w", err)
	}

	result := &domain.PurgeJob{Status: "completed"}

	// Step 1: enumerate S3 keys older than cutoff.
	s3Keys, err := e.listAgeS3Keys(ctx, job.ProjectID, *job.CutoffDate)
	if err != nil {
		return e.failJob(ctx, job, fmt.Sprintf("list s3 keys: %v", err))
	}

	// Step 2: delete S3 objects.
	if len(s3Keys) > 0 {
		deleted, err := e.deleteS3Keys(ctx, s3Keys)
		result.S3KeysDeleted = deleted
		if err != nil {
			e.logger.Warn("purge age: s3 partial failure", zap.String("project_id", job.ProjectID), zap.Error(err))
			result.PartialFailure = true
			result.ErrorMsg = err.Error()
		}
	}

	// Step 3: count spans before deletion.
	spanCount, err := e.countAgeSpans(ctx, job.ProjectID, *job.CutoffDate)
	if err != nil {
		e.logger.Warn("purge age: count spans", zap.String("project_id", job.ProjectID), zap.Error(err))
	}
	result.SpansDeleted = spanCount

	// Step 4: ClickHouse async mutations.
	cutoffStr := job.CutoffDate.Format("2006-01-02 15:04:05")
	if err := e.ch.AsyncInsert(ctx, fmt.Sprintf(
		"ALTER TABLE spans DELETE WHERE project_id = '%s' AND start_time < '%s' AND _date >= toDate('%s') - 1",
		job.ProjectID, cutoffStr, cutoffStr,
	), false); err != nil {
		slog.Warn("purge age: ch spans mutation", "project_id", job.ProjectID, "error", err)
	}
	if err := e.ch.AsyncInsert(ctx, fmt.Sprintf(
		"ALTER TABLE metrics_agg DELETE WHERE project_id = '%s' AND toDate(start_time) < toDate('%s')",
		job.ProjectID, cutoffStr,
	), false); err != nil {
		slog.Warn("purge age: ch metrics_agg mutation", "project_id", job.ProjectID, "error", err)
	}
	if job.IncludeEvals {
		if err := e.ch.AsyncInsert(ctx, fmt.Sprintf(
			"ALTER TABLE span_evals DELETE WHERE run_id IN (SELECT DISTINCT run_id FROM spans WHERE project_id = '%s' AND start_time < '%s')",
			job.ProjectID, cutoffStr,
		), false); err != nil {
			slog.Warn("purge age: ch span_evals mutation", "project_id", job.ProjectID, "error", err)
		}
	}

	// Step 5: delete Postgres rows older than cutoff.
	pgDeleted, pgErr := e.deleteAgePGRows(ctx, job.ProjectID, *job.CutoffDate)
	result.PGRowsDeleted = pgDeleted
	if pgErr != nil {
		result.PartialFailure = true
		if result.ErrorMsg != "" {
			result.ErrorMsg += "; " + pgErr.Error()
		} else {
			result.ErrorMsg = pgErr.Error()
		}
	}

	if result.PartialFailure {
		result.Status = "completed"
	}

	return e.jobs.Complete(ctx, job.ID, result)
}

// ── public helpers (for dry-run support in the API handler) ──────────────────

// CountRunSpans returns how many spans would be deleted for a run purge (dry run support).
func (e *PurgeExecutor) CountRunSpans(ctx context.Context, projectID, runID string) (int64, error) {
	return e.countRunSpans(ctx, projectID, runID)
}

// CountAgeSpans returns how many spans would be deleted for an age purge (dry run support).
func (e *PurgeExecutor) CountAgeSpans(ctx context.Context, projectID string, cutoff interface{}) (int64, error) {
	return e.countAgeSpans(ctx, projectID, cutoff)
}

// ── private helpers ───────────────────────────────────────────────────────────

func (e *PurgeExecutor) failJob(ctx context.Context, job *domain.PurgeJob, reason string) error {
	result := &domain.PurgeJob{Status: "failed", ErrorMsg: reason}
	if err := e.jobs.Complete(ctx, job.ID, result); err != nil {
		return fmt.Errorf("purge executor fail_job: %w", err)
	}
	return fmt.Errorf("purge executor: %s", reason)
}

func (e *PurgeExecutor) listRunS3Keys(ctx context.Context, projectID, runID string) ([]string, error) {
	rows, err := e.ch.Query(ctx,
		"SELECT payload_s3_key FROM spans WHERE project_id = ? AND run_id = ? AND payload_s3_key != ''",
		projectID, runID,
	)
	if err != nil {
		return nil, fmt.Errorf("list run s3 keys query: %w", err)
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func (e *PurgeExecutor) listAgeS3Keys(ctx context.Context, projectID string, cutoff interface{}) ([]string, error) {
	rows, err := e.ch.Query(ctx,
		"SELECT payload_s3_key FROM spans WHERE project_id = ? AND start_time < ? AND payload_s3_key != '' AND _date >= toDate(?) - 1",
		projectID, cutoff, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("list age s3 keys query: %w", err)
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

func (e *PurgeExecutor) countRunSpans(ctx context.Context, projectID, runID string) (int64, error) {
	var count uint64
	err := e.ch.QueryRow(ctx,
		"SELECT count() FROM spans WHERE project_id = ? AND run_id = ?",
		projectID, runID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count run spans: %w", err)
	}
	return int64(count), nil
}

func (e *PurgeExecutor) countAgeSpans(ctx context.Context, projectID string, cutoff interface{}) (int64, error) {
	var count uint64
	err := e.ch.QueryRow(ctx,
		"SELECT count() FROM spans WHERE project_id = ? AND start_time < ? AND _date >= toDate(?) - 1",
		projectID, cutoff, cutoff,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count age spans: %w", err)
	}
	return int64(count), nil
}

func (e *PurgeExecutor) deleteS3Keys(ctx context.Context, keys []string) (int64, error) {
	type batchDeleter interface {
		DeleteByKeys(ctx context.Context, keys []string) (int64, error)
	}
	if bd, ok := e.payloads.(batchDeleter); ok {
		return bd.DeleteByKeys(ctx, keys)
	}
	// Fallback: delete one by one.
	var count int64
	for _, k := range keys {
		if err := e.payloads.Delete(ctx, k); err != nil {
			return count, fmt.Errorf("delete s3 key %q: %w", k, err)
		}
		count++
	}
	return count, nil
}

func (e *PurgeExecutor) deleteRunPGRows(ctx context.Context, projectID, runID string) (int64, error) {
	var total int64
	tables := []struct {
		query string
		args  []interface{}
	}{
		{"DELETE FROM topology_nodes WHERE run_id = $1", []interface{}{runID}},
		{"DELETE FROM run_loops WHERE run_id = $1", []interface{}{runID}},
		{"DELETE FROM run_tags WHERE run_id = $1 AND project_id = $2", []interface{}{runID, projectID}},
		{"DELETE FROM run_annotations WHERE run_id = $1 AND project_id = $2", []interface{}{runID, projectID}},
		{"DELETE FROM span_feedback WHERE project_id = $1 AND span_id IN (SELECT span_id FROM span_feedback WHERE project_id = $1) AND run_id = $2", []interface{}{projectID, runID}},
	}

	var firstErr error
	for _, t := range tables {
		tag, err := e.pg.Exec(ctx, t.query, t.args...)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("delete pg rows (%s): %w", t.query[:40], err)
			}
			continue
		}
		total += tag.RowsAffected()
	}
	return total, firstErr
}

func (e *PurgeExecutor) deleteAgePGRows(ctx context.Context, projectID string, cutoff interface{}) (int64, error) {
	var total int64
	tag, err := e.pg.Exec(ctx,
		"DELETE FROM topology_nodes WHERE project_id = $1 AND start_time < $2",
		projectID, cutoff,
	)
	if err != nil {
		return total, fmt.Errorf("delete age pg topology_nodes: %w", err)
	}
	total += tag.RowsAffected()
	return total, nil
}

func scanStringColumn(rows driver.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan string column: %w", err)
		}
		out = append(out, key)
	}
	return out, rows.Err()
}
