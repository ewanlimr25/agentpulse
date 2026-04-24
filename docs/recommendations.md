# Recommendations

**Date:** April 2026
**Audience:** maintainers of AgentPulse; contributors evaluating where to spend effort
**Goal:** identify the highest-leverage improvements that would make AgentPulse the default self-hosted observability platform for *independent developers* running multi-agent AI workflows.

This is a research-backed roadmap, not an opinion piece. Every claim has a citation. Where the evidence contradicts intuition, the citation wins.

---

## Executive summary — the five biggest levers

1. **Ship a single-binary "indie mode"** (SQLite metadata + DuckDB span store). The single loudest complaint about self-hosted observability in 2026 is operational TCO — *"Langfuse is free software that is not free to operate… Postgres + ClickHouse + Redis + S3 + several hours per week"* ([Glassbrain, "Langfuse Pricing in 2026"](https://glassbrain.dev/blog/langfuse-pricing)). Arize Phoenix wins indie mindshare today mostly because it runs as one container. A single-binary AgentPulse that starts in under 60 seconds undercuts both Langfuse and Phoenix on the dimension indie devs value most.
2. **Close the MCP observability gap.** Reviews routinely name "no MCP support" as the defect that forces tool switching — *"It's genuinely good… except for the MCP gap. No MCP support. If you're building with Claude and MCP tools, you're blind."* ([dev.to comparative review](https://dev.to/soufian_azzaoui_85ea1c030/i-tried-langsmith-langfuse-helicone-and-phoenix-heres-what-each-gets-wrong-2cjk)). AgentPulse already has a `mcp.tool.call` span kind plumbed — shipping an end-to-end MCP surface (client-side + server-side correlation, MCP server exposing trace tools to IDE agents) is weeks of work, not months.
3. **Ship agent-run replay ("time-travel debugging").** AgentOps owns this narrative commercially, Sentry ships it for multi-agent runs, and the playground is already 70% of the way there. Finish it.
4. **Add trajectory evals and online (sampled) evals.** Current evals are output-level. The 2026 frontier is *trajectory* evaluation — scoring the full sequence of reasoning/tool-use/handoffs — backed by Anthropic's published guidance on this exact pattern ([Anthropic — Demystifying Evals for AI Agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)).
5. **Detect silent tool failures as a first-class signal.** Community reporting converges on silent tool failure (a tool returns a valid-looking response the agent misinterprets) as the dominant undetected agent bug ([Latitude — debugging AI agents in production](https://latitude.so/blog/complete-guide-debugging-ai-agents-production)). Nobody ships a named feature for it. AgentPulse's multi-signal alerting can.

Everything below expands these five, plus P1/P2 recommendations, plus a concrete indie-mode blueprint.

---

## Landscape — who else is in the market

### Pure OSS / self-hostable

- **[Langfuse](https://langfuse.com)** — OSS (MIT), YC W23, ~19k GitHub stars. Most complete feature set (traces, prompts, datasets, evals, playground). Self-host stack: Postgres + ClickHouse + Redis + S3. Cloud pricing: Hobby free (50k units), Core $29/mo, Pro $199/mo, Teams add-on $300/mo for SSO/RBAC ([langfuse.com/pricing](https://langfuse.com/pricing)).
- **[Arize Phoenix](https://arize.com/docs/phoenix)** — OSS (Apache 2). Single-container self-host. OpenInference spec. Strong for classic-ML + LLM combined teams.
- **[Helicone](https://helicone.ai)** — OSS, YC W23. Proxy-based — two-line SDK swap, no instrumentation needed. Pricing: Hobby free (10k req), Pro $79/mo ([helicone.ai/pricing](https://helicone.ai/pricing)). *"Helicone is a proxy, not an instrumentation layer… it only sees HTTP traffic. No agent tracing, no span-level visibility"* ([dev.to](https://dev.to/soufian_azzaoui_85ea1c030/i-tried-langsmith-langfuse-helicone-and-phoenix-heres-what-each-gets-wrong-2cjk)).
- **[Traceloop / OpenLLMetry](https://www.traceloop.com)** — OSS SDK + commercial cloud. OTel-native. Multi-language (Py/TS/Go/Ruby).
- **[OpenLIT](https://openlit.io)** — OSS, OTel-native. Kubernetes operator for auto-instrumentation. 50+ integrations.
- **[Lunary](https://lunary.ai)** — OSS with cloud. PII masking built-in; SOC 2 Type II + ISO 27001; VPC self-host on K8s/Docker.

### Commercial

- **[Braintrust](https://www.braintrust.dev)** — eval-first; online scoring of production traces with no latency impact; IDE-native MCP server for Cursor/Claude Code. Not self-hostable in OSS form.
- **[LangSmith](https://www.langchain.com/langsmith)** — deep LangChain/LangGraph integration. *"$39/seat/month — before you log a single trace"* is the dominant community complaint.
- **[AgentOps](https://www.agentops.ai)** — agent-first, time-travel debugging. 400+ framework claims. Pricing: Basic free (5k events), Pro $40/mo.
- **[W&B Weave](https://wandb.ai/site/weave)** — multimodal tracking (text/images/audio), purpose-built for agentic systems, OpenAI Agents SDK + MCP integrations.
- **[Datadog LLM Observability](https://docs.datadoghq.com/llm_observability/)** — enterprise, anomaly detection, prompt injection detection, native OTel GenAI semconv support. Priced out of indie reach.
- **[Logfire (Pydantic)](https://pydantic.dev/logfire)** — full-stack (not LLM-only), SQL query over traces, native Pydantic AI integration, SOC2 Type II + HIPAA. Self-host only on enterprise plans. 10M free spans/month without a card.

### Feature matrix vs AgentPulse

| Feature | AgentPulse | Langfuse | Phoenix | Helicone | Braintrust | AgentOps | Logfire |
|---|---|---|---|---|---|---|---|
| OTel-native collection | ✅ | ingest only | ✅ | ❌ | partial | partial | ✅ |
| Traces + DAG view | ✅ React Flow | waterfall | waterfall | ❌ | waterfall | ✅ | waterfall |
| Cost tracking | ✅ | ✅ | partial | ✅ | ✅ | ✅ | ✅ |
| LLM-as-judge | ✅ (6 types) | ✅ | ✅ | ❌ | ✅ | partial | ✅ |
| Prompt playground | ✅ | ✅ | ✅ | partial | ✅ | ❌ | partial |
| Datasets + experiments | partial | ✅ | ✅ | partial | ✅ | ❌ | ✅ |
| Sessions | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Mid-run budget halts | ✅ | ❌ | ❌ | partial | ❌ | ❌ | ❌ |
| Multi-signal alerting | ✅ | partial | ❌ | partial | partial | partial | ✅ |
| Semantic + FTS trace search | ✅ | partial | partial | ❌ | partial | ❌ | ✅ (SQL) |
| Framework auto-instrument | ✅ | ✅ | ✅ | partial | ✅ | ✅ | ✅ |
| CLI | ✅ | partial | partial | ❌ | ✅ | ❌ | ❌ |
| Claude Code hook | ✅ | community | ❌ | ❌ | ❌ | ❌ | community |
| S3 payload offload | ✅ | ✅ (req'd) | partial | ❌ | ✅ | ❌ | ✅ |
| CI quality gates | ✅ | partial | ❌ | ❌ | ✅ | ❌ | partial |
| Slack/Discord notifs | ✅ | partial | ❌ | partial | ❌ | ❌ | partial |
| **MCP-native observability** | 🟡 span kind | ❌ | ❌ | ❌ | MCP server | partial | ❌ |
| **Trajectory eval** | ❌ | ❌ | partial | ❌ | ✅ | partial | ❌ |
| **Online eval (sampled prod)** | ❌ | partial | partial | ❌ | ✅ | ❌ | ✅ |
| **Time-travel replay** | 🟡 bundle only | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| **Guardrails telemetry** | ❌ | partial | ❌ | ❌ | ❌ | partial | partial |
| **Reasoning-token analysis** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | partial |
| **Multimodal trace support** | ❌ | partial | partial | ❌ | partial | ❌ | partial |
| **Cost-per-end-user attribution** | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| **Single-binary self-host** | ❌ | ❌ | ✅ | ❌ | N/A | N/A | N/A |
| **PII redaction built-in** | ✅ regex | partial | ❌ | ❌ | ❌ | ❌ | partial |

Legend: ✅ shipped · 🟡 partial/backend-only · ❌ not yet

---

## User pain points (with citations)

These are the recurring complaints from indie builders in late-2025 and early-2026. Any recommendation that maps to one of these has evidence behind it.

### "Self-hosted open source ≠ cheap"

> *"The software is free but the infrastructure and operations are not… Postgres, ClickHouse, Redis, S3, container orchestration… several hours per week in steady state. For teams that do not already run this infrastructure, the total cost of ownership often exceeds the cloud plan they were avoiding."*
> — [Glassbrain, "Langfuse Pricing in 2026"](https://glassbrain.dev/blog/langfuse-pricing)

### Per-seat pricing is hostile to small teams

> *"$39/seat/month — before you log a single trace. A team of 5 = $195/month just to get started."*
> — [dev.to — four-platform comparative review](https://dev.to/soufian_azzaoui_85ea1c030/i-tried-langsmith-langfuse-helicone-and-phoenix-heres-what-each-gets-wrong-2cjk)

### MCP blindness

> *"No MCP support. If you're building with Claude and MCP tools, you're blind."*
> — same dev.to review.

### Proxy-only tools can't see agents

> *"Helicone is a proxy, not an instrumentation layer. That means it only sees HTTP traffic. No agent tracing, no span-level visibility. If you want to understand why your agent made a decision, Helicone can't help you."*
> — same review.

### Silent multi-turn failures are the dominant agent bug

> *"Silent tool failure is the most dangerous: a tool returns a valid response the agent misinterprets, corrupting all downstream reasoning without triggering any error — only detectable in full session trace analysis."*
> — [Latitude — complete guide to debugging AI agents in production](https://latitude.so/blog/complete-guide-debugging-ai-agents-production)

### Multi-agent graphs: failures compound invisibly

> *"Multi-agent observability tracks a graph of interconnected reasoning chains where the output of one agent becomes the input of another. A failure anywhere in the graph can silently corrupt everything downstream."*
> — [Sentry — debugging multi-agent AI](https://blog.sentry.io/debugging-multi-agent-ai-when-the-failure-is-in-the-space-between-agents/)

### Agents generate far more spans than humans expect

> *"An agent that reasons through 5 steps generates 40–75 spans."*
> — [OneUptime — AI agents are breaking your observability budget](https://oneuptime.com/blog/post/2026-03-07-ai-agents-breaking-observability-budget/view)

### Self-hosted Langfuse has real production-scaling rough edges

Bug [langfuse/langfuse#7591](https://github.com/langfuse/langfuse/issues/7591): *"K8s Self-Host, High Data Volume Causes Liveness Probe Failures & Restarts"*. Multiple community reports: `@smithy/node-http-handler` socket warnings, liveness probe failures, Redis memory climb.

### Quality is the #1 production blocker

> *"57% of organizations have agents in production; quality is cited as the top deployment barrier by 32%"*
> — [LangChain — 2026 State of AI Agents](https://www.langchain.com/articles/agent-observability)

---

## Prioritized recommendations

Priority is impact × cost-to-indie-dev — the degree to which this moves the needle for the target audience, weighted against how much engineering time it takes.

### P0 — ship in the next 8 weeks

#### P0-1. Single-binary "Indie Mode" (SQLite + DuckDB backend)

**Problem.** Self-host TCO kills adoption; Phoenix wins indies today purely on container simplicity.

**How others do it.** Phoenix: one Docker image. Langfuse: Postgres + ClickHouse + Redis + S3. No major player ships a true `./agentpulse` binary that starts in 2 seconds. Prior-art for single-binary indie tools: Plausible Analytics, Umami, Grafana Agent, SigNoz (close).

**Proposed approach.** Compile the Go backend with:
- `modernc.org/sqlite` for Postgres-equivalent tables (`projects`, `topology_*`, `budget_*`, `alert_*`, `eval_configs`, `run_tags`, `ingest_tokens`, `retention_policies`, `pii_configs`, `playground_versions`, `span_feedback`, `push_subscriptions`)
- `marcboeker/go-duckdb` for ClickHouse-equivalent columnar tables (`spans`, `metrics_agg`, `run_metrics`, `session_agg`, `user_agg`, `span_evals`, search indexes via DuckDB FTS + `vss` extension)
- Embedded OTLP receiver (no separate collector process)
- Embedded Next.js build served as static assets

Keep ClickHouse + Postgres + separate collector as "team mode," selected by startup flag (`--mode=team|indie`). Indie mode falls back to local filesystem for payload offload instead of S3.

**Effort.** M (2–4 engineer-weeks). The storage layer is already behind repository interfaces; the bulk of the work is implementing `chstore` and `pgstore` against DuckDB and SQLite plus bundling the frontend.

**Why this is P0 for indies.** It's the single biggest barrier between "I'll try this tonight" and "I'll set this up next sprint." See the Glassbrain quote above.

**Target footprint.**

| State | RAM | Disk |
|---|---|---|
| Idle | <100 MB | <50 MB |
| 100 sessions/day | <500 MB | ~50 MB per 100k spans (DuckDB compression) |
| 1M spans/day | ~1 GB | growth dominated by payloads |

#### P0-2. MCP-native observability (plus auth tightening)

**Problem.** Every reviewed platform is weak on MCP except Braintrust (consumer-side MCP server only, not server-side MCP observability). Claude Code, Cursor, and Windsurf users generate a rising share of MCP tool calls; they're the target buyer.

**How others do it.** Braintrust exposes an MCP server so IDEs can *query* traces ([braintrust.dev](https://www.braintrust.dev/articles/langfuse-alternatives-2026)); Galileo has Agent Evals MCP ([Maxim](https://www.getmaxim.ai/articles/top-5-agent-observability-platforms-in-2026/)). Langfuse has not shipped. OTel now specifies `invoke_agent` / `execute_tool` span operations ([OTel GenAI agent semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/)).

**Proposed approach.**

1. Finish the `mcp.tool.call` span kind end-to-end: server-side MCP spans carry `mcp.server.name`, `mcp.tool.name`, `mcp.session.id`; client-side instrumentation in Python/TS SDKs correlates via `mcp.session.id` so a stitched trace shows both the agent-side call and the MCP-server-side execution.
2. Ship an **AgentPulse MCP server** exposing tools: `search_traces`, `get_run_details`, `compare_prompts`, `replay_run`, `current_budget_status`. This lets Claude Code self-diagnose via MCP queries — "why did my last run cost $2?" becomes a natural-language prompt the IDE can satisfy locally.
3. **Tighten OTel-receiver auth** in the process: collector requires `Authorization: Bearer <ingest_token>` from the same `project_ingest_tokens` table the REST API uses. Enforcement off by default in indie mode, on by default in team mode with a clear env-var flag.

**Effort.** M (2–3 engineer-weeks).

**Why this is P0 for indies.** Every Claude Code / Cursor user today is the ideal user; this is the feature that makes them switch from ad-hoc logging.

#### P0-3. Agent run replay ("time-travel")

**Problem.** Indies want to re-run a failing agent from turn N with a patched prompt or tool mock, not wade through spans.

**How others do it.** AgentOps markets "Time Travel Debugging"; Sentry ships session-replay linkage; Traceloop lets you "re-run issues from production in your IDE."

**Proposed approach.** AgentPulse already has the `replay` CLI command (downloads a bundle) and the playground (edit + re-send a single LLM span). Finish the loop:

1. **Fork-run endpoint.** `POST /api/v1/runs/:runId/fork?from_span=:spanId` rebuilds context at an arbitrary turn (all spans up to and including `spanId`, with optional prompt/tool overrides) and emits it as a new run with `parent_run_id = :runId`.
2. **Stub-mode tool calls.** When replaying, the SDK can be told to use recorded tool responses instead of calling the real tool. Useful for determinism and for debugging without re-incurring side effects.
3. **Diff-view.** UI overlays the old run's trajectory against the new run's trajectory — shows where they diverge, cost delta, quality delta.

**Effort.** M (2 engineer-weeks).

**Why this is P0 for indies.** Single strongest *demo* feature — easy to screenshot on HN/Twitter. Converts curious visitors into users.

---

### P1 — next 3 months

#### P1-1. Trajectory evals / agent-as-judge

Current LLM-as-judge scores outputs. Anthropic's published guidance emphasizes trajectory-level eval: tool-selection correctness, step efficiency, backtracking, goal adherence ([Anthropic — Demystifying Evals for AI Agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)). Braintrust and Maxim market this; Langfuse does not.

Add a `trajectory_evaluator` that consumes the span tree for a run and produces a score. Runs offline on datasets and optionally online (sampled) on prod.

**Effort.** M. **Why indies:** "did my agent screw up this whole session?" is a more useful question than "is this individual answer relevant?"

#### P1-2. Online evals (sampled production scoring)

Running evals on every span is prohibitively expensive. Braintrust advertises production scoring "with no impact on latency"; Logfire runs Pydantic Evals on sampled production traces.

Add a sampler (1-in-N or per-project budget cap) that dispatches trajectory + output evals from a queue worker. Scores emitted as additional spans tagged `eval.kind=online`. Reuses the existing `budgetenforceproc` path for the cost cap.

**Effort.** S. Builds on existing eval infra. **Why indies:** lets them say "yes, I eval in prod" without 100×'ing their token bill.

#### P1-3. Guardrails telemetry as a first-class span kind

Datadog monitors prompt-injection attempts; Langfuse has docs on it; nobody ships an OSS-first implementation.

Define `gen_ai.guardrail.*` attributes (`injection_score`, `pii_found`, `toxicity`, `policy_violations`). Ship optional in-process detectors (LLM Guard / Presidio / regex PII) as collector processors. Default alert rule: spike in `guardrail.injection_score > 0.8`.

**Effort.** M. **Why indies:** GDPR / privacy is a structural advantage of self-hosted — AgentPulse should lean into it.

#### P1-4. Silent tool-failure detector

The dominant undetected agent-failure pattern. Nobody ships this as a named feature.

Statistical anomaly detection on tool outputs per `tool.name`:
- Empty-result frequency
- Schema drift (new/missing fields)
- Downstream-reference rate (did the agent *use* the tool output in its next turn?)
- Semantic similarity of consecutive outputs (stuck loops)

Emit `anomaly.*` events; wire into the existing multi-signal alert evaluator.

**Effort.** M. **Why indies:** genuinely *novel* — "AgentPulse catches silent tool failures no one else sees" is a marketable headline.

#### P1-5. Cost-per-end-user / per-customer attribution, polished

AgentPulse already supports `agentpulse.user.id` and has a `user_agg` ClickHouse table. Surface it better:
- First-class dashboard tiles: top-N users by cost, fastest-growing user
- Power-user alerting — e.g., *"3% of tenants consume 60% of tokens"* as seen in [Digital Applied — LLM agent cost attribution](https://www.digitalapplied.com/blog/llm-agent-cost-attribution-guide-production-2026)
- Per-user quality score (not just cost) — correlate with churn risk

**Effort.** S. **Why indies:** converts hobby-scale users into paying SaaS operators who need billing-grade attribution.

---

### P2 — 3–6 months, strategic

#### P2-1. Reasoning-trace analysis for extended-thinking models

Claude extended-thinking, o1/o3, Gemini/DeepSeek thinking traces dominate cost for agentic workloads. Today AgentPulse treats them as one opaque token count.

Parse `thinking` blocks: length histograms, effort-level attribution, "wasted thinking" detection (high reasoning tokens + wrong answer → red flag).

**Effort.** M.

#### P2-2. Synthetic dataset generation from production traces

Indies can't afford to curate eval datasets. One-click *"take the last 500 failed sessions, de-PII them, generate eval test cases."* Patterns exist ([Anthropic guidance](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)); no OSS platform has shipped it as one button.

**Effort.** M.

#### P2-3. Human-in-the-loop annotation queue

Minimal keyboard-driven UI — when auto-eval flags a run low, a human hits ↑/↓ in 5 seconds to label it. Feeds labeled examples back into the dataset for future regression tests.

**Effort.** S.

#### P2-4. Agent memory observability

Mem0 / Letta / Zep are standard in 2026 ([Mem0 — State of AI Agent Memory 2026](https://mem0.ai/blog/state-of-ai-agent-memory-2026)); memory reads/writes are a new blind spot.

Promote `memory.read` / `memory.write` from existing span kinds into a dedicated dashboard: memory-hit-rate, stale-memory detection, cost of memory ops per session.

**Effort.** M.

#### P2-5. Framework auto-instrumentation for Pydantic AI, Mastra, Google ADK

Framework adoption in 2026 nearly doubled YoY. The fast-movers AgentPulse doesn't yet cover:

- **Pydantic AI** — Logfire pairs it natively; AgentPulse should undercut Logfire by supporting it + being self-hostable.
- **Mastra** — TS-first, indie-friendly.
- **Google ADK** — emerging but picking up.

**Effort.** S each, additive.

---

## Things *not* to build

Every enterprise feature has a cost. For this target audience, these are drag:

- **Heavy RBAC / SSO / SCIM.** Langfuse's Teams add-on is $300/mo for this alone; indies want one admin user. Keep SSO as an opt-in build tag (e.g., `go build -tags=sso`), not a core feature.
- **SOC2 / HIPAA pursuit.** Self-hosted means *your* posture covers it; AgentPulse shouldn't spend cycles on vendor audits the target audience doesn't need.
- **Multi-region deployment topology.** Single-region fine for indie self-host.
- **Dedicated support SLAs / CSMs.** Drives pricing model expectations upward.
- **Per-seat pricing** if a cloud tier appears. It is the single most-cited reason for abandoning LangSmith. Charge by volume, not seats.
- **A bespoke query language.** Logfire's SQL-over-traces is the right move ([Logfire docs](https://pydantic.dev/logfire)). Avoid PromQL-like DSLs that force learning.
- **Full-text enterprise audit logs.** OTel spans *are* the audit log for AgentPulse. Surface them better; don't add a second log stack.
- **Vendor-specific agent "marketplaces" or template stores.** Scope creep; stay observability-native.
- **A managed cloud as the primary business.** Stay self-host-first. A hosted tier can come later but shouldn't shape the roadmap.

---

## Indie-mode blueprint

**Goal.** `curl -sSL get.agentpulse.io | sh` → functional observability in under 60 seconds, no Docker required.

### Design principles (drawn from Plausible, Umami, Grafana-Agent, SQLite-first indie tools)

1. **One binary, zero deps.** Single Go binary, ~40 MB. No Docker required. Runs on a $5 VPS or a laptop.
2. **SQLite for metadata, DuckDB for spans.** Both embedded. File-based. Snapshot by `cp`. DuckDB handles columnar aggregations at 1M+ spans/day — competitive with ClickHouse at solo scale ([DuckDB vs SQLite analytics benchmark](https://marending.dev/notes/sqlite-vs-duckdb/)).
3. **Single config file** (`~/.agentpulse/config.toml`) or env vars. Auto-generates a Bearer token on first run, prints it to stdout once, writes it to `~/.agentpulse/api_key`.
4. **Built-in OTLP ingest** on `:4318` (HTTP) and `:4317` (gRPC). No separate collector process.
5. **Static-served UI** from the same binary. No Next.js SSR — build with `next export` or migrate to a pre-bundled SPA.
6. **Local-first LLM-as-judge.** Ship with Ollama auto-detection; if a local model exists, evals run locally for free. Cloud-model fallback with user API key.
7. **Retention defaults: 14 days of spans, unlimited metric aggregates.** Periodic DuckDB compaction.
8. **Payload offload to local filesystem**, not S3, in indie mode. Pluggable to S3/R2 later.
9. **Upgrade path.** `agentpulse migrate --to=team` dumps DuckDB → ClickHouse and SQLite → Postgres. Same binary, different storage-mode flag.
10. **Offline-friendly.** No phone-home except opt-in update checks.

### Feature delta

| | Indie mode | Team mode |
|---|---|---|
| Binary | single Go binary | Go + static frontend + collector |
| Spans store | DuckDB file | ClickHouse cluster |
| Metadata store | SQLite | Postgres |
| Payloads | local FS | S3 / R2 / MinIO |
| Auth | single admin token | multi-user, optional SSO |
| Retention | 14 days default | configurable / unlimited |
| Evals | local Ollama default | any provider |
| Alerts | Slack / Discord webhooks | full notification channels |
| MCP server | ✅ | ✅ |
| Replay | ✅ | ✅ |

### Prior art to emulate

- **Plausible Analytics** — single binary, SQLite + ClickHouse dual mode.
- **Umami** — indie-scale analytics, one container.
- **Grafana Agent / Alloy** — single binary, OTLP native.
- **SigNoz** — close to the target architecture but still requires docker-compose.
- **Fly.io LiteFS** — demonstrates SQLite scales further than people expect.

---

## Closing synthesis

AgentPulse has the right spine: OTel-native collection, multi-judge evals, sessions, multi-signal alerts, Claude Code hooks. The gap between "credible competitor" and "default for indie devs" is not more features — it's three things the incumbents are *structurally* bad at:

1. **Operational simplicity** — indie mode single binary. Langfuse cannot ship this without rewriting their core; Phoenix has the simplicity but not the feature set.
2. **MCP-native + agent-trajectory observability** — the feature frontier everyone is marketing but nobody has shipped cleanly. AgentPulse's OTel spine is the right place to do it first.
3. **Zero-seat, zero-lock-in posture** — the most-cited limitation of every commercial tool. If a cloud tier appears, volume-priced from day one.

If AgentPulse ships **P0-1** (indie mode), **P0-2** (MCP observability + auth), and **P0-3** (replay) in the next two months, the narrative *"the self-hosted observability platform you can actually run in 60 seconds"* becomes defensible on HN, r/LocalLLaMA, and r/LangChain. Everything in P1 compounds from there.

---

## Sources

- [Langfuse pricing](https://langfuse.com/pricing)
- [Helicone pricing](https://helicone.ai/pricing)
- [Arize Phoenix docs](https://arize.com/docs/phoenix)
- [AgentOps](https://www.agentops.ai/)
- [OpenLIT](https://openlit.io/)
- [Braintrust — evals](https://www.braintrust.dev/docs/guides/evals)
- [Traceloop docs](https://www.traceloop.com/docs)
- [Pydantic Logfire](https://pydantic.dev/logfire)
- [Lunary](https://lunary.ai/)
- [W&B Weave](https://wandb.ai/site/weave)
- [Datadog LLM Observability](https://docs.datadoghq.com/llm_observability/)
- [Glassbrain — Langfuse Pricing in 2026](https://glassbrain.dev/blog/langfuse-pricing)
- [dev.to — I tried LangSmith, Langfuse, Helicone, Phoenix](https://dev.to/soufian_azzaoui_85ea1c030/i-tried-langsmith-langfuse-helicone-and-phoenix-heres-what-each-gets-wrong-2cjk)
- [Braintrust — Langfuse alternatives 2026](https://www.braintrust.dev/articles/langfuse-alternatives-2026)
- [OpenTelemetry GenAI agent spans semconv](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/)
- [Latitude — Debugging AI agents in production](https://latitude.so/blog/complete-guide-debugging-ai-agents-production)
- [Sentry — Debugging multi-agent AI](https://blog.sentry.io/debugging-multi-agent-ai-when-the-failure-is-in-the-space-between-agents/)
- [OneUptime — AI agents breaking observability budget](https://oneuptime.com/blog/post/2026-03-07-ai-agents-breaking-observability-budget/view)
- [Anthropic — Demystifying evals for AI agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)
- [Langfuse K8s self-host issue #7591](https://github.com/langfuse/langfuse/issues/7591)
- [LangChain — AI agent observability](https://www.langchain.com/articles/agent-observability)
- [Maxim — Top 5 agent observability platforms 2026](https://www.getmaxim.ai/articles/top-5-agent-observability-platforms-in-2026/)
- [Digital Applied — LLM agent cost attribution](https://www.digitalapplied.com/blog/llm-agent-cost-attribution-guide-production-2026)
- [DuckDB vs SQLite for analytics](https://marending.dev/notes/sqlite-vs-duckdb/)
- [Mem0 — State of AI agent memory 2026](https://mem0.ai/blog/state-of-ai-agent-memory-2026)
- [Claude Code + Langfuse self-host template](https://github.com/doneyli/claude-code-langfuse-template)
- [Pydantic AI docs](https://ai.pydantic.dev/)
