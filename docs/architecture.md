# Architecture

This document walks through how AgentPulse works end-to-end — from a span leaving your agent to a toast firing in the browser.

Audience: someone who wants to contribute, debug a weird behaviour, or decide whether AgentPulse's shape fits their use case before committing to it.

---

## 10,000-foot view

```
Your agents ─ OTLP ─▶ Collector ─▶ ClickHouse ─┐
                         │                     ├─▶ Backend API ─▶ Next.js UI
                         └──────▶ Postgres ────┘          │
                                                         ├─ Eval worker (bg)
                                                         ├─ Alert evaluator (bg, 60s)
                                                         └─ Retention enforcer (bg)
```

Three storage substrates, three long-running Go processes, and one Next.js app. Every arrow is either OTLP (gRPC/HTTP), raw SQL, HTTP, WebSocket, or SSE — no message broker, no Kafka, no Redis.

---

## Why two databases

AgentPulse deliberately splits storage into ClickHouse and Postgres.

**ClickHouse** is a columnar OLAP store. It is used for everything that grows linearly with agent traffic:

- `spans` — one row per OTel span; columns include `project_id`, `run_id`, `session_id`, `user_id`, `span_kind`, `agent_name`, `tool_name`, token counts, cost in USD, timestamp, duration, latency, payload ref.
- `metrics_agg` — materialized aggregates (cost/latency/token rollups per project × hour).
- `run_metrics` / `session_agg` / `user_agg` — pre-aggregated views for dashboard charts.
- `span_evals` — eval scores produced by the worker.
- Full-text and semantic search indexes.

ClickHouse is the only storage that sees writes at span-scale — the collector pushes batches every few seconds.

**Postgres** is for *operational* state — data that is small but frequently updated, requires strong consistency, and benefits from transactions and joins:

- `projects`, `project_ingest_tokens` (hashed Bearer tokens)
- `topology_nodes`, `topology_edges` — the agent DAG graph
- `budget_rules`, `budget_alerts`, `alert_rules`, `alert_events`
- `project_eval_configs` — which judges run for which project
- `run_tags`, `run_annotations`, `span_feedback` — human-entered metadata
- `prompt_playground_versions` — edited prompts for A/B replay
- `retention_policies`, `purge_jobs`
- `push_subscriptions` — VAPID web-push recipients

This split lets ClickHouse do what ClickHouse does (scan 10M spans for a dashboard in <200 ms) without paying OLAP latency for a three-row settings page.

**S3 / MinIO** holds raw span payloads larger than 8 KB — LLM prompts, tool outputs, generation responses. ClickHouse stores only the `payload_ref` (`bucket/object`). MinIO lifecycle rules expire objects after 35 days by default.

---

## The collector pipeline

`collector/cmd/collector/main.go` wires a stock OpenTelemetry Collector with AgentPulse-specific components. The default pipeline:

```
otlp (receiver :4317/:4318)
  → batch (receiver-side buffer)
  → agentsemanticproc
  → piimaskerproc (if project has a PII config row)
  → budgetenforceproc
  → fan-out:
      • clickhouseexporter
      • topologyexporter
```

### `agentsemanticproc` ([`collector/processor/agentsemanticproc/`](../collector/processor/agentsemanticproc))

The single most important processor. It takes a raw OTel span and gives it AgentPulse semantics.

1. **Classification.** Looks at `agentpulse.span.kind` if present, otherwise infers from `gen_ai.*` attributes and span name:
   - `gen_ai.system` + `gen_ai.request.model` → `llm.call`
   - span name matching a tool convention + `agentpulse.tool.name` → `tool.call`
   - `agentpulse.handoff.target` → `agent.handoff`
   - `agentpulse.memory.key` + direction → `memory.read` / `memory.write`
   - New in 2026: MCP tool calls become `mcp.tool.call` when `mcp.tool.name` is present
2. **Cost computation.** For `llm.call` spans, looks up the model in `config/model_pricing.yaml`, multiplies by input/output tokens, and stamps `agentpulse.cost.usd`. Cached-input and thinking tokens get their own multipliers where the pricing row supports it.
3. **Session and user stamping.** If `agentpulse.session.id` or `agentpulse.user.id` are present on the parent span, they propagate to children.
4. **Run metadata extraction.** Extracts `agentpulse.run.id`, validates it's a UUID, and tags the span.

### `piimaskerproc` ([`collector/processor/piimaskerproc/`](../collector/processor/piimaskerproc))

Opt-in: looks up `project_pii_configs` in Postgres on a 30s refresh. If a regex list is configured for the project, it runs each pattern over `gen_ai.prompt`, `gen_ai.completion`, and tool payload attributes, replacing matches with `<redacted:label>`. A hash of the original is stamped so you can still deduplicate.

### `budgetenforceproc` ([`collector/processor/budgetenforceproc/`](../collector/processor/budgetenforceproc))

Holds an in-memory cache of `(project_id, run_id) → cumulative_cost_usd`, `(project_id, agent_name, run_id)`, and `(project_id, user_id) → cost_per_minute`. On every span:

1. Increment the relevant counters by `agentpulse.cost.usd`.
2. Compare against the rules cache (refreshed from Postgres every 30 s).
3. If a threshold is crossed and the rule's action is `halt`, stamp `agentpulse.budget.halted=true` on subsequent spans for that run and enqueue an alert.
4. If action is `notify`, enqueue an alert but do not stamp.

Alerts go to `budget_alerts` in Postgres; the backend picks them up for fan-out (WebSocket, Slack, Discord, webhook).

### `clickhouseexporter` ([`collector/exporter/clickhouseexporter/`](../collector/exporter/clickhouseexporter))

Batches spans and INSERTs them with the ClickHouse native protocol. On payload larger than 8 KB, it first offloads to S3 and stores only `payload_ref`.

### `topologyexporter` ([`collector/exporter/topologyexporter/`](../collector/exporter/topologyexporter))

Per run, it buffers spans until the run's root completes, then constructs:

- A node for each distinct `agent.name` seen in the run
- An edge for each `agent.handoff` span (source → target)
- An edge for each parent→child across agents (implicit handoff via span tree)

These are UPSERTed into `topology_nodes` and `topology_edges` keyed by `(project_id, run_id)`. React Flow in the UI renders directly off those rows.

---

## The backend API

`backend/cmd/server/main.go` wires a Chi router with:

- **Handler layer** — `backend/internal/api/handler/*` (one file per resource: `runs.go`, `sessions.go`, `budget.go`, `evals.go`, `alerts.go`, `analytics.go`, `playground.go`, `search.go`, `export.go`, `storage.go`, `notifications.go`, `ingest_tokens.go`, `spans.go`, `ws_alerts.go`, `sse_spans.go`).
- **Store layer** — `backend/internal/store/*` with separate modules for ClickHouse (`chstore/`), Postgres (`pgstore/`), and S3 (`s3store/`). Each is a small interface implemented by a struct that holds a connection pool.
- **Domain types** — `backend/internal/domain/` contains the pure Go types passed between handler and store.
- **Auth** — `backend/internal/authutil/` enforces project-scoped Bearer tokens. Admin mutations (settings, ingest tokens) require a separate `AdminKeyAuth` header check.
- **HTTP util** — `backend/internal/httputil/` for response envelopes (`{data, error, pagination}`), CORS, rate limiting (token bucket per project), and structured error responses.

### Background workers

Three goroutines start with the API server and run for the life of the process:

1. **Eval worker** (`backend/internal/eval/worker/`). Pulls pending eval jobs from Postgres, fetches the span + prompt/completion from ClickHouse (and payload from S3 if offloaded), dispatches to the judge model via `llmclient`, stamps the score into `span_evals` on ClickHouse.
2. **Alert evaluator** (`backend/internal/alerteval/`). Every 60 seconds, for each active signal rule, runs a ClickHouse aggregation query (e.g., "error rate in last 5 min"), compares against the rule, emits an `alert_event` if threshold is crossed. Then fans out to configured notification channels.
3. **Retention enforcer** (`backend/internal/storage/`). Walks `retention_policies` hourly; issues `DELETE FROM spans WHERE project_id = ? AND timestamp < ?` on ClickHouse and similar on Postgres tables.

---

## Real-time paths

AgentPulse uses two realtime mechanisms, deliberately separate:

- **WebSocket — `/api/v1/ws/alerts?project_id=...`.** Broadcasts alert events (budget + signal) to the UI. A handful of connections per project, long-lived, noisy only when something is actually firing.
- **Server-Sent Events — `/api/v1/projects/:projectId/spans/stream`.** Streams the most recent spans (via ClickHouse changelog polling every few hundred milliseconds). Used by the live-tail view on the project page and the "streaming run" view on the run detail page. SSE is one-way and rides ordinary HTTP/2 so it works through any proxy.

Neither path requires a Redis pubsub or a message broker; each handler reads directly from ClickHouse/Postgres.

---

## Evals

End-to-end flow of a single eval score:

```
1. Span arrives at ClickHouse with gen_ai.prompt + gen_ai.completion.
2. EvalEnqueuer (polls ClickHouse every 10s) finds spans lacking a score
   for the configured judge types in project_eval_configs.
3. It inserts rows into `eval_jobs` (Postgres): (span_id, judge_type, judge_model, status=pending).
4. EvalWorker dequeues, calls the judge model with a templated prompt:
   e.g., for hallucination —
     "Given this context: <ctx>. Given this answer: <ans>. Rate factuality 0..1."
   Multi-model consensus runs all configured judge_models for a judge_type
   and averages.
5. On response, worker inserts into `span_evals` on ClickHouse:
   (span_id, judge_type, judge_model, score, rationale, cost_usd, timestamp).
6. UI reads `span_evals` via /api/v1/runs/:runId/evals for per-run view,
   and /api/v1/projects/:projectId/evals/summary for the trend chart.
```

Custom judges are first-class: the project eval config can specify a `custom` judge with a user-supplied prompt template. The worker substitutes `{{prompt}}`, `{{completion}}`, `{{context}}` and calls the same `llmclient`.

---

## Loop detection

Two independent detectors, both running in the collector's goroutine context, both writing to `run_loops` in Postgres:

1. **Repeated-tool detector.** If the same `(agent_name, tool_name)` pair is called ≥ N times (default 5) in a sliding window (default 60 s), emit a `repeated_tool` loop event.
2. **Topology cycle detector.** On each handoff span, check whether the induced directed graph for this run has a cycle. If so, emit a `topology_cycle` loop event.

The UI renders a `LoopBanner` on the run detail page; the alert evaluator can fire on "N loops in the last hour."

---

## Ingest authentication

There are two auth surfaces:

- **Backend REST API** — requires `Authorization: Bearer <api_key>`. The token's SHA-256 is looked up in `project_ingest_tokens`; the matched row gives `project_id` and scopes. Run-scoped routes (`/api/v1/runs/:runId/...`) do an extra "does this project own this run?" check in ClickHouse before responding. This was audited and fixed in commit `323a6ec` (IDOR on run-scoped routes).
- **Collector OTel receiver** — by default, **no** authentication. On a laptop or private network this is fine; on the open internet it is not. Projects are identified purely by the `agentpulse.project.id` resource attribute on spans. A planned improvement is to require a Bearer token at the collector gRPC/HTTP interceptor and look it up against `project_ingest_tokens` — see [recommendations.md](./recommendations.md#p0-2-mcp-native-observability-plus-auth-tightening).

---

## Typical span's round trip

To make all the above concrete, here's what happens when a Python agent emits one `llm.call` span:

1. **Your code** runs `with pulse.llm(model="claude-sonnet-4-6"): ...`. The SDK wraps `anthropic.messages.create()` and records usage.
2. **OTel SDK** batches the span in-process, flushes every second (or on 512-span boundary).
3. **Collector receiver** (`:4317`) decodes the OTLP protobuf. Span enters the pipeline.
4. **`agentsemanticproc`** sees `gen_ai.system=anthropic` + `gen_ai.request.model=claude-sonnet-4-6`, tags `agentpulse.span.kind=llm.call`, computes cost using the pricing table, stamps `agentpulse.cost.usd=0.00321`.
5. **`budgetenforceproc`** bumps the counter for `(project_id, run_id)`. If it's now >= any matching rule's threshold, action fires.
6. **`clickhouseexporter`** batches with other spans and INSERTs. If the prompt string is >8 KB, the batcher first POSTs the payload to MinIO and replaces `gen_ai.prompt` with `payload_ref`.
7. **`topologyexporter`** tags `topology_nodes` with the agent if new; waits for the run's root span to close before writing edges.
8. **Backend** (polled every 10 s by the eval enqueuer) finds the span missing a `relevance` score, inserts an `eval_jobs` row.
9. **Eval worker** dequeues, calls Claude Haiku as judge, inserts score into `span_evals`.
10. **UI** — if a browser has the run detail page open, it already received this span via SSE; otherwise it shows up on next refresh. The eval score appears once the worker finishes (usually 1-3 s later).
11. **Alert evaluator** next cycle (60 s window): if average eval score in the last 5 min dropped below a configured rule, fire a `quality_alert`.
12. **Notifications** — alert event triggers a WebSocket toast for connected browsers, a Slack post, and a Discord webhook. Email digest aggregates alerts for the daily summary.

That is the full critical path.

---

## Configuration files

| File | Purpose | Hot reload? |
|---|---|---|
| `config/agent_attributes.yaml` | Extraction rules mapping raw OTel attributes to AgentPulse semantic attributes. Used by `agentsemanticproc`. | No — collector restart |
| `config/model_pricing.yaml` | Per-model `input_cost_per_million`, `output_cost_per_million`, optional `cached_input`, `thinking`. Used by `agentsemanticproc` and the playground. | Yes for backend (watched); no for collector (baked at build) |
| `collector/config.yaml` / `collector/config.dev.yaml` | OTel collector pipeline config (receivers, processors, exporters). | No |
| `.env` | Runtime env vars (DB DSNs, LLM API keys, VAPID keys, Resend key, CORS origins). Loaded by the backend `config.Load()` in `backend/internal/config/config.go`. | No |

Budget rules, signal alert rules, eval configs, retention policies, PII configs, and ingest tokens are **not** configuration files — they live in Postgres and are mutated via the REST API / UI without restart.

---

## Where to read the code first

If you want to understand AgentPulse by reading it, I'd go in this order:

1. `backend/internal/domain/` — the core types. Short, no dependencies.
2. `collector/processor/agentsemanticproc/processor.go` — the one place span classification + cost happens.
3. `backend/internal/store/chstore/spans.go` — the main ClickHouse query used by the UI.
4. `backend/internal/api/handler/runs.go` — the shape of a typical REST handler.
5. `web/src/app/projects/[projectId]/page.tsx` — how the frontend composes calls.
6. `backend/internal/alerteval/evaluator.go` — the 60 s signal loop.
7. `backend/internal/eval/worker/worker.go` — how LLM-as-judge actually runs.

Each one is < 300 lines and stands alone.

---

## Scaling notes

This section is forward-looking; AgentPulse today targets solo / small-team self-host, not multi-million-span-per-day enterprise.

- **ClickHouse** is the only horizontally-scalable piece out of the box. A single node handles ~10k spans/second on modest hardware. Partition by `project_id, toYYYYMM(timestamp)` if you need to shard.
- **Postgres** holds operational state only; it stays small (~100 MB for 100 projects × 1 year of alerts).
- **Collector** is stateless except for the budget-counter cache. To scale horizontally, run N collectors behind a load balancer and accept that budget counters become per-instance — either accept fuzzy thresholds or move counters to Redis (not shipped).
- **Backend** is stateless; scale horizontally freely.
- **Eval worker** is currently in-process with the backend. If it becomes a bottleneck, split it into its own binary (consuming the same `eval_jobs` table) — the code is already isolated behind a `RunOnce()` entrypoint.

---

## See also

- [getting-started.md](./getting-started.md) — get a working stack up
- [feasibility-analysis.md](./feasibility-analysis.md) — is this right for you?
- [recommendations.md](./recommendations.md) — where the product is going
- [roadmap.md](./roadmap.md) — shipped / in-flight / planned
