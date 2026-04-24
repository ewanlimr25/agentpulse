# AgentPulse

**Self-hosted observability for AI agent workflows.**

AgentPulse unifies traces, cost, evals, and alerts for multi-agent systems into a single OpenTelemetry-native platform. Built for developers who want to see what their agents are doing — without stitching Helicone + Langfuse + Braintrust together, and without shipping their prompts to a third-party SaaS.

> **Project status — April 2026:** feature-rich but pre-1.0. The core collection/storage/query path is solid and well-tested; several UI surfaces and operational affordances are incomplete. See [What's missing](#whats-missing) before deploying.

---

## Table of contents

- [Why AgentPulse](#why-agentpulse)
- [What works today](#what-works-today)
- [What's missing](#whats-missing)
- [Architecture](#architecture)
- [Quick start](#quick-start)
- [Sending your first trace](#sending-your-first-trace)
- [Instrumenting real agents](#instrumenting-real-agents)
- [CLI](#cli)
- [Claude Code integration](#claude-code-integration)
- [API reference](#api-reference)
- [Makefile reference](#makefile-reference)
- [Project layout](#project-layout)
- [Further reading](#further-reading)

---

## Why AgentPulse

| | AgentPulse | Helicone | Langfuse | Braintrust |
|---|---|---|---|---|
| OTel-native collection | ✅ | ❌ (proxy) | Partial | Partial |
| Topology / DAG view | ✅ React Flow | ❌ | Waterfall | Waterfall |
| Unified cost + latency + quality | ✅ | cost only | ✅ | eval-first |
| Mid-run budget enforcement | ✅ halt or notify | post-hoc | post-hoc | ❌ |
| Multi-signal alerting | ✅ error/latency/quality/loop | ❌ | Partial | Partial |
| Self-hostable | ✅ Go + Docker | ✅ | ✅ heavy | ❌ |
| Claude Code hook integration | ✅ zero-config | ❌ | community | ❌ |
| License | Pre-release | MIT | MIT | commercial |

Every major feature is optional and runs in the same process tree — no Kafka, no Redis, no separate worker cluster.

---

## What works today

These have merged implementations across collector + backend + UI, backed by either unit tests, integration tests, or a demo workflow.

### Collection
- OTel receiver on `:4317` (gRPC) and `:4318` (HTTP)
- **`agentsemanticproc`** — classifies spans into `llm.call`, `tool.call`, `agent.handoff`, `memory.read`, `memory.write`; computes cost from [`config/model_pricing.yaml`](config/model_pricing.yaml)
- **`budgetenforceproc`** — refreshes rules from Postgres every 30s, halts or notifies when a run crosses threshold
- **`piimaskerproc`** — regex-based redaction, opt-in per project
- **`clickhouseexporter`** — writes spans, metrics, session and user aggregates
- **`topologyexporter`** — builds the agent DAG in Postgres
- **Streaming span support** — TTFT and generation throughput for streamed LLM responses
- **Payload offloading** — spans with payloads >8 KB get offloaded to S3/MinIO with a `payload_ref`
- **Framework auto-instrumentation** in the SDKs for CrewAI, AutoGen/AG2, LlamaIndex, OpenAI Agents SDK, LangChain, Vercel AI SDK, OpenAI JS, and MCP tool calls

### Storage + query
- **ClickHouse** — spans, metrics aggregations, session aggregates, user/customer cost aggregates, search indexes
- **Postgres** — projects, topology graph, budget rules and alerts, signal alert rules, run loops, eval configs, ingest tokens, retention policies, playground versions, run tags/annotations, PII configs, span feedback
- **Semantic + full-text search** over spans
- **Run comparison** — side-by-side diff of two runs' spans and prompts
- **Deterministic replay** — download a replay bundle for a run and re-execute locally

### Evals
- **LLM-as-judge** covering relevance, hallucination, faithfulness, toxicity, tool-correctness, and custom judges
- Multi-model consensus (score the same span with multiple judge models)
- Per-project eval config + trend chart
- **Quality gates** — `agentpulse-cli eval check` for CI pipelines ([docs/quality-gates.md](docs/quality-gates.md))

### Alerts + notifications
- **Budget alerts** — run-scope, agent-scope, or user-scope cost thresholds
- **Signal alerts** — error rate, p95 latency, quality score, tool failure, loop detection (60-second evaluator)
- **Loop detection** — two-tier (repeated-tool N times + topology cycle)
- **Notification channels** — Slack, Discord, browser push (VAPID), email digest (Resend), webhook
- **Real-time toasts** over WebSocket

### Sessions + users
- Sessions group runs into multi-turn conversations with a timeline UI
- Per-user / per-customer cost attribution (tag spans with `agentpulse.user.id`)

### Frontend
- Project dashboard with cost/latency/error/token charts
- Runs list with pagination, filter, tags, annotations
- Run detail: span drawer, topology graph, evals, loops, replay, tag editor
- Sessions and session detail pages
- Evals dashboard with trend chart and per-type breakdown
- Alerts and budget rule management
- **Prompt playground** — edit a span's prompt and re-send it with A/B variants, model picker, side-by-side cost comparison
- **Prompt version diff** across runs
- **Model cost comparison** dashboard
- **SSE live span tail** and health indicator
- **CSV/JSONL export** for spans, runs, analytics
- Sidebar navigation (two-tier, collapsible)

### CLI (`backend/agentpulse-cli`)
- `runs list` / `runs tail` / `runs tag` / `runs annotate`
- `status` — collector health check
- `eval check` — CI quality gate
- `replay <run-id>` — download replay bundle

### Integrations
- **Claude Code hook** — drop-in `SessionStart`/`Stop` hooks that emit OTel spans per tool call with zero config ([docs/claude-code-integration.md](docs/claude-code-integration.md))
- **Python SDK** (`sdk/python`) — framework auto-instrumentors for CrewAI, AutoGen, LlamaIndex, LangChain
- **TypeScript SDK** (`sdk/typescript`) — wrappers for Vercel AI SDK, OpenAI JS, LangChain with attribute codegen
- **MCP** instrumentation for Model Context Protocol tool invocations

### Operations
- Per-project **retention policies** with background enforcer
- Usage stats endpoint + manual purge
- Per-project **ingest tokens** (Bearer) with rotation
- Rate limiting (global + per-route)
- Health endpoint at `/healthz`

---

## What's missing

Be honest with yourself before cloning. These items are known gaps as of April 2026.

### Blocks a fresh clone from booting

- [ ] **Collector is not in the default `docker compose up`.** It's gated behind `--profile full`. `make collector-run` builds and runs it locally instead, which is the documented path.

### Incomplete UI surfaces (backend works, frontend doesn't)

- [ ] **Ingest token management UI** — endpoint exists, there's no "create/rotate/delete token" page
- [ ] **Retention policy UI** — API supports per-project retention, no settings page to change it
- [ ] **PII redaction UI** — regex config is stored per-project in Postgres, no UI to edit
- [ ] **Email digest config UI** — the worker runs, there's no form for frequency/recipients
- [ ] **First-run wizard** — after migrations, the UI shows a blank dashboard. You must `curl POST /api/v1/projects` to proceed.

### Safety / production-readiness gaps

- [ ] **No TLS on the backend** (plaintext HTTP only; Bearer tokens travel in the clear unless you front with nginx/Caddy/Cloudflare)
- [ ] **No multi-user / RBAC** — one API key per project; everyone with the key has full read/write
- [ ] **No audit logging**
- [ ] **Collector does not validate ingest tokens** — the backend REST endpoints require Bearer auth, but the OTel receiver accepts anything on `:4317`/`:4318`. On a trusted network this is fine; on the internet it is not.
- [ ] **Webhook notifications are unsigned** (no HMAC)
- [ ] **No secret-rotation tooling** — DB credentials and LLM API keys are env vars

### Testing + CI gaps

- [ ] **Backend has strong unit test coverage**, but there's no GitHub Actions workflow running it — only `sdk-typescript.yml` exists
- [ ] **No frontend tests** (`web/` has zero `*.test.tsx`)
- [ ] **`tests/e2e/` directory exists but is empty**

### Documentation gaps

- [ ] **No deployment guide** (Kubernetes / Fly.io / Railway / Cloud Run)
- [ ] **No OpenAPI / Swagger** — API reference is hand-maintained in this README
- [ ] **No troubleshooting guide** (ClickHouse full disk, Postgres pool exhaustion, collector lag)

### Things the roadmap calls out as missing

See [docs/roadmap.md](docs/roadmap.md). Notable items: MCP-native observability, trajectory evals, online (sampled production) evals, guardrails telemetry, single-binary indie mode, reasoning-trace analysis. These are discussed in depth in [docs/recommendations.md](docs/recommendations.md).

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│  L3 — Intelligence                                   │
│  Budget enforcement · multi-signal alerts · SLOs     │
│  LLM-as-judge evals · loop detection · quality gates │
├──────────────────────────────────────────────────────┤
│  L2 — Storage + Query                                │
│  ClickHouse  — spans, metrics, sessions, users       │
│  Postgres    — topology, budgets, alerts, configs    │
│  S3 / MinIO  — large-payload offload (>8 KB)         │
├──────────────────────────────────────────────────────┤
│  L1 — Collection (OpenTelemetry-native)              │
│  OTel Collector with agent semantic extensions:      │
│  llm.call · tool.call · agent.handoff ·              │
│  memory.read · memory.write · mcp.tool.call          │
└──────────────────────────────────────────────────────┘
```

```
Your agents
     │  OTLP (gRPC :4317 / HTTP :4318)
     ▼
┌──────────────────────────────────┐
│ Collector                        │
│  batch                           │
│   → agentsemanticproc            │  classify + cost
│   → piimaskerproc (opt-in)       │  redact
│   → budgetenforceproc            │  threshold check
│   → clickhouseexporter           │  spans + metrics
│   → topologyexporter             │  DAG graph
└──────────────────────────────────┘
     │                    │
     ▼                    ▼
 ClickHouse           Postgres
 (spans)              (topology / budgets / configs)
     │                    │
     └──────┬─────────────┘
            ▼
     Backend API :8080 (Chi)
     + Eval worker (background)
     + Alert evaluator (background, 60s)
     + Retention enforcer (background)
            │
            ▼
      Next.js UI :3000
      + WebSocket (realtime alerts)
      + SSE (live span tail)
```

For the full architectural walkthrough — span lifecycle, processor internals, eval worker, alert evaluator — see [docs/architecture.md](docs/architecture.md).

---

## Quick start

**Prerequisites.**

- [Docker Desktop](https://www.docker.com/products/docker-desktop) (for ClickHouse, Postgres, MinIO)
- Go 1.22+ (`brew install go`)
- Node.js 20+ (`brew install node` or [nvm](https://github.com/nvm-sh/nvm))
- Python 3.10+ (only if you plan to use the Python SDK)

**The short version.**

```bash
git clone https://github.com/agentpulse/agentpulse.git
cd agentpulse

cp .env.example .env             # optional — defaults work for local dev

make web-install
make dev-up                      # ClickHouse + Postgres + MinIO
make migrate-up                  # see caveat below
make migrate-up-missing          # apply 011/012/013 pg + 015/016 ch (if not in migrate-up yet)

# Three terminals, one command each:
make collector-run               # :4317 gRPC, :4318 HTTP
make backend-run                 # :8080 API
make web-dev                     # :3000 UI

# Create your first project:
curl -s -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name":"my-project"}' | jq .
```

> **Migration caveat:** as of this writing, `make migrate-up` is missing the three newest Postgres migrations and two newest ClickHouse migrations. Until that's fixed, apply them manually — see [docs/getting-started.md](docs/getting-started.md#applying-migrations-manually).

For a richer walkthrough with screenshots, context, and the first eval + budget rule, read [docs/getting-started.md](docs/getting-started.md).

---

## Sending your first trace

With collector + backend + web running, the fastest way to prove the stack is to shoot synthetic traces at the collector:

```bash
go run ./tools/tracegen/... \
  --project-id <your-project-id-from-post-projects> \
  --scenario all \
  --runs 5
```

| Scenario | What it emits |
|---|---|
| `multi-agent-research` | orchestrator → researcher (web-search + summarize) → critic → final answer. Exercises all 5 span kinds. |
| `simple-llm` | One agent, one tool call + one LLM response. |
| `parallel-tools` | Planner fires two tool calls then synthesizes. |
| `all` | Runs all three in each loop iteration. |

Then open [http://localhost:3000](http://localhost:3000).

---

## Instrumenting real agents

### Python (any framework)

```bash
pip install agentpulse-sdk
```

```python
from agentpulse import AgentPulse

pulse = AgentPulse(
    project_id="...",               # from POST /api/v1/projects
    endpoint="http://localhost:4317",
)

with pulse.run(name="research") as run:
    with run.agent("orchestrator"):
        with run.llm(model="claude-sonnet-4-6") as span:
            span.record_usage(input_tokens=1024, output_tokens=256)
            resp = anthropic_client.messages.create(...)

        with run.tool("web_search"):
            results = search(query)
```

Auto-instrumentors for CrewAI, AutoGen, LlamaIndex, LangChain live in `sdk/python/src/agentpulse/instrumentation/`. See [docs/sdk-getting-started.md](docs/sdk-getting-started.md).

### TypeScript / Vercel AI / OpenAI JS

```bash
npm install @agentpulse/sdk
```

```ts
import { AgentPulse } from "@agentpulse/sdk";

const pulse = new AgentPulse({ projectId: "...", endpoint: "http://localhost:4317" });

await pulse.run("research", async (run) => {
  await run.llm({ model: "gpt-4o" }, async (span) => {
    const res = await openai.chat.completions.create({ /* ... */ });
    span.recordUsage(res.usage);
    return res;
  });
});
```

See [sdk/typescript/README.md](sdk/typescript/README.md).

### Raw OTel (no SDK)

If you can't use the SDKs, emit spans with these attributes:

| Attribute | Required | Values |
|---|---|---|
| `agentpulse.project.id` | ✅ | Project UUID |
| `agentpulse.run.id` | ✅ | Unique per agent run |
| `agentpulse.span.kind` | ✅ | `llm.call` · `tool.call` · `agent.handoff` · `memory.read` · `memory.write` · `mcp.tool.call` |
| `agentpulse.agent.name` | recommended | Identifies emitting agent |
| `agentpulse.session.id` | for sessions | Groups runs into a conversation |
| `agentpulse.user.id` | for user attribution | Your end-user ID |
| `gen_ai.system` | recommended | `anthropic` · `openai` · `google` |
| `gen_ai.request.model` | recommended | Model name matching `config/model_pricing.yaml` |
| `gen_ai.usage.input_tokens` | for cost | integer |
| `gen_ai.usage.output_tokens` | for cost | integer |
| `agentpulse.tool.name` | tool spans | Tool identifier |
| `agentpulse.handoff.target` | handoff spans | Target agent name |

Cost is computed automatically from the pricing table — never set it manually.

---

## CLI

```bash
make cli-build                   # → tools/agentpulse-cli
make cli-install                 # → /usr/local/bin/agentpulse

export AGENTPULSE_API_URL=http://localhost:8080
export AGENTPULSE_API_KEY=<paste the api_key from your project>

agentpulse status                # collector/backend health
agentpulse runs list --limit 20
agentpulse runs tail             # live stream spans for a project
agentpulse runs tag add <run-id> broken tuesday
agentpulse runs annotate <run-id> --note "classic tool loop"
agentpulse replay <run-id>       # download replay bundle
agentpulse eval check --run <id> --min-score 0.8   # CI quality gate
```

CI integration recipe: [docs/quality-gates.md](docs/quality-gates.md).

---

## Claude Code integration

Observe every Claude Code session with zero code changes:

```bash
agentpulse hook install
```

This wires `SessionStart` / `PostToolUse` / `Stop` hooks in `~/.claude/settings.json` to emit OTel spans to your local collector. Runs and sessions appear in the UI automatically, one session per Claude Code invocation.

Full setup (token creation, verification, troubleshooting): [docs/claude-code-integration.md](docs/claude-code-integration.md).

---

## API reference

Grouped by resource. All `/api/v1/projects/:projectId/...` routes require the project Bearer token (`Authorization: Bearer <api_key>`).

```
GET    /healthz

# Projects
GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/:id

# Runs
GET    /api/v1/projects/:projectId/runs?limit=50&offset=0&tags=foo
GET    /api/v1/runs/:runId
GET    /api/v1/runs/:runId/spans
GET    /api/v1/runs/:runId/topology
GET    /api/v1/runs/:runId/evals
GET    /api/v1/runs/:runId/loops
GET    /api/v1/runs/:runId/replay
POST   /api/v1/runs/:runId/tags
DELETE /api/v1/runs/:runId/tags/:tag
PUT    /api/v1/runs/:runId/annotation

# Sessions
GET    /api/v1/projects/:projectId/sessions
GET    /api/v1/projects/:projectId/sessions/:sessionId
GET    /api/v1/projects/:projectId/sessions/:sessionId/runs

# Search
GET    /api/v1/projects/:projectId/search?q=...&mode=full_text|semantic

# Analytics
GET    /api/v1/projects/:projectId/analytics/tools?window=24h|7d
GET    /api/v1/projects/:projectId/analytics/agents?window=24h|7d
GET    /api/v1/projects/:projectId/analytics/users?window=24h|7d
GET    /api/v1/projects/:projectId/analytics/models?window=24h|7d

# Budget rules
GET    /api/v1/projects/:projectId/budget/rules
POST   /api/v1/projects/:projectId/budget/rules
PUT    /api/v1/projects/:projectId/budget/rules/:ruleId
DELETE /api/v1/projects/:projectId/budget/rules/:ruleId
GET    /api/v1/projects/:projectId/budget/alerts

# Signal alert rules
GET    /api/v1/projects/:projectId/alerts/rules
POST   /api/v1/projects/:projectId/alerts/rules
PUT    /api/v1/projects/:projectId/alerts/rules/:ruleId
DELETE /api/v1/projects/:projectId/alerts/rules/:ruleId
GET    /api/v1/projects/:projectId/alerts/events

# Evals
GET    /api/v1/projects/:projectId/evals/summary
GET    /api/v1/projects/:projectId/evals/config
PUT    /api/v1/projects/:projectId/evals/config

# Playground
POST   /api/v1/projects/:projectId/playground/run

# Ingest tokens
GET    /api/v1/projects/:projectId/ingest-tokens
POST   /api/v1/projects/:projectId/ingest-tokens
DELETE /api/v1/projects/:projectId/ingest-tokens/:tokenId

# Storage
GET    /api/v1/projects/:projectId/storage/usage
POST   /api/v1/projects/:projectId/storage/purge
GET    /api/v1/projects/:projectId/storage/retention
PUT    /api/v1/projects/:projectId/storage/retention

# Notifications
GET    /api/v1/projects/:projectId/notifications/subscriptions
POST   /api/v1/projects/:projectId/notifications/subscriptions

# Streaming
WS     /api/v1/ws/alerts?project_id=:projectId
GET    /api/v1/projects/:projectId/spans/stream            # SSE

# Export
GET    /api/v1/projects/:projectId/export/spans?format=csv|jsonl
GET    /api/v1/projects/:projectId/export/runs?format=csv|jsonl
```

---

## Makefile reference

```bash
make help            # prints this table

# Infrastructure
make dev-up          # start ClickHouse, Postgres, MinIO
make dev-down        # stop infrastructure
make dev-logs        # tail Docker logs

# Migrations
make migrate-up      # apply migrations (⚠ currently missing the newest 5 — see above)
make migrate-down    # rollback Postgres

# Services
make collector-build / collector-run
make backend-build   / backend-run
make web-install     / web-dev / web-build
make cli-build       / cli-install

# Seeding
make seed            # wipe app data + create demo projects + realistic runs
make seed-alert-history
make seed-evals

# SDKs
make sdk-ts-install / sdk-ts-codegen / sdk-ts-build

# Tests
make test            # collector + backend + web + sdk-ts
make test-collector
make test-backend
make test-web
make test-sdk-ts
make lint
```

---

## Project layout

```
agentpulse/
├── collector/                      # OTel Collector (Go)
│   ├── cmd/collector/              # main + component wiring
│   ├── processor/
│   │   ├── agentsemanticproc/      # span classification + cost
│   │   ├── budgetenforceproc/      # budget threshold enforcement
│   │   └── piimaskerproc/          # regex redaction
│   └── exporter/
│       ├── clickhouseexporter/     # spans → ClickHouse
│       └── topologyexporter/       # spans → Postgres DAG
├── backend/
│   ├── cmd/server/                 # API main
│   ├── agentpulse-cli/             # CLI (runs, tail, tag, replay, eval check, status)
│   └── internal/
│       ├── api/handler/            # Chi handlers
│       ├── store/                  # ClickHouse + Postgres repositories
│       ├── domain/                 # core types
│       ├── alert/ alerteval/       # alert rules + 60s signal evaluator
│       ├── eval/                   # eval enqueuer + worker + judges
│       ├── loopdetect/             # two-tier loop + cycle detection
│       ├── llmclient/              # unified LLM client (Anthropic/OpenAI/Google)
│       ├── pushnotify/             # VAPID web push
│       ├── emaildigest/            # Resend daily digest
│       ├── pricing/                # model cost computation
│       └── storage/                # retention + purge
├── web/                            # Next.js 15 frontend
│   └── src/
│       ├── app/
│       │   └── projects/[projectId]/
│       │       ├── page.tsx
│       │       ├── runs/[runId]/
│       │       ├── sessions/[sessionId]/
│       │       ├── evals/
│       │       ├── playground/
│       │       └── settings/
│       ├── components/
│       │   ├── runs/ sessions/ topology/ budget/ alerts/
│       │   ├── analytics/ evals/ playground/ search/
│       │   └── notifications/
│       └── lib/                    # API client + types
├── sdk/
│   ├── python/                     # OTel wrapper + framework instrumentors
│   └── typescript/                 # OTel JS + Vercel AI + OpenAI JS
├── migrations/
│   ├── postgres/                   # 001 → 013
│   └── clickhouse/                 # 001 → 016
├── config/
│   ├── agent_attributes.yaml       # OTel attribute extraction rules
│   └── model_pricing.yaml          # per-token cost table
├── tools/
│   └── tracegen/                   # synthetic trace generator
├── docs/
│   ├── getting-started.md
│   ├── architecture.md
│   ├── feasibility-analysis.md
│   ├── recommendations.md
│   ├── sdk-getting-started.md
│   ├── claude-code-integration.md
│   ├── quality-gates.md
│   └── roadmap.md
└── .env.example
```

---

## Further reading

- **[docs/getting-started.md](docs/getting-started.md)** — step-by-step first-run tutorial
- **[docs/architecture.md](docs/architecture.md)** — how the system works end-to-end
- **[docs/feasibility-analysis.md](docs/feasibility-analysis.md)** — can a solo developer actually run this?
- **[docs/recommendations.md](docs/recommendations.md)** — where AgentPulse sits in the 2026 observability landscape and what to build next
- **[docs/sdk-getting-started.md](docs/sdk-getting-started.md)** — Python SDK deep dive
- **[docs/claude-code-integration.md](docs/claude-code-integration.md)** — zero-config Claude Code hooks
- **[docs/quality-gates.md](docs/quality-gates.md)** — CI quality-gate CLI
- **[docs/roadmap.md](docs/roadmap.md)** — shipped / in-flight / planned

---

## Contributing

No `CONTRIBUTING.md` yet. Conventions that apply anyway:

- Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`) — see `git log` for examples
- Go code is formatted with `gofmt` / `goimports`; run `make lint` before pushing
- Keep migrations additive; never edit a migration file that has shipped
- When adding a new feature with a UI surface, update [docs/roadmap.md](docs/roadmap.md) and flag any "backend-only, UI pending" state in this README's [What's missing](#whats-missing) section

---

## License

AgentPulse is released under the [PolyForm Noncommercial License 1.0.0](LICENSE) — free to run, modify, and share for personal, educational, research, hobby, or any other noncommercial purpose. Selling it, offering it as a hosted service, or bundling it into a commercial product is not permitted without a separate commercial license; open an issue if you want to discuss one.
