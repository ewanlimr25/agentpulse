package eval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	evaltypes "github.com/agentpulse/agentpulse/backend/internal/eval/types"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const workerInterval = 5 * time.Second

// Worker claims pending eval jobs and scores them via the LLM judge.
// It is eval-type-agnostic: it delegates prompt building to the Registry.
type Worker struct {
	chConn    driver.Conn
	jobStore  store.EvalJobStore
	evalStore store.EvalStore
	registry  evaltypes.Registry
	apiKey    string
}

func NewWorker(chConn driver.Conn, jobStore store.EvalJobStore, evalStore store.EvalStore, registry evaltypes.Registry, apiKey string) *Worker {
	return &Worker{
		chConn:    chConn,
		jobStore:  jobStore,
		evalStore: evalStore,
		registry:  registry,
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
			slog.Error("eval worker: score job", "span_id", job.SpanID, "eval_name", job.EvalName, "error", err)
			_ = w.jobStore.MarkFailed(ctx, job.ID, err.Error())
		}
	}
}

func (w *Worker) score(ctx context.Context, job *domain.EvalJob) error {
	// Look up the eval type in the registry.
	evalType, ok := w.registry[job.EvalName]
	if !ok {
		// Unknown eval type — skip gracefully.
		slog.Warn("eval worker: unknown eval type, skipping", "eval_name", job.EvalName, "span_id", job.SpanID)
		return w.jobStore.MarkDone(ctx, job.ID)
	}

	// Fetch span attributes from ClickHouse.
	row := w.chConn.QueryRow(ctx, `
		SELECT
			project_id,
			attributes['gen_ai.prompt'],
			attributes['gen_ai.completion'],
			attributes['gen_ai.context'],
			attributes['gen_ai.request.model'],
			attributes['agent.name'],
			attributes['tool.name']
		FROM spans
		WHERE span_id = ?
		LIMIT 1
	`, job.SpanID)

	var projectID, prompt, completion, genAIContext, model, agentName, toolName string
	if err := row.Scan(&projectID, &prompt, &completion, &genAIContext, &model, &agentName, &toolName); err != nil {
		return fmt.Errorf("fetch span: %w", err)
	}

	if prompt == "" && completion == "" {
		return w.jobStore.MarkDone(ctx, job.ID)
	}

	spanCtx := evaltypes.SpanContext{
		Input:     prompt,
		Output:    completion,
		Context:   genAIContext,
		Model:     model,
		AgentName: agentName,
		ToolName:  toolName,
	}

	builtPrompt := evalType.BuildPrompt(spanCtx)

	result, err := callJudge(ctx, w.apiKey, builtPrompt)
	if err != nil {
		return fmt.Errorf("judge call: %w", err)
	}

	e := &domain.SpanEval{
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
	if err := w.evalStore.Insert(ctx, e); err != nil {
		return fmt.Errorf("insert eval: %w", err)
	}

	slog.Info("eval worker: scored", "span_id", job.SpanID, "eval_name", job.EvalName, "score", result.Score)
	return w.jobStore.MarkDone(ctx, job.ID)
}
