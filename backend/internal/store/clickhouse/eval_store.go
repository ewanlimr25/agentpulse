package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// baselineRunIDsQuery fetches the last N run_ids for a project from run_metrics,
// ordered by run start time descending. Using run_metrics (not span_evals) keeps
// this query index-aligned on (project_id) and avoids a full span_evals scan.
const baselineRunIDsQuery = `
SELECT run_id
FROM run_metrics
WHERE project_id = ?
ORDER BY min_start DESC
LIMIT ?
`

// baselineEvalsQuery aggregates per-eval-type scores for a specific set of run IDs.
// FINAL ensures dedup on the ReplacingMergeTree. project_id filter leverages the
// sort-key prefix for efficient pruning before the IN clause scan.
const baselineEvalsQuery = `
SELECT eval_name, avg(score) AS avg_score, count() AS span_count, countDistinct(run_id) AS run_count
FROM span_evals FINAL
WHERE project_id = ? AND run_id IN (?)
GROUP BY eval_name
ORDER BY eval_name
`

type EvalStore struct {
	conn driver.Conn
}

func NewEvalStore(conn driver.Conn) *EvalStore {
	return &EvalStore{conn: conn}
}

func (s *EvalStore) Insert(ctx context.Context, e *domain.SpanEval) error {
	return s.conn.Exec(ctx, `
		INSERT INTO span_evals
			(project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ProjectID, e.RunID, e.SpanID, e.EvalName,
		e.Score, e.Reasoning, e.JudgeModel, e.EvalVersion, e.CreatedAt,
	)
}

const listEvalsByRunQuery = `
SELECT project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, created_at
FROM span_evals FINAL
WHERE run_id = ?
ORDER BY span_id, eval_name
`

func (s *EvalStore) ListByRun(ctx context.Context, runID string) ([]*domain.SpanEval, error) {
	rows, err := s.conn.Query(ctx, listEvalsByRunQuery, runID)
	if err != nil {
		return nil, fmt.Errorf("eval_store list_by_run: %w", err)
	}
	defer rows.Close()

	var evals []*domain.SpanEval
	for rows.Next() {
		e := &domain.SpanEval{}
		var createdAt time.Time
		if err := rows.Scan(
			&e.ProjectID, &e.RunID, &e.SpanID, &e.EvalName,
			&e.Score, &e.Reasoning, &e.JudgeModel, &e.EvalVersion, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("eval_store scan: %w", err)
		}
		e.CreatedAt = createdAt.UTC()
		evals = append(evals, e)
	}
	return evals, rows.Err()
}

const summaryByProjectQuery = `
SELECT run_id, eval_name, avg(score) AS avg_score, count() AS span_count
FROM span_evals FINAL
WHERE project_id = ?
GROUP BY run_id, eval_name
ORDER BY run_id, eval_name
`

// BaselineByProject returns per-eval-type avg scores across the last N runs.
// It uses a two-query strategy: first fetch run IDs from run_metrics (index-aligned),
// then aggregate eval scores for only those runs.
func (s *EvalStore) BaselineByProject(ctx context.Context, projectID string, lastNRuns int) (*domain.EvalBaseline, error) {
	// Step 1: Get last N run IDs from run_metrics.
	runRows, err := s.conn.Query(ctx, baselineRunIDsQuery, projectID, lastNRuns)
	if err != nil {
		return nil, fmt.Errorf("eval_store baseline run_ids: %w", err)
	}
	var runIDs []string
	for runRows.Next() {
		var rid string
		if err := runRows.Scan(&rid); err != nil {
			runRows.Close()
			return nil, fmt.Errorf("eval_store baseline run_ids scan: %w", err)
		}
		runIDs = append(runIDs, rid)
	}
	runRows.Close()
	if err := runRows.Err(); err != nil {
		return nil, fmt.Errorf("eval_store baseline run_ids rows: %w", err)
	}

	baseline := &domain.EvalBaseline{
		ProjectID:      projectID,
		RunsConsidered: len(runIDs),
	}

	// No runs → return empty baseline (not an error).
	if len(runIDs) == 0 {
		return baseline, nil
	}

	// Step 2: Aggregate eval scores for those run IDs.
	evalRows, err := s.conn.Query(ctx, baselineEvalsQuery, projectID, runIDs)
	if err != nil {
		return nil, fmt.Errorf("eval_store baseline evals: %w", err)
	}
	defer evalRows.Close()

	var totalScore float32
	for evalRows.Next() {
		t := domain.EvalTypeBaseline{}
		var avgScore float64
		var spanCount, runCount uint64
		if err := evalRows.Scan(&t.EvalName, &avgScore, &spanCount, &runCount); err != nil {
			return nil, fmt.Errorf("eval_store baseline evals scan: %w", err)
		}
		t.AvgScore = float32(avgScore)
		t.SpanCount = int(spanCount)
		t.RunCount = int(runCount)
		baseline.Types = append(baseline.Types, t)
		totalScore += t.AvgScore
	}
	if err := evalRows.Err(); err != nil {
		return nil, fmt.Errorf("eval_store baseline evals rows: %w", err)
	}

	// Unweighted average across types — for informational display only.
	if len(baseline.Types) > 0 {
		baseline.OverallScore = totalScore / float32(len(baseline.Types))
	}

	return baseline, nil
}

func (s *EvalStore) SummaryByProject(ctx context.Context, projectID string) ([]*domain.RunEvalSummary, error) {
	rows, err := s.conn.Query(ctx, summaryByProjectQuery, projectID)
	if err != nil {
		return nil, fmt.Errorf("eval_store summary_by_project: %w", err)
	}
	defer rows.Close()

	var summaries []*domain.RunEvalSummary
	for rows.Next() {
		s := &domain.RunEvalSummary{}
		var avgScore float64
		var spanCount uint64
		if err := rows.Scan(&s.RunID, &s.EvalName, &avgScore, &spanCount); err != nil {
			return nil, fmt.Errorf("eval_store summary scan: %w", err)
		}
		s.AvgScore = float32(avgScore)
		s.SpanCount = int(spanCount)
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}
