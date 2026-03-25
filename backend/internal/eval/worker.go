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

const (
	workerInterval = 5 * time.Second
	registryTTL    = 60 * time.Second
)

// Worker claims pending eval jobs and scores them via the LLM judge.
// It is eval-type-agnostic: it delegates prompt building to the Registry.
// The registry is reloaded from Postgres every registryTTL so that custom
// evals created via the API are picked up without a restart.
type Worker struct {
	chConn         driver.Conn
	jobStore       store.EvalJobStore
	evalStore      store.EvalStore
	configStore    store.EvalConfigStore
	registry       evaltypes.Registry
	promptVersions map[string]int // evalName → prompt_version (custom evals only)
	registryAt     time.Time
	apiKey         string
}

func NewWorker(chConn driver.Conn, jobStore store.EvalJobStore, evalStore store.EvalStore, configStore store.EvalConfigStore, apiKey string) *Worker {
	return &Worker{
		chConn:      chConn,
		jobStore:    jobStore,
		evalStore:   evalStore,
		configStore: configStore,
		registry:    NewRegistry(nil), // built-ins only until first refresh
		apiKey:      apiKey,
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
	// Reload registry when TTL expires so newly created custom evals are picked up.
	if time.Since(w.registryAt) > registryTTL {
		configs, err := w.configStore.ListAllEnabled(ctx)
		if err != nil {
			slog.Warn("eval worker: registry reload failed, using cached registry", "error", err)
		} else {
			w.registry = NewRegistry(configs)
			w.promptVersions = buildPromptVersionMap(configs)
			w.registryAt = time.Now()
			slog.Debug("eval worker: registry reloaded", "types", len(w.registry))
		}
	}

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

// buildPromptVersionMap maps evalName → prompt_version for custom evals.
// Built-in types always use the default evalVersion constant and are not included.
func buildPromptVersionMap(configs []*domain.EvalConfig) map[string]int {
	m := make(map[string]int, len(configs))
	for _, cfg := range configs {
		if cfg.PromptTemplate != nil && *cfg.PromptTemplate != "" {
			m[cfg.EvalName] = cfg.PromptVersion
		}
	}
	return m
}

func (w *Worker) score(ctx context.Context, job *domain.EvalJob) error {
	// Look up the eval type in the registry.
	evalType, ok := w.registry[job.EvalName]
	if !ok {
		// Unknown eval type — skip gracefully (may appear until next registry reload).
		slog.Warn("eval worker: unknown eval type, skipping", "eval_name", job.EvalName, "span_id", job.SpanID)
		return w.jobStore.MarkDone(ctx, job.ID)
	}

	// Fetch span attributes from ClickHouse, including tool-specific fields.
	row := w.chConn.QueryRow(ctx, `
		SELECT
			project_id,
			agent_span_kind,
			attributes['gen_ai.prompt'],
			attributes['gen_ai.completion'],
			attributes['gen_ai.context'],
			attributes['gen_ai.request.model'],
			attributes['agent.name'],
			attributes['tool.name'],
			attributes['tool.input'],
			attributes['tool.output']
		FROM spans
		WHERE span_id = ?
		LIMIT 1
	`, job.SpanID)

	var projectID, spanKind, prompt, completion, genAIContext, model, agentName, toolName, toolInput, toolOutput string
	if err := row.Scan(&projectID, &spanKind, &prompt, &completion, &genAIContext, &model, &agentName, &toolName, &toolInput, &toolOutput); err != nil {
		return fmt.Errorf("fetch span: %w", err)
	}

	// Map to SpanContext based on span kind.
	// tool.call spans store their content in tool.input / tool.output.
	input, output := prompt, completion
	if spanKind == "tool.call" && (toolInput != "" || toolOutput != "") {
		input, output = toolInput, toolOutput
	}

	// Skip spans with no scorable content.
	if input == "" && output == "" {
		return w.jobStore.MarkDone(ctx, job.ID)
	}

	spanCtx := evaltypes.SpanContext{
		Input:     input,
		Output:    output,
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

	// Use config's prompt_version for custom evals; fall back to evalVersion for built-ins.
	version := evalVersion
	if v, ok := w.promptVersions[job.EvalName]; ok && v > 0 {
		version = uint16(v)
	}

	e := &domain.SpanEval{
		ProjectID:   job.ProjectID,
		RunID:       job.RunID,
		SpanID:      job.SpanID,
		EvalName:    job.EvalName,
		Score:       result.Score,
		Reasoning:   result.Reasoning,
		JudgeModel:  judgeModel,
		EvalVersion: version,
		CreatedAt:   time.Now().UTC(),
	}
	if err := w.evalStore.Insert(ctx, e); err != nil {
		return fmt.Errorf("insert eval: %w", err)
	}

	slog.Info("eval worker: scored", "span_id", job.SpanID, "eval_name", job.EvalName, "score", result.Score)
	return w.jobStore.MarkDone(ctx, job.ID)
}
