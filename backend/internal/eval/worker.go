package eval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const workerInterval = 5 * time.Second

// Worker claims pending eval jobs and scores them via the LLM judge.
type Worker struct {
	chConn    driver.Conn
	jobStore  store.EvalJobStore
	evalStore store.EvalStore
	apiKey    string
}

func NewWorker(chConn driver.Conn, jobStore store.EvalJobStore, evalStore store.EvalStore, apiKey string) *Worker {
	return &Worker{
		chConn:    chConn,
		jobStore:  jobStore,
		evalStore: evalStore,
		apiKey:    apiKey,
	}
}

func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(workerInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.process(ctx)
		}
	}
}

func (w *Worker) process(ctx context.Context) {
	jobs, err := w.jobStore.Claim(ctx, 5)
	if err != nil {
		slog.Error("eval worker: claim jobs", "error", err)
		return
	}
	for _, job := range jobs {
		if err := w.score(ctx, job); err != nil {
			slog.Error("eval worker: score job", "span_id", job.SpanID, "error", err)
			_ = w.jobStore.MarkFailed(ctx, job.ID, err.Error())
		}
	}
}

func (w *Worker) score(ctx context.Context, job *domain.EvalJob) error {
	// Fetch span attributes from ClickHouse
	row := w.chConn.QueryRow(ctx, `
		SELECT
			project_id,
			attributes['gen_ai.prompt'],
			attributes['gen_ai.completion']
		FROM spans
		WHERE span_id = ?
		LIMIT 1
	`, job.SpanID)

	var projectID, prompt, completion string
	if err := row.Scan(&projectID, &prompt, &completion); err != nil {
		return fmt.Errorf("fetch span: %w", err)
	}

	if prompt == "" {
		// Nothing to evaluate
		return w.jobStore.MarkDone(ctx, job.ID)
	}

	// Call judge
	result, err := callJudge(ctx, w.apiKey, prompt, completion)
	if err != nil {
		return fmt.Errorf("judge call: %w", err)
	}

	// Write score
	eval := &domain.SpanEval{
		ProjectID:   job.ProjectID,
		RunID:       job.RunID,
		SpanID:      job.SpanID,
		EvalName:    job.EvalName,
		Score:       result.Score,
		Reasoning:   result.Reasoning,
		JudgeModel:  judgeModel,
		EvalVersion: evalVersion,
		CreatedAt:   time.Now().UTC(),
	}
	if err := w.evalStore.Insert(ctx, eval); err != nil {
		return fmt.Errorf("insert eval: %w", err)
	}

	slog.Info("eval worker: scored", "span_id", job.SpanID, "score", result.Score)
	return w.jobStore.MarkDone(ctx, job.ID)
}
