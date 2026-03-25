package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// withToolCall returns attributes for a tool span that includes input/output
// so the loop detector's hash queries can match them.
func withToolCall(toolName, input, output string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "tool.call"),
		attribute.String("agentpulse.tool.name", toolName),
		attribute.String("tool.input", input),
		attribute.String("tool.output", output),
	}
}

// ── Scenario: stuck-search-loop (Tier 1 — high confidence) ──────────────────
//
// Realistic stuck-agent pattern: the LLM reasons about the empty result,
// decides to retry the same query, gets the same result, and loops.
//
// topology per iteration:
//   research-agent (root llm)
//     └─ research-agent/decide-query   (llm — picks the search query)
//         └─ tool/web_search           (same input + output every time)
//             └─ research-agent/assess (llm — reads result, decides to retry)
//   ...repeated 3×, then give-up llm at the root
//
// The identical (span_name, tool.input, tool.output) across all 3 iterations
// triggers Tier 1 high-confidence detection.
func scenarioStuckSearchLoop(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())

	rootCtx, root := tracer.Start(ctx, "research-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "research-agent"),
			withLLM("claude-sonnet-4-6", "anthropic", rv(300, 500), rv(60, 100)),
		)...),
	)

	const query = `{"query":"latest quarterly revenue figures for ACME Corp","max_results":5}`
	const result = `{"results":[],"total":0,"message":"No results found. Try a different query."}`

	// Three identical search iterations — each with an LLM decision step before
	// the tool call and an LLM assessment step after, exactly as a real ReAct
	// agent would behave.
	assessments := []string{
		"Search returned no results. The query may be too specific — I'll try again with the same terms before broadening.",
		"Still no results. This is unexpected. One more attempt before I change strategy.",
		"Third consecutive empty result for the same query. Something is wrong — escalating.",
	}

	for i, assessment := range assessments {
		// LLM decides what to search for
		decideCtx, decideSpan := tracer.Start(rootCtx, "research-agent/decide-query",
			trace.WithAttributes(combine(
				baseAttrs(projectID, runID, "research-agent"),
				withLLM("claude-sonnet-4-6", "anthropic", rv(400, 700), rv(80, 120)),
			)...),
		)

		// Tool call — same input and output every iteration
		toolCtx, toolSpan := tracer.Start(decideCtx, "tool/web_search",
			trace.WithAttributes(combine(
				baseAttrs(projectID, runID, "research-agent"),
				withToolCall("web_search", query, result),
			)...),
		)
		time.Sleep(jitterD(time.Duration(80+i*20) * time.Millisecond))
		toolSpan.End()

		// LLM reads result and decides to retry
		span(tracer, toolCtx, "research-agent/assess-result",
			combine(
				baseAttrs(projectID, runID, "research-agent"),
				withLLM("claude-sonnet-4-6", "anthropic", rv(500, 900), rv(100, 180)),
				[]attribute.KeyValue{
					attribute.String("gen_ai.completion", assessment),
				},
			)...,
		)(jitterD(250 * time.Millisecond))

		decideSpan.End()
	}

	// Final give-up step
	span(tracer, rootCtx, "research-agent/give-up",
		combine(baseAttrs(projectID, runID, "research-agent"),
			withLLM("claude-sonnet-4-6", "anthropic", rv(600, 1000), rv(80, 150)))...,
	)(jitterD(200 * time.Millisecond))

	root.SetStatus(codes.Error, "unable to find required data after 3 identical search attempts")
	root.End()
	return nil
}

// ── Scenario: rapid-poll-loop (Tier 2 — low confidence) ──────────────────────
//
// A deployment-monitor agent polls check_deployment_status in a tight loop.
// Between each poll an LLM checks whether to keep waiting — it always says yes.
// Outputs vary slightly (different progress %) so Tier 1 won't match, but
// count ≥ 4 with avg_interval < 3s triggers Tier 2 low-confidence detection.
//
// topology per iteration:
//   deployment-monitor (root llm)
//     └─ monitor/should-retry   (llm — "still waiting?")
//         └─ tool/check_deployment_status  (same input, slowly changing output)
func scenarioRapidPollLoop(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())

	rootCtx, root := tracer.Start(ctx, "deployment-monitor",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "deployment-monitor"),
			withLLM("gpt-4o-mini", "openai", rv(200, 400), rv(40, 80)),
		)...),
	)

	const deployInput = `{"deployment_id":"deploy-a3f7b2","env":"production"}`

	// Status barely changes — agent keeps polling because it never times out.
	polls := []struct {
		output  string
		opinion string
	}{
		{`{"status":"pending","progress":2}`, "Deployment is at 2%. Still initialising — will check again shortly."},
		{`{"status":"pending","progress":4}`, "Only 4% after another check. Slow but still moving — continue waiting."},
		{`{"status":"pending","progress":4}`, "Still 4%. Could be stalled. I should check one more time before alerting."},
		{`{"status":"pending","progress":6}`, "6% now. Progress is real but extremely slow. Checking again."},
		{`{"status":"pending","progress":6}`, "Still 6%. This is taking far too long. One final check before I escalate."},
	}

	for _, p := range polls {
		retryCtx, retrySpan := tracer.Start(rootCtx, "monitor/should-retry",
			trace.WithAttributes(combine(
				baseAttrs(projectID, runID, "deployment-monitor"),
				withLLM("gpt-4o-mini", "openai", rv(300, 600), rv(60, 100)),
				[]attribute.KeyValue{attribute.String("gen_ai.completion", p.opinion)},
			)...),
		)

		span(tracer, retryCtx, "tool/check_deployment_status",
			combine(
				baseAttrs(projectID, runID, "deployment-monitor"),
				withToolCall("check_deployment_status", deployInput, p.output),
			)...,
		)(jitterD(700 * time.Millisecond))

		retrySpan.End()
	}

	span(tracer, rootCtx, "monitor/escalate",
		combine(baseAttrs(projectID, runID, "deployment-monitor"),
			withLLM("gpt-4o-mini", "openai", rv(500, 900), rv(100, 180)))...,
	)(jitterD(200 * time.Millisecond))

	root.SetStatus(codes.Error, "deployment timed out — still pending after 5 status checks")
	root.End()
	return nil
}
