package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// scenario is a function that emits a complete multi-agent run.
type scenario func(ctx context.Context, tracer trace.Tracer, projectID string) error

var scenarios = map[string]scenario{
	"multi-agent-research": scenarioMultiAgentResearch,
	"simple-llm":          scenarioSimpleLLM,
	"parallel-tools":      scenarioParallelTools,
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func llmAttrs(model string, inputTokens, outputTokens int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("gen_ai.operation.name", "chat"),
		attribute.String("gen_ai.system", "openai"),
		attribute.String("gen_ai.request.model", model),
		attribute.Int("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int("gen_ai.usage.output_tokens", outputTokens),
		attribute.String("agentpulse.span.kind", "llm.call"),
	}
}

func toolAttrs(toolName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "tool.call"),
		attribute.String("agentpulse.tool.name", toolName),
	}
}

func handoffAttrs(fromAgent, toAgent string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "agent.handoff"),
		attribute.String("agentpulse.agent.name", fromAgent),
		attribute.String("agentpulse.handoff.target", toAgent),
	}
}

func memReadAttrs(key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "memory.read"),
		attribute.String("agentpulse.memory.key", key),
	}
}

func memWriteAttrs(key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "memory.write"),
		attribute.String("agentpulse.memory.key", key),
	}
}

func jitter(base time.Duration) time.Duration {
	return base + time.Duration(rand.Int63n(int64(base/4)))
}

func sleep(d time.Duration) {
	time.Sleep(d)
}

// ── Scenario: multi-agent-research ──────────────────────────────────────────
//
// Orchestrator
//   ├─ memory.read (context)
//   ├─ llm.call   (plan)
//   ├─ agent.handoff → researcher
//   │     ├─ tool.call (web_search)
//   │     ├─ llm.call  (summarize)
//   │     └─ memory.write (summary)
//   ├─ agent.handoff → critic
//   │     ├─ memory.read (summary)
//   │     └─ llm.call (critique)
//   └─ llm.call   (final answer)

func scenarioMultiAgentResearch(ctx context.Context, tracer trace.Tracer, projectID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())

	rootCtx, rootSpan := tracer.Start(ctx, "orchestrator",
		trace.WithAttributes(
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "orchestrator"),
			attribute.String("agentpulse.span.kind", "llm.call"),
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", "openai"),
			attribute.String("gen_ai.request.model", "gpt-4o"),
			attribute.Int("gen_ai.usage.input_tokens", 512),
			attribute.Int("gen_ai.usage.output_tokens", 128),
		),
	)

	// memory.read — load context
	_, memReadSpan := tracer.Start(rootCtx, "memory.read/context",
		trace.WithAttributes(append(memReadAttrs("research_context"),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "orchestrator"),
		)...),
	)
	sleep(jitter(20 * time.Millisecond))
	memReadSpan.End()

	// llm.call — orchestrator plans
	_, planSpan := tracer.Start(rootCtx, "orchestrator/plan",
		trace.WithAttributes(append(llmAttrs("gpt-4o", 1024, 256),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "orchestrator"),
		)...),
	)
	sleep(jitter(200 * time.Millisecond))
	planSpan.End()

	// agent.handoff → researcher
	researchCtx, handoffSpan := tracer.Start(rootCtx, "handoff/researcher",
		trace.WithAttributes(append(handoffAttrs("orchestrator", "researcher"),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
		)...),
	)

	// researcher: tool.call web_search
	_, searchSpan := tracer.Start(researchCtx, "tool/web_search",
		trace.WithAttributes(append(toolAttrs("web_search"),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "researcher"),
		)...),
	)
	sleep(jitter(150 * time.Millisecond))
	searchSpan.End()

	// researcher: llm.call summarize
	_, summarizeSpan := tracer.Start(researchCtx, "researcher/summarize",
		trace.WithAttributes(append(llmAttrs("gpt-4o-mini", 2048, 512),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "researcher"),
		)...),
	)
	sleep(jitter(400 * time.Millisecond))
	summarizeSpan.End()

	// researcher: memory.write summary
	_, memWriteSpan := tracer.Start(researchCtx, "memory.write/summary",
		trace.WithAttributes(append(memWriteAttrs("research_summary"),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "researcher"),
		)...),
	)
	sleep(jitter(10 * time.Millisecond))
	memWriteSpan.End()

	handoffSpan.End()

	// agent.handoff → critic
	criticCtx, criticHandoffSpan := tracer.Start(rootCtx, "handoff/critic",
		trace.WithAttributes(append(handoffAttrs("orchestrator", "critic"),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
		)...),
	)

	// critic: memory.read summary
	_, criticReadSpan := tracer.Start(criticCtx, "memory.read/summary",
		trace.WithAttributes(append(memReadAttrs("research_summary"),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "critic"),
		)...),
	)
	sleep(jitter(15 * time.Millisecond))
	criticReadSpan.End()

	// critic: llm.call critique
	_, critiqueSpan := tracer.Start(criticCtx, "critic/critique",
		trace.WithAttributes(append(llmAttrs("gpt-4o", 3000, 400),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "critic"),
		)...),
	)
	sleep(jitter(350 * time.Millisecond))
	critiqueSpan.End()

	criticHandoffSpan.End()

	// orchestrator: final llm.call
	_, finalSpan := tracer.Start(rootCtx, "orchestrator/final-answer",
		trace.WithAttributes(append(llmAttrs("gpt-4o", 4000, 800),
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "orchestrator"),
		)...),
	)
	sleep(jitter(500 * time.Millisecond))
	finalSpan.End()

	rootSpan.SetStatus(codes.Ok, "")
	rootSpan.End()
	return nil
}

// ── Scenario: simple-llm ────────────────────────────────────────────────────
//
// Single agent, one LLM call with a tool lookup.

func scenarioSimpleLLM(ctx context.Context, tracer trace.Tracer, projectID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	attrs := func(extra ...attribute.KeyValue) []attribute.KeyValue {
		base := []attribute.KeyValue{
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", "assistant"),
		}
		return append(base, extra...)
	}

	rootCtx, rootSpan := tracer.Start(ctx, "assistant",
		trace.WithAttributes(attrs(
			attribute.String("agentpulse.span.kind", "llm.call"),
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", "anthropic"),
			attribute.String("gen_ai.request.model", "claude-sonnet-4-6"),
			attribute.Int("gen_ai.usage.input_tokens", 300),
			attribute.Int("gen_ai.usage.output_tokens", 150),
		)...),
	)

	_, toolSpan := tracer.Start(rootCtx, "tool/calculator",
		trace.WithAttributes(attrs(toolAttrs("calculator")...)...),
	)
	sleep(jitter(30 * time.Millisecond))
	toolSpan.End()

	_, llmSpan := tracer.Start(rootCtx, "assistant/respond",
		trace.WithAttributes(attrs(llmAttrs("claude-sonnet-4-6", 800, 200)...)...),
	)
	sleep(jitter(300 * time.Millisecond))
	llmSpan.SetStatus(codes.Ok, "")
	llmSpan.End()

	rootSpan.SetStatus(codes.Ok, "")
	rootSpan.End()
	return nil
}

// ── Scenario: parallel-tools ─────────────────────────────────────────────────
//
// Agent fires two tool calls "concurrently" (sequential in tracegen for
// simplicity) then synthesizes with an LLM call.

func scenarioParallelTools(ctx context.Context, tracer trace.Tracer, projectID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	attrs := func(agent string, extra ...attribute.KeyValue) []attribute.KeyValue {
		base := []attribute.KeyValue{
			attribute.String("agentpulse.project.id", projectID),
			attribute.String("agentpulse.run.id", runID),
			attribute.String("agentpulse.agent.name", agent),
		}
		return append(base, extra...)
	}

	rootCtx, rootSpan := tracer.Start(ctx, "planner",
		trace.WithAttributes(attrs("planner",
			attribute.String("agentpulse.span.kind", "llm.call"),
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", "openai"),
			attribute.String("gen_ai.request.model", "gpt-4o"),
			attribute.Int("gen_ai.usage.input_tokens", 600),
			attribute.Int("gen_ai.usage.output_tokens", 100),
		)...),
	)

	for _, tool := range []string{"fetch_weather", "fetch_news"} {
		_, ts := tracer.Start(rootCtx, fmt.Sprintf("tool/%s", tool),
			trace.WithAttributes(attrs("planner", toolAttrs(tool)...)...),
		)
		sleep(jitter(80 * time.Millisecond))
		ts.End()
	}

	_, synthSpan := tracer.Start(rootCtx, "planner/synthesize",
		trace.WithAttributes(attrs("planner", llmAttrs("gpt-4o", 1500, 350)...)...),
	)
	sleep(jitter(450 * time.Millisecond))
	synthSpan.SetStatus(codes.Ok, "")
	synthSpan.End()

	rootSpan.SetStatus(codes.Ok, "")
	rootSpan.End()
	return nil
}
