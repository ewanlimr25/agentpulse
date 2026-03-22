package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

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

func (s *EvalStore) SummaryByProject(ctx context.Context, projectID string) ([]*domain.RunEvalSummary, error) {
	rows, err := s.conn.Query(ctx, summaryByProjectQuery, projectID)
	if err != nil {
		return nil, fmt.Errorf("eval_store summary_by_project: %w", err)
	}
	defer rows.Close()

	var summaries []*domain.RunEvalSummary
	for rows.Next() {
		s := &domain.RunEvalSummary{}
		if err := rows.Scan(&s.RunID, &s.EvalName, &s.AvgScore, &s.SpanCount); err != nil {
			return nil, fmt.Errorf("eval_store summary scan: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}
