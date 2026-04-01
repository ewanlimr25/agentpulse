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

// ── helpers ──────────────────────────────────────────────────────────────────

// sampleConversations provides realistic prompt/completion pairs for eval testing.
// Each entry is {prompt, completion}. withLLM picks one at random so spans have
// varied content for the LLM judge to score.
var sampleConversations = [][2]string{
	{
		"Analyze the customer's message and classify their intent into one of: billing, technical, shipping, general.",
		"Based on the customer message I classify this as a billing dispute. The customer is questioning a charge from last month and wants a refund. Confidence: high.",
	},
	{
		"Review the following code diff and identify any security vulnerabilities or quality issues.",
		"I identified 2 issues: (1) SQL injection risk on line 45 — user input is concatenated directly into the query; (2) missing authentication check before the admin endpoint on line 78. Both are HIGH severity.",
	},
	{
		"Synthesize the following research sources into a concise summary of key findings.",
		"Three key findings emerge: latency increases non-linearly with batch size above 256; memory peaks at model initialization; throughput scales linearly up to 8 parallel workers before contention appears.",
	},
	{
		"Verify the following claim: 'The new feature reduced p99 latency by 40%'.",
		"Claim is SUPPORTED. Benchmark data from three test runs shows p99 latency dropped from 840ms to 504ms — a 40.0% reduction. Sample size is adequate (n=1200 per run).",
	},
	{
		"Draft a helpful response to this customer support inquiry about a delayed shipment.",
		"Thank you for reaching out. I can see your order #84729 is currently held at the regional distribution center due to weather delays. I've escalated this to priority handling and you should receive it within 2 business days. I'm applying a 10% discount to your next order as an apology for the inconvenience.",
	},
	{
		"Assess the credibility of the following sources and identify any conflicting information.",
		"Source A and Source C are credible (peer-reviewed, recent). Source B contains outdated statistics from 2019 that conflict with current data. I recommend relying on Sources A and C; Source B should be cited only for historical context.",
	},
	{
		"Diagnose the root cause of the pipeline failure based on the error logs provided.",
		"Root cause: the upstream Kafka topic 'events-raw' has a consumer lag of 2.3M messages, indicating the consumer group 'pipeline-worker' stopped processing 47 minutes ago. Likely cause: OOM kill on worker node ip-10-0-4-23. Recommended action: restart the consumer group and increase memory limits.",
	},
	{
		"Write a brief summary of the PR changes and assess whether they are ready to merge.",
		"This PR refactors the authentication middleware to use JWT instead of session cookies. Changes are clean and well-tested (92% coverage). One concern: the token expiry is hardcoded to 24h — recommend making it configurable. Otherwise ready to merge pending that fix.",
	},
	{
		"The external API call failed. Please analyze the error and suggest a retry strategy.",
		"The call to the payments API returned: rate limit exceeded, retry after 60 seconds. This is a transient error. Recommended strategy: exponential backoff starting at 60s, max 3 retries. If retries are exhausted, queue the request for async processing and notify the user.",
	},
	{
		"Authenticate with the model provider and run the inference job.",
		"authentication failed: invalid API key — please check that OPENAI_API_KEY is set correctly in your environment. The key provided starts with 'sk-proj-' which suggests it may be a project-scoped key that lacks inference permissions.",
	},
	{
		"The data pipeline job has not completed after the expected window. Diagnose the issue.",
		"connection timeout after 30s waiting for response from the data-enrichment service. The service at enrichment-svc:8080/enrich appears to be unresponsive. Last successful heartbeat was 4 minutes ago. Suggest restarting the pod and checking memory pressure on the node.",
	},
	{
		"Run inference on the provided dataset using the production model.",
		"quota exceeded for model gpt-4o — you have consumed 100% of your monthly token budget. Current usage: 10,000,000 / 10,000,000 tokens. Please upgrade your plan or wait until the quota resets on the 1st of next month.",
	},
	{
		"Please summarize the following document: 'Q3 Infrastructure Reliability Report — Service uptime was 99.2% across all regions. Two P1 incidents occurred: a database failover on Aug 14 (42-minute outage) and a CDN misconfiguration on Sep 3 (18-minute partial outage). Root causes have been addressed.'",
		"Summary: Q3 infrastructure achieved 99.2% uptime with two P1 incidents — a 42-minute database failover on Aug 14 and an 18-minute CDN misconfiguration on Sep 3. Both root causes have been resolved. Overall reliability is within SLA targets.",
	},
}

// piiSupportConversations contains realistic-looking but entirely fake PII for
// demonstrating the PII redaction feature. These are used by scenarioSupportTriagePII
// so that the customer-support-bot project has visible [REDACTED:xxx] tokens in its
// span attributes when PII redaction is enabled in the Settings tab.
//
// All data is synthetic: names, emails, card numbers, and API keys are fabricated.
var piiSupportConversations = [][2]string{
	{
		"Customer alice.morgan@example.com (account #ACC-847291) is disputing a charge of $149.99 on card 4111 1111 1111 1111. Billing address: 123 Main St, Springfield IL 62701. Please process the refund and send confirmation to her email.",
		"I've located account #ACC-847291 for alice.morgan@example.com. The disputed charge of $149.99 on the Visa card ending in 1111 has been flagged for review. A full refund will be credited within 3-5 business days and a confirmation will be sent to alice.morgan@example.com.",
	},
	{
		"User bob.chen@acme-corp.com is reporting an API integration issue. He shared his key sk-ant-api03-FAKEKEYDEMOONLY1234567890ABCDEFGHIJKLMNOPQRST for debugging. His phone is (555) 867-5309. Can you verify the credentials?",
		"I see the support ticket from bob.chen@acme-corp.com. Important: the customer should not share API credentials over support channels — the key beginning with sk-ant- must be rotated immediately. I've flagged this as a security education case and will contact bob.chen@acme-corp.com to guide them through key rotation.",
	},
	{
		"Please locate the order for customer SSN 123-45-6789, email carol.james@gmail.com, phone (408) 555-0192. She used card 5500 0000 0000 0004 for a purchase last Tuesday and is requesting an expedited shipping upgrade.",
		"Order located for carol.james@gmail.com. Note: SSNs are not required for order lookups — I've flagged this intake form for a privacy review. I've processed the shipping upgrade using the order ID derived from the email address only. Carol will receive a tracking update at carol.james@gmail.com within 1 hour.",
	},
}

// withLLMPII is like withLLM but picks from piiSupportConversations so the resulting
// span attributes contain PII that the piimaskerproc processor will redact when
// PII redaction is enabled for the project.
func withLLMPII(model, system string, inputTokens, outputTokens int) []attribute.KeyValue {
	conv := piiSupportConversations[rand.Intn(len(piiSupportConversations))]
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "llm.call"),
		attribute.String("gen_ai.operation.name", "chat"),
		attribute.String("gen_ai.system", system),
		attribute.String("gen_ai.request.model", model),
		attribute.Int("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int("gen_ai.usage.output_tokens", outputTokens),
		attribute.String("gen_ai.prompt", conv[0]),
		attribute.String("gen_ai.completion", conv[1]),
	}
}

func randTokens(minInput, maxInput, minOutput, maxOutput int) (int, int) {
	input := minInput + rand.Intn(maxInput-minInput)
	output := minOutput + rand.Intn(maxOutput-minOutput)
	return input, output
}

func baseAttrs(projectID, runID, agentName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.project.id", projectID),
		attribute.String("agentpulse.run.id", runID),
		attribute.String("agentpulse.agent.name", agentName),
	}
}

// sessionAttrs returns an attribute slice that stamps agentpulse.session_id on a span.
// Append to baseAttrs (or combine) when building session-grouped runs.
func sessionAttrs(sessionID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.session_id", sessionID),
	}
}

// userAttrs returns an attribute slice that stamps agentpulse.user_id on a span.
// userID should be an opaque identifier (e.g. "user-alice"), not an email address.
func userAttrs(userID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.user_id", userID),
	}
}

// optUserAttrs returns userAttrs(userID) when userID is non-empty, or nil otherwise.
// Use this to conditionally add user attribution without changing combine() call sites.
func optUserAttrs(userID string) []attribute.KeyValue {
	if userID == "" {
		return nil
	}
	return userAttrs(userID)
}

func withLLM(model, system string, inputTokens, outputTokens int) []attribute.KeyValue {
	conv := sampleConversations[rand.Intn(len(sampleConversations))]
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "llm.call"),
		attribute.String("gen_ai.operation.name", "chat"),
		attribute.String("gen_ai.system", system),
		attribute.String("gen_ai.request.model", model),
		attribute.Int("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int("gen_ai.usage.output_tokens", outputTokens),
		attribute.String("gen_ai.prompt", conv[0]),
		attribute.String("gen_ai.completion", conv[1]),
	}
}

func withTool(name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "tool.call"),
		attribute.String("agentpulse.tool.name", name),
	}
}

// withToolIO is like withTool but attaches tool.input and tool.output for search indexing.
func withToolIO(name, input, output string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "tool.call"),
		attribute.String("agentpulse.tool.name", name),
		attribute.String("tool.input", input),
		attribute.String("tool.output", output),
	}
}

func withHandoff(from, to string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "agent.handoff"),
		attribute.String("agentpulse.agent.name", from),
		attribute.String("agentpulse.handoff.target", to),
	}
}

func withMemRead(key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "memory.read"),
		attribute.String("agentpulse.memory.key", key),
	}
}

func withMemWrite(key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span.kind", "memory.write"),
		attribute.String("agentpulse.memory.key", key),
	}
}

func combine(slices ...[]attribute.KeyValue) []attribute.KeyValue {
	var out []attribute.KeyValue
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

func jitterD(base time.Duration) time.Duration {
	return base + time.Duration(rand.Int63n(int64(base/3)+1))
}

// ── Scenario: support-triage ─────────────────────────────────────────────────
//
// triage-agent (classify intent)
//   ├─ tool: fetch_customer_profile
//   ├─ llm:  classify intent  (haiku — cheap + fast)
//   ├─ handoff → kb-agent
//   │     ├─ tool: search_knowledge_base
//   │     └─ llm:  draft response  (sonnet)
//   └─ llm: compose final reply

func scenarioSupportTriage(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "triage-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withLLM("claude-haiku-4-5", "anthropic", rv(200, 600), rv(50, 150)),
		)...),
	)

	span(tracer, rootCtx, "tool/fetch_customer_profile",
		combine(baseAttrs(projectID, runID, "triage-agent"), user, withTool("fetch_customer_profile"))...,
	)(jitterD(30*time.Millisecond))

	spanLLM(tracer, rootCtx, "triage-agent/classify",
		combine(baseAttrs(projectID, runID, "triage-agent"), user,
			withLLM("claude-haiku-4-5", "anthropic", rv(400, 800), rv(80, 160)))...,
	)(jitterD(120*time.Millisecond))

	kbCtx, kbHandoff := tracer.Start(rootCtx, "handoff/kb-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withHandoff("triage-agent", "kb-agent"),
		)...),
	)

	span(tracer, kbCtx, "tool/search_knowledge_base",
		combine(baseAttrs(projectID, runID, "kb-agent"), user,
			withToolIO("search_knowledge_base",
				`{"query": "billing dispute refund policy", "index": "support-kb-v2"}`,
				`{"status": "error", "error": "insufficient permissions to read knowledge base index 'support-kb-v2' — service account lacks read role"}`,
			))...,
	)(jitterD(80*time.Millisecond))

	in, out := randTokens(600, 1800, 200, 600)
	spanLLM(tracer, kbCtx, "kb-agent/draft-response",
		combine(baseAttrs(projectID, runID, "kb-agent"), user,
			withLLM("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(300*time.Millisecond))

	kbHandoff.End()

	in, out = randTokens(500, 1200, 150, 400)
	spanLLM(tracer, rootCtx, "triage-agent/compose-reply",
		combine(baseAttrs(projectID, runID, "triage-agent"), user,
			withLLM("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(250*time.Millisecond))

	root.SetStatus(codes.Ok, "")
	root.End()
	return nil
}

// ── Scenario: support-triage-pii ─────────────────────────────────────────────
//
// Identical topology to support-triage but LLM calls use piiSupportConversations
// which contain fake emails, credit card numbers, and API keys. When PII
// redaction is enabled on the project (via the Settings tab), these attributes
// will be stored as [REDACTED:email], [REDACTED:credit_card], etc. in ClickHouse
// and displayed accordingly in the span detail drawer.

func scenarioSupportTriagePII(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "triage-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withLLMPII("claude-haiku-4-5", "anthropic", rv(200, 600), rv(50, 150)),
		)...),
	)

	span(tracer, rootCtx, "tool/fetch_customer_profile",
		combine(baseAttrs(projectID, runID, "triage-agent"), user, withTool("fetch_customer_profile"))...,
	)(jitterD(30 * time.Millisecond))

	spanLLM(tracer, rootCtx, "triage-agent/classify",
		combine(baseAttrs(projectID, runID, "triage-agent"), user,
			withLLMPII("claude-haiku-4-5", "anthropic", rv(400, 800), rv(80, 160)))...,
	)(jitterD(120 * time.Millisecond))

	kbCtx, kbHandoff := tracer.Start(rootCtx, "handoff/kb-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withHandoff("triage-agent", "kb-agent"),
		)...),
	)

	span(tracer, kbCtx, "tool/search_knowledge_base",
		combine(baseAttrs(projectID, runID, "kb-agent"), user,
			withToolIO("search_knowledge_base",
				`{"query": "billing dispute refund policy", "index": "support-kb-v2"}`,
				`{"status": "ok", "results": [{"id": "kb-482", "title": "Refund Policy", "snippet": "Refunds are processed within 3-5 business days to the original payment method."}]}`,
			))...,
	)(jitterD(80 * time.Millisecond))

	in, out := randTokens(600, 1800, 200, 600)
	spanLLM(tracer, kbCtx, "kb-agent/draft-response",
		combine(baseAttrs(projectID, runID, "kb-agent"), user,
			withLLMPII("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(300 * time.Millisecond))

	kbHandoff.End()

	in, out = randTokens(500, 1200, 150, 400)
	spanLLM(tracer, rootCtx, "triage-agent/compose-reply",
		combine(baseAttrs(projectID, runID, "triage-agent"), user,
			withLLMPII("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(250 * time.Millisecond))

	root.SetStatus(codes.Ok, "")
	root.End()
	return nil
}

// ── Scenario: support-escalation ─────────────────────────────────────────────
//
// Like triage but the kb-agent fails to find an answer → escalation-agent
// creates a ticket. Occasionally errors.

func scenarioSupportEscalation(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	shouldFail := rand.Intn(5) == 0 // 20% error rate
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "triage-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withLLM("claude-haiku-4-5", "anthropic", rv(300, 700), rv(60, 120)),
		)...),
	)

	span(tracer, rootCtx, "tool/fetch_customer_profile",
		combine(baseAttrs(projectID, runID, "triage-agent"), user, withTool("fetch_customer_profile"))...,
	)(jitterD(25*time.Millisecond))

	spanLLM(tracer, rootCtx, "triage-agent/classify",
		combine(baseAttrs(projectID, runID, "triage-agent"), user,
			withLLM("claude-haiku-4-5", "anthropic", rv(400, 900), rv(80, 180)))...,
	)(jitterD(100*time.Millisecond))

	// kb-agent — can't resolve, triggers escalation
	kbCtx, kbHandoff := tracer.Start(rootCtx, "handoff/kb-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withHandoff("triage-agent", "kb-agent"),
		)...),
	)
	span(tracer, kbCtx, "tool/search_knowledge_base",
		combine(baseAttrs(projectID, runID, "kb-agent"), user, withTool("search_knowledge_base"))...,
	)(jitterD(90*time.Millisecond))
	spanLLM(tracer, kbCtx, "kb-agent/no-match",
		combine(baseAttrs(projectID, runID, "kb-agent"), user,
			withLLM("claude-haiku-4-5", "anthropic", rv(300, 700), rv(40, 80)))...,
	)(jitterD(100*time.Millisecond))
	kbHandoff.End()

	// escalation-agent
	escCtx, escHandoff := tracer.Start(rootCtx, "handoff/escalation-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), user,
			withHandoff("triage-agent", "escalation-agent"),
		)...),
	)

	span(tracer, escCtx, "tool/fetch_case_history",
		combine(baseAttrs(projectID, runID, "escalation-agent"), user, withTool("fetch_case_history"))...,
	)(jitterD(40*time.Millisecond))

	_, ticketSpan := tracer.Start(escCtx, "tool/create_ticket",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "escalation-agent"), user,
			withTool("create_ticket"),
		)...),
	)
	time.Sleep(jitterD(60 * time.Millisecond))
	if shouldFail {
		ticketSpan.SetStatus(codes.Error, "ticket service timeout")
	}
	ticketSpan.End()

	escHandoff.End()

	if shouldFail {
		root.SetStatus(codes.Error, "escalation failed — ticket service unavailable")
	} else {
		root.SetStatus(codes.Ok, "")
	}
	root.End()
	return nil
}

// ── Scenario: deep-research ──────────────────────────────────────────────────
//
// orchestrator
//   ├─ memory.read (prior research context)
//   ├─ handoff → web-researcher
//   │     ├─ tool: web_search ×3
//   │     ├─ tool: arxiv_search
//   │     └─ llm: synthesize sources
//   ├─ memory.write (synthesis cache)
//   ├─ handoff → fact-checker
//   │     ├─ tool: verify_claim ×2
//   │     └─ llm: assess credibility
//   └─ llm: write final report

func scenarioDeepResearch(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "orchestrator",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator"), user,
			withLLM("gpt-4o", "openai", rv(800, 1500), rv(200, 400)),
		)...),
	)

	span(tracer, rootCtx, "memory.read/prior-research",
		combine(baseAttrs(projectID, runID, "orchestrator"), user, withMemRead("research_context"))...,
	)(jitterD(15*time.Millisecond))

	// web-researcher
	resCtx, resHandoff := tracer.Start(rootCtx, "handoff/web-researcher",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator"), user,
			withHandoff("orchestrator", "web-researcher"),
		)...),
	)
	for i := range 3 {
		span(tracer, resCtx, fmt.Sprintf("tool/web_search_%d", i+1),
			combine(baseAttrs(projectID, runID, "web-researcher"), user, withTool("web_search"))...,
		)(jitterD(120*time.Millisecond))
	}
	span(tracer, resCtx, "tool/arxiv_search",
		combine(baseAttrs(projectID, runID, "web-researcher"), user, withTool("arxiv_search"))...,
	)(jitterD(200*time.Millisecond))
	in, out := randTokens(3000, 6000, 600, 1200)
	spanLLM(tracer, resCtx, "web-researcher/synthesize",
		combine(baseAttrs(projectID, runID, "web-researcher"), user,
			withLLM("gpt-4o-mini", "openai", in, out))...,
	)(jitterD(500*time.Millisecond))
	resHandoff.End()

	span(tracer, rootCtx, "memory.write/synthesis-cache",
		combine(baseAttrs(projectID, runID, "orchestrator"), user, withMemWrite("synthesis_cache"))...,
	)(jitterD(10*time.Millisecond))

	// fact-checker
	fcCtx, fcHandoff := tracer.Start(rootCtx, "handoff/fact-checker",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator"), user,
			withHandoff("orchestrator", "fact-checker"),
		)...),
	)
	span(tracer, fcCtx, "memory.read/synthesis-cache",
		combine(baseAttrs(projectID, runID, "fact-checker"), user, withMemRead("synthesis_cache"))...,
	)(jitterD(12*time.Millisecond))
	for _, claim := range []string{"claim_1", "claim_2"} {
		span(tracer, fcCtx, "tool/verify_"+claim,
			combine(baseAttrs(projectID, runID, "fact-checker"), user, withTool("verify_claim"))...,
		)(jitterD(150*time.Millisecond))
	}
	in, out = randTokens(2000, 4000, 300, 700)
	spanLLM(tracer, fcCtx, "fact-checker/assess-credibility",
		combine(baseAttrs(projectID, runID, "fact-checker"), user,
			withLLM("gpt-4o", "openai", in, out))...,
	)(jitterD(400*time.Millisecond))
	fcHandoff.End()

	in, out = randTokens(5000, 10000, 800, 2000)
	spanLLM(tracer, rootCtx, "orchestrator/write-report",
		combine(baseAttrs(projectID, runID, "orchestrator"), user,
			withLLM("gpt-4o", "openai", in, out))...,
	)(jitterD(800*time.Millisecond))

	root.SetStatus(codes.Ok, "")
	root.End()
	return nil
}

// ── Scenario: fact-check ─────────────────────────────────────────────────────

func scenarioFactCheck(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	shouldFail := rand.Intn(10) == 0
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "fact-checker",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "fact-checker"), user,
			withLLM("gpt-4o", "openai", rv(600, 1200), rv(100, 300)),
		)...),
	)

	span(tracer, rootCtx, "memory.read/claim-store",
		combine(baseAttrs(projectID, runID, "fact-checker"), user, withMemRead("claim_store"))...,
	)(jitterD(10*time.Millisecond))

	for _, src := range []string{"primary_source", "secondary_source", "reference_db"} {
		span(tracer, rootCtx, "tool/search_"+src,
			combine(baseAttrs(projectID, runID, "fact-checker"), user, withTool("search_source"))...,
		)(jitterD(100*time.Millisecond))
	}

	_, verdictSpan := tracer.Start(rootCtx, "fact-checker/verdict",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "fact-checker"), user,
			withLLM("gpt-4o", "openai", rv(2000, 5000), rv(300, 600)),
		)...),
	)
	time.Sleep(jitterD(400 * time.Millisecond))
	if shouldFail {
		verdictSpan.SetStatus(codes.Error, "contradictory sources — unable to determine verdict")
	} else {
		verdictSpan.SetStatus(codes.Ok, "")
	}
	verdictSpan.End()

	span(tracer, rootCtx, "memory.write/verdict-cache",
		combine(baseAttrs(projectID, runID, "fact-checker"), user, withMemWrite("verdict_cache"))...,
	)(jitterD(10*time.Millisecond))

	if shouldFail {
		root.SetStatus(codes.Error, "fact-check inconclusive")
	} else {
		root.SetStatus(codes.Ok, "")
	}
	root.End()
	return nil
}

// ── Scenario: pr-review ──────────────────────────────────────────────────────
//
// review-orchestrator
//   ├─ tool: fetch_pr_diff
//   ├─ handoff → security-scanner
//   │     ├─ tool: run_semgrep
//   │     └─ llm: triage findings
//   ├─ handoff → style-checker
//   │     ├─ tool: run_linter
//   │     └─ llm: suggest improvements
//   └─ llm: write review summary

func scenarioPRReview(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	shouldFail := rand.Intn(7) == 0 // ~15% error
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "review-orchestrator",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "review-orchestrator"), user,
			withLLM("claude-sonnet-4-6", "anthropic", rv(400, 800), rv(100, 200)),
		)...),
	)

	span(tracer, rootCtx, "tool/fetch_pr_diff",
		combine(baseAttrs(projectID, runID, "review-orchestrator"), user, withTool("fetch_pr_diff"))...,
	)(jitterD(50*time.Millisecond))

	// security scanner
	secCtx, secHandoff := tracer.Start(rootCtx, "handoff/security-scanner",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "review-orchestrator"), user,
			withHandoff("review-orchestrator", "security-scanner"),
		)...),
	)
	span(tracer, secCtx, "tool/run_semgrep",
		combine(baseAttrs(projectID, runID, "security-scanner"), user,
			withToolIO("run_semgrep",
				`{"target": "src/", "rules": ["p/owasp-top-ten", "p/sql-injection"], "severity": ["ERROR", "WARNING"]}`,
				`{"findings": [{"rule": "sql-injection", "file": "src/db/queries.go", "line": 45, "message": "rate limit exceeded calls should be retried — but raw user input is concatenated into SQL string on this line"}, {"rule": "auth-bypass", "file": "src/api/admin.go", "line": 78, "message": "endpoint accessible without authentication check"}]}`,
			))...,
	)(jitterD(300*time.Millisecond))
	in, out := randTokens(1500, 3000, 200, 500)
	spanLLM(tracer, secCtx, "security-scanner/triage-findings",
		combine(baseAttrs(projectID, runID, "security-scanner"), user,
			withLLM("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(350*time.Millisecond))
	secHandoff.End()

	// style checker
	styleCtx, styleHandoff := tracer.Start(rootCtx, "handoff/style-checker",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "review-orchestrator"), user,
			withHandoff("review-orchestrator", "style-checker"),
		)...),
	)
	span(tracer, styleCtx, "tool/run_linter",
		combine(baseAttrs(projectID, runID, "style-checker"), user, withTool("run_linter"))...,
	)(jitterD(200*time.Millisecond))
	in, out = randTokens(2000, 4000, 300, 700)
	spanLLM(tracer, styleCtx, "style-checker/suggest-improvements",
		combine(baseAttrs(projectID, runID, "style-checker"), user,
			withLLM("claude-haiku-4-5", "anthropic", in, out))...,
	)(jitterD(280*time.Millisecond))
	styleHandoff.End()

	// final review summary
	_, summarySpan := tracer.Start(rootCtx, "review-orchestrator/write-summary",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "review-orchestrator"), user,
			withLLM("claude-sonnet-4-6", "anthropic", rv(3000, 6000), rv(500, 1000)),
		)...),
	)
	time.Sleep(jitterD(400 * time.Millisecond))
	if shouldFail {
		summarySpan.SetStatus(codes.Error, "conflicting signals from security and style agents")
	} else {
		summarySpan.SetStatus(codes.Ok, "")
	}
	summarySpan.End()

	if shouldFail {
		root.SetStatus(codes.Error, "review failed")
	} else {
		root.SetStatus(codes.Ok, "")
	}
	root.End()
	return nil
}

// ── Scenario: security-scan ──────────────────────────────────────────────────

func scenarioSecurityScan(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "security-scanner",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "security-scanner"), user,
			withLLM("claude-sonnet-4-6", "anthropic", rv(500, 1000), rv(100, 250)),
		)...),
	)

	for _, tool := range []string{"run_semgrep", "run_bandit", "check_dependencies"} {
		span(tracer, rootCtx, "tool/"+tool,
			combine(baseAttrs(projectID, runID, "security-scanner"), user, withTool(tool))...,
		)(jitterD(150*time.Millisecond))
	}

	in, out := randTokens(2000, 5000, 400, 900)
	spanLLM(tracer, rootCtx, "security-scanner/prioritize-findings",
		combine(baseAttrs(projectID, runID, "security-scanner"), user,
			withLLM("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(450*time.Millisecond))

	span(tracer, rootCtx, "memory.write/scan-results",
		combine(baseAttrs(projectID, runID, "security-scanner"), user, withMemWrite("scan_results"))...,
	)(jitterD(10*time.Millisecond))

	root.SetStatus(codes.Ok, "")
	root.End()
	return nil
}

// ── Scenario: pipeline-health ─────────────────────────────────────────────────
//
// monitor-agent checks data pipeline health, high error rate simulates
// real-world flakiness in data infrastructure.

func scenarioPipelineHealth(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	shouldFail := rand.Intn(4) == 0 // 25% error rate
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "monitor-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "monitor-agent"), user,
			withLLM("gpt-4o-mini", "openai", rv(200, 500), rv(50, 150)),
		)...),
	)

	span(tracer, rootCtx, "tool/query_pipeline_status",
		combine(baseAttrs(projectID, runID, "monitor-agent"), user, withTool("query_pipeline_status"))...,
	)(jitterD(80*time.Millisecond))

	span(tracer, rootCtx, "tool/check_data_quality",
		combine(baseAttrs(projectID, runID, "monitor-agent"), user, withTool("check_data_quality"))...,
	)(jitterD(120*time.Millisecond))

	span(tracer, rootCtx, "tool/fetch_error_logs",
		combine(baseAttrs(projectID, runID, "monitor-agent"), user,
			withToolIO("fetch_error_logs",
				`{"service": "data-enrichment", "tail": 50, "level": "ERROR"}`,
				`{"lines": ["[ERROR] connection timeout after 30s connecting to postgres-primary:5432", "[ERROR] connection timeout after 30s connecting to postgres-primary:5432", "[ERROR] max retries exceeded — circuit breaker open"]}`,
			))...,
	)(jitterD(60*time.Millisecond))

	_, diagSpan := tracer.Start(rootCtx, "monitor-agent/diagnose",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "monitor-agent"), user,
			withLLM("gpt-4o-mini", "openai", rv(1000, 2500), rv(150, 400)),
		)...),
	)
	time.Sleep(jitterD(200 * time.Millisecond))
	if shouldFail {
		diagSpan.SetStatus(codes.Error, "anomaly detected — downstream lag exceeded threshold")
	} else {
		diagSpan.SetStatus(codes.Ok, "")
	}
	diagSpan.End()

	if shouldFail {
		// alert path
		_, alertHandoff := tracer.Start(rootCtx, "handoff/alert-agent",
			trace.WithAttributes(combine(
				baseAttrs(projectID, runID, "monitor-agent"), user,
				withHandoff("monitor-agent", "alert-agent"),
			)...),
		)
		span(tracer, rootCtx, "tool/send_pagerduty_alert",
			combine(baseAttrs(projectID, runID, "alert-agent"), user, withTool("send_pagerduty_alert"))...,
		)(jitterD(40*time.Millisecond))
		alertHandoff.End()
		root.SetStatus(codes.Error, "pipeline degraded")
	} else {
		root.SetStatus(codes.Ok, "")
	}
	root.End()
	return nil
}

// ── span helper ──────────────────────────────────────────────────────────────

// span starts a span with attrs and returns a func(duration) that sleeps then ends it.
func span(tracer trace.Tracer, ctx context.Context, name string, attrs ...attribute.KeyValue) func(time.Duration) {
	_, s := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return func(d time.Duration) {
		time.Sleep(d)
		s.End()
	}
}

// spanLLM is like span but adds a stream.first_token event on ~50% of calls,
// simulating LLM streaming with realistic TTFT values (100–800 ms after start).
func spanLLM(tracer trace.Tracer, ctx context.Context, name string, attrs ...attribute.KeyValue) func(time.Duration) {
	startTime := time.Now()
	_, s := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return func(d time.Duration) {
		if rand.Intn(2) == 0 {
			ttftDelay := time.Duration(100+rand.Intn(700)) * time.Millisecond
			// Only add the event if the delay is within the span duration.
			if ttftDelay < d {
				time.Sleep(ttftDelay)
				s.AddEvent("stream.first_token", trace.WithTimestamp(startTime.Add(ttftDelay)))
				time.Sleep(d - ttftDelay)
			} else {
				time.Sleep(d)
			}
		} else {
			time.Sleep(d)
		}
		s.End()
	}
}

// rv returns a random int in [min, max).
func rv(min, max int) int {
	return min + rand.Intn(max-min)
}

// ── Session-aware scenario variants ─────────────────────────────────────────
//
// These mirror the support scenarios but stamp agentpulse.session_id on every
// span, grouping the run into a named conversation session.

func scenarioSupportTriageSession(ctx context.Context, tracer trace.Tracer, projectID, sessionID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	sess := sessionAttrs(sessionID)
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "triage-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withLLM("claude-haiku-4-5", "anthropic", rv(200, 600), rv(50, 150)),
		)...),
	)

	span(tracer, rootCtx, "tool/fetch_customer_profile",
		combine(baseAttrs(projectID, runID, "triage-agent"), sess, user, withTool("fetch_customer_profile"))...,
	)(jitterD(30 * time.Millisecond))

	spanLLM(tracer, rootCtx, "triage-agent/classify",
		combine(baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withLLM("claude-haiku-4-5", "anthropic", rv(400, 800), rv(80, 160)))...,
	)(jitterD(120 * time.Millisecond))

	kbCtx, kbHandoff := tracer.Start(rootCtx, "handoff/kb-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withHandoff("triage-agent", "kb-agent"),
		)...),
	)

	span(tracer, kbCtx, "tool/search_knowledge_base",
		combine(baseAttrs(projectID, runID, "kb-agent"), sess, user, withTool("search_knowledge_base"))...,
	)(jitterD(80 * time.Millisecond))

	in, out := randTokens(600, 1800, 200, 600)
	spanLLM(tracer, kbCtx, "kb-agent/draft-response",
		combine(baseAttrs(projectID, runID, "kb-agent"), sess, user,
			withLLM("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(300 * time.Millisecond))

	kbHandoff.End()

	in, out = randTokens(500, 1200, 150, 400)
	spanLLM(tracer, rootCtx, "triage-agent/compose-reply",
		combine(baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withLLM("claude-sonnet-4-6", "anthropic", in, out))...,
	)(jitterD(250 * time.Millisecond))

	root.SetStatus(codes.Ok, "")
	root.End()
	return nil
}

func scenarioSupportEscalationSession(ctx context.Context, tracer trace.Tracer, projectID, sessionID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	sess := sessionAttrs(sessionID)
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "triage-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withLLM("claude-haiku-4-5", "anthropic", rv(300, 700), rv(60, 120)),
		)...),
	)

	span(tracer, rootCtx, "tool/fetch_customer_profile",
		combine(baseAttrs(projectID, runID, "triage-agent"), sess, user, withTool("fetch_customer_profile"))...,
	)(jitterD(25 * time.Millisecond))

	spanLLM(tracer, rootCtx, "triage-agent/classify",
		combine(baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withLLM("claude-haiku-4-5", "anthropic", rv(400, 900), rv(80, 180)))...,
	)(jitterD(100 * time.Millisecond))

	kbCtx, kbHandoff := tracer.Start(rootCtx, "handoff/kb-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withHandoff("triage-agent", "kb-agent"),
		)...),
	)
	span(tracer, kbCtx, "tool/search_knowledge_base",
		combine(baseAttrs(projectID, runID, "kb-agent"), sess, user, withTool("search_knowledge_base"))...,
	)(jitterD(90 * time.Millisecond))
	spanLLM(tracer, kbCtx, "kb-agent/no-match",
		combine(baseAttrs(projectID, runID, "kb-agent"), sess, user,
			withLLM("claude-haiku-4-5", "anthropic", rv(300, 700), rv(40, 80)))...,
	)(jitterD(100 * time.Millisecond))
	kbHandoff.End()

	escCtx, escHandoff := tracer.Start(rootCtx, "handoff/escalation-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "triage-agent"), sess, user,
			withHandoff("triage-agent", "escalation-agent"),
		)...),
	)

	span(tracer, escCtx, "tool/fetch_case_history",
		combine(baseAttrs(projectID, runID, "escalation-agent"), sess, user, withTool("fetch_case_history"))...,
	)(jitterD(40 * time.Millisecond))

	span(tracer, escCtx, "tool/create_ticket",
		combine(baseAttrs(projectID, runID, "escalation-agent"), sess, user, withTool("create_ticket"))...,
	)(jitterD(60 * time.Millisecond))

	escHandoff.End()

	root.SetStatus(codes.Ok, "")
	root.End()
	return nil
}
