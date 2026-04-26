//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// EvalStore implements store.EvalStore against DuckDB. The team-mode equivalent
// uses ClickHouse `span_evals FINAL` (a ReplacingMergeTree). DuckDB has no
// equivalent, so we emulate "replace on duplicate" with `INSERT OR REPLACE`
// against the (project_id, run_id, span_id, eval_name, judge_model) PK
// declared in migrations/duckdb/0002_span_evals.sql.
//
// Run-id aggregations (BaselineByProject) read from `spans` directly because
// indie mode has no `run_metrics` MV.
type EvalStore struct {
	db *sql.DB
}

func NewEvalStore(db *sql.DB) *EvalStore { return &EvalStore{db: db} }

func (s *EvalStore) Insert(ctx context.Context, e *domain.SpanEval) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO span_evals
			(project_id, run_id, span_id, eval_name, judge_model,
			 score, reasoning, eval_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ProjectID, e.RunID, e.SpanID, e.EvalName, e.JudgeModel,
		e.Score, e.Reasoning, e.EvalVersion, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("eval_store insert: %w", err)
	}
	return nil
}

func (s *EvalStore) ListByRun(ctx context.Context, runID string) ([]*domain.SpanEval, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, created_at
		FROM span_evals
		WHERE run_id = ?
		ORDER BY span_id, eval_name`, runID)
	if err != nil {
		return nil, fmt.Errorf("eval_store list_by_run: %w", err)
	}
	defer rows.Close()

	var evals []*domain.SpanEval
	for rows.Next() {
		e, err := scanEval(rows)
		if err != nil {
			return nil, err
		}
		evals = append(evals, e)
	}
	return evals, rows.Err()
}

type evalGroupKey struct {
	spanID   string
	evalName string
}

func (s *EvalStore) ListByRunGrouped(ctx context.Context, runID string) ([]*domain.SpanEvalGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, created_at
		FROM span_evals
		WHERE run_id = ?
		ORDER BY span_id, eval_name, judge_model`, runID)
	if err != nil {
		return nil, fmt.Errorf("eval_store list_by_run_grouped: %w", err)
	}
	defer rows.Close()

	var order []evalGroupKey
	groups := make(map[evalGroupKey]*domain.SpanEvalGroup)

	for rows.Next() {
		e, err := scanEval(rows)
		if err != nil {
			return nil, fmt.Errorf("eval_store list_by_run_grouped scan: %w", err)
		}

		k := evalGroupKey{spanID: e.SpanID, evalName: e.EvalName}
		g, exists := groups[k]
		if !exists {
			g = &domain.SpanEvalGroup{SpanID: e.SpanID, EvalName: e.EvalName}
			groups[k] = g
			order = append(order, k)
		}
		g.Scores = append(g.Scores, domain.ModelScore{
			Model:     e.JudgeModel,
			Score:     e.Score,
			Reasoning: e.Reasoning,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eval_store list_by_run_grouped rows: %w", err)
	}

	result := make([]*domain.SpanEvalGroup, 0, len(order))
	for _, k := range order {
		g := groups[k]
		n := len(g.Scores)
		if n == 0 {
			result = append(result, g)
			continue
		}
		var sum, minS, maxS float32
		minS = g.Scores[0].Score
		maxS = g.Scores[0].Score
		for _, ms := range g.Scores {
			sum += ms.Score
			if ms.Score < minS {
				minS = ms.Score
			}
			if ms.Score > maxS {
				maxS = ms.Score
			}
		}
		consensus := sum / float32(n)
		g.ConsensusScore = &consensus
		if maxS-minS > 0.2 {
			g.Disagreement = true
		}
		result = append(result, g)
	}
	return result, nil
}

func (s *EvalStore) SummaryByProject(ctx context.Context, projectID string) ([]*domain.RunEvalSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, eval_name, avg(score) AS avg_score, count(*) AS span_count
		FROM span_evals
		WHERE project_id = ?
		GROUP BY run_id, eval_name
		ORDER BY run_id, eval_name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("eval_store summary_by_project: %w", err)
	}
	defer rows.Close()

	var summaries []*domain.RunEvalSummary
	for rows.Next() {
		sum := &domain.RunEvalSummary{}
		var avgScore float64
		var spanCount int64
		if err := rows.Scan(&sum.RunID, &sum.EvalName, &avgScore, &spanCount); err != nil {
			return nil, fmt.Errorf("eval_store summary scan: %w", err)
		}
		sum.AvgScore = float32(avgScore)
		sum.SpanCount = int(spanCount)
		summaries = append(summaries, sum)
	}
	return summaries, rows.Err()
}

func (s *EvalStore) BaselineByProject(ctx context.Context, projectID string, lastNRuns int) (*domain.EvalBaseline, error) {
	// Step 1: last N run IDs by min(start_time).
	runRows, err := s.db.QueryContext(ctx, `
		SELECT run_id FROM (
		    SELECT run_id, min(start_time) AS min_start
		    FROM spans
		    WHERE project_id = ?
		    GROUP BY run_id
		    ORDER BY min_start DESC
		    LIMIT ?
		)`, projectID, lastNRuns)
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

	baseline := &domain.EvalBaseline{ProjectID: projectID, RunsConsidered: len(runIDs)}
	if len(runIDs) == 0 {
		return baseline, nil
	}

	// Step 2: 2-level aggregation — per-span consensus, then per-eval avg.
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, 0, len(runIDs)+1)
	args = append(args, projectID)
	for _, id := range runIDs {
		args = append(args, id)
	}

	evalRows, err := s.db.QueryContext(ctx, `
		SELECT eval_name, avg(span_consensus) AS avg_score, count(*) AS span_count, count(DISTINCT run_id) AS run_count
		FROM (
		    SELECT span_id, eval_name, run_id, avg(score) AS span_consensus
		    FROM span_evals
		    WHERE project_id = ? AND run_id IN (`+placeholders+`)
		    GROUP BY span_id, eval_name, run_id
		)
		GROUP BY eval_name
		ORDER BY eval_name`, args...)
	if err != nil {
		return nil, fmt.Errorf("eval_store baseline evals: %w", err)
	}
	defer evalRows.Close()

	var totalScore float32
	for evalRows.Next() {
		t := domain.EvalTypeBaseline{}
		var avgScore float64
		var spanCount, runCount int64
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

	if len(baseline.Types) > 0 {
		baseline.OverallScore = totalScore / float32(len(baseline.Types))
	}
	return baseline, nil
}

func scanEval(rows *sql.Rows) (*domain.SpanEval, error) {
	e := &domain.SpanEval{}
	var createdAt time.Time
	if err := rows.Scan(
		&e.ProjectID, &e.RunID, &e.SpanID, &e.EvalName,
		&e.Score, &e.Reasoning, &e.JudgeModel, &e.EvalVersion, &createdAt,
	); err != nil {
		return nil, fmt.Errorf("eval_store scan: %w", err)
	}
	e.CreatedAt = createdAt.UTC()
	return e, nil
}
