# AgentPulse

**"Datadog for AI Agents"** — unified observability platform for multi-agent AI systems.

AgentPulse solves the fragmentation problem: teams today use three separate tools (Helicone for cost, Langfuse for traces, Braintrust for evals) with no correlation between them. AgentPulse unifies all three into a single platform built on OpenTelemetry.

---

## What makes it different

| Feature | AgentPulse | Competitors |
|---|---|---|
| **Topology view** | DAG graph (actual execution flow) | Gantt/waterfall |
| **Framework-agnostic** | OTel-native, works with any SDK | Vendor lock-in |
| **Unified view** | Cost + latency + quality in one run view | Separate tools |
| **Budget enforcement** | Mid-run cost alerts and halt signals | Post-hoc only |
| **Session tracking** | Groups runs into conversations with timeline | Not available |
| **Loop detection** | Two-tier repeated-tool + topology cycle detection | Not available |

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Layer 3: Intelligence                          │
│  Budget enforcement · Cost alerts · SLOs        │
├─────────────────────────────────────────────────┤
│  Layer 2: Storage + Query                       │
│  ClickHouse (traces + metrics + sessions)       │
│  Postgres (topology + budget + alerts + loops)  │
├─────────────────────────────────────────────────┤
│  Layer 1: Collection (OTel-native)              │
│  OTel Collector with agent semantic extensions  │
│  Understands: llm.call · tool.call ·            │
│  agent.handoff · memory.read · memory.write     │
└─────────────────────────────────────────────────┘
```

```
Your agents
    │  OTLP (gRPC :4317 / HTTP :4318)
    ▼
┌──────────────────────────────────┐
│ Collector                        │
│  batch → agentsemanticproc       │  ← classifies spans, computes cost
│       → budgetenforceproc        │  ← checks budget rules, fires alerts
│       → clickhouseexporter       │  ← stores spans + metrics
│       → topologyexporter         │  ← builds DAG graph in Postgres
└──────────────────────────────────┘
    │                    │
    ▼                    ▼
ClickHouse           Postgres
(spans/metrics)    (topology/budgets)
    │                    │
    └────────┬───────────┘
             ▼
        Backend API  :8080
             │
             ▼
        Next.js UI  :3000
```

---

## Tech stack

| Component | Technology |
|---|---|
| Collector | Go · OpenTelemetry Collector (custom processors + exporters) |
| Backend API | Go · Chi router · pgx · clickhouse-go |
| Frontend | Next.js 15 · TypeScript · Tailwind CSS · Recharts · React Flow |
| Traces + metrics + sessions | ClickHouse 24.8 |
| Topology + budgets + alerts | Postgres 16 |
| Python SDK | opentelemetry-sdk wrapper + LangChain integration |
| TypeScript SDK | OTel JS SDK wrapper + Vercel AI SDK + OpenAI JS auto-instrumentation |

---

## Initial setup

### Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop) — for ClickHouse, Postgres, MinIO
- Go 1.22+ — `brew install go`
- Node.js 20+ — `brew install node` or use [nvm](https://github.com/nvm-sh/nvm)

### 1. Clone and install

```bash
git clone https://github.com/agentpulse/agentpulse.git
cd agentpulse
make web-install   # installs Next.js dependencies
```

### 2. Start infrastructure

```bash
make dev-up
```

This starts ClickHouse (`:9000`), Postgres (`:5432`), and MinIO (`:9090`).
Wait for the "Infrastructure ready." message.

### 3. Apply migrations

```bash
make migrate-up
```

Creates:
- Postgres: `projects`, `topology_nodes`, `topology_edges`, `budget_rules`, `budget_alerts`
- ClickHouse: `spans` table, `metrics_agg` aggregating view, `run_metrics` query view

### 4. Create a project

```bash
curl -s -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name":"my-project"}' | jq .
```

Response:
```json
{
  "data": {
    "project": {
      "ID": "a1b2c3d4-...",
      "Name": "my-project"
    },
    "api_key": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }
}
```

> Save the `ID` — you'll use it as `agentpulse.project.id` in your spans. The `api_key` is shown once only.

---

## Running locally

Start all four services in separate terminals:

**Terminal 1 — Collector**
```bash
make collector-run
# OTel gRPC on :4317, HTTP on :4318, health on :13133
```

**Terminal 2 — Backend API**
```bash
make backend-run
# REST API on :8080
```

**Terminal 3 — Frontend**
```bash
make web-dev
# UI on http://localhost:3000
```

---

## Sending synthetic traces (smoke test)

With all three services running, send synthetic multi-agent traces:

```bash
# Replace with your project ID from the setup step
go run ./tools/tracegen/... \
  --project-id a1b2c3d4-xxxx-xxxx-xxxx-xxxxxxxxxxxx \
  --scenario all \
  --runs 5
```

Then open [http://localhost:3000](http://localhost:3000).

Available scenarios:
| Scenario | Description |
|---|---|
| `multi-agent-research` | Orchestrator → researcher (web search + summarize) → critic → final answer. Exercises all 5 span kinds. |
| `simple-llm` | Single agent, one tool call + LLM response. |
| `parallel-tools` | Planner fires two tool calls then synthesizes. |
| `all` | Runs all three in each loop iteration. |

Additional flags:
```bash
go run ./tools/tracegen/... --help

  --endpoint   string   OTLP gRPC endpoint (default "localhost:4317")
  --runs       int      Number of trace runs to emit (default 3)
  --scenario   string   Scenario to run (default "multi-agent-research")
  --project-id string   Project ID to embed in spans (default "demo-project")
  --delay      duration Delay between runs (default 500ms)
```

---

## Instrumenting your own agents

### Python (Anthropic SDK / any framework)

Install the OTel SDK:
```bash
pip install opentelemetry-sdk opentelemetry-exporter-otlp-proto-grpc
```

One-time setup (call once at startup):
```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

resource = Resource({"service.name": "my-agent"})
provider = TracerProvider(resource=resource)
provider.add_span_processor(
    BatchSpanProcessor(
        OTLPSpanExporter(endpoint="http://localhost:4317", insecure=True)
    )
)
trace.set_tracer_provider(provider)
tracer = trace.get_tracer("my-agent")
```

Wrap each agent operation:
```python
import uuid

PROJECT_ID = "a1b2c3d4-xxxx-xxxx-xxxx-xxxxxxxxxxxx"  # from setup
RUN_ID = str(uuid.uuid4())   # one per agent run

# LLM call
with tracer.start_as_current_span("my_llm_call") as span:
    span.set_attribute("agentpulse.project.id", PROJECT_ID)
    span.set_attribute("agentpulse.run.id", RUN_ID)
    span.set_attribute("agentpulse.agent.name", "researcher")
    span.set_attribute("agentpulse.span.kind", "llm.call")
    span.set_attribute("gen_ai.system", "anthropic")
    span.set_attribute("gen_ai.request.model", "claude-sonnet-4-6")
    span.set_attribute("gen_ai.usage.input_tokens", 1024)
    span.set_attribute("gen_ai.usage.output_tokens", 256)

    response = anthropic_client.messages.create(...)   # your actual call

# Tool call
with tracer.start_as_current_span("web_search") as span:
    span.set_attribute("agentpulse.project.id", PROJECT_ID)
    span.set_attribute("agentpulse.run.id", RUN_ID)
    span.set_attribute("agentpulse.agent.name", "researcher")
    span.set_attribute("agentpulse.span.kind", "tool.call")
    span.set_attribute("agentpulse.tool.name", "web_search")

    result = search(query)

# Agent handoff
with tracer.start_as_current_span("handoff_to_critic") as span:
    span.set_attribute("agentpulse.project.id", PROJECT_ID)
    span.set_attribute("agentpulse.run.id", RUN_ID)
    span.set_attribute("agentpulse.agent.name", "orchestrator")
    span.set_attribute("agentpulse.span.kind", "agent.handoff")
    span.set_attribute("agentpulse.handoff.target", "critic")

# Flush before process exit
provider.shutdown()
```

### Key span attributes

| Attribute | Required | Values |
|---|---|---|
| `agentpulse.project.id` | ✅ | Your project UUID |
| `agentpulse.run.id` | ✅ | Unique ID per agent run (`uuid.uuid4()`) |
| `agentpulse.span.kind` | ✅ | `llm.call` · `tool.call` · `agent.handoff` · `memory.read` · `memory.write` |
| `agentpulse.agent.name` | recommended | Identifies which agent emitted this span |
| `gen_ai.system` | recommended | `anthropic` · `openai` · `google` |
| `gen_ai.request.model` | recommended | `claude-sonnet-4-6` · `gpt-4o` etc |
| `gen_ai.usage.input_tokens` | for cost | integer |
| `gen_ai.usage.output_tokens` | for cost | integer |
| `agentpulse.tool.name` | tool spans | tool identifier |
| `agentpulse.handoff.target` | handoff spans | target agent name |
| `agentpulse.memory.key` | memory spans | memory key |

Cost is computed automatically from `config/model_pricing.yaml` — no need to set it manually.

### Go agents

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup (once)
exp, _ := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(),
    otlptracegrpc.WithEndpoint("localhost:4317"))
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
otel.SetTracerProvider(tp)
tracer := otel.Tracer("my-agent")

// Instrument
ctx, span := tracer.Start(ctx, "my_llm_call")
span.SetAttributes(
    attribute.String("agentpulse.project.id", projectID),
    attribute.String("agentpulse.run.id", runID),
    attribute.String("agentpulse.agent.name", "researcher"),
    attribute.String("agentpulse.span.kind", "llm.call"),
    attribute.String("gen_ai.system", "anthropic"),
    attribute.String("gen_ai.request.model", "claude-sonnet-4-6"),
    attribute.Int("gen_ai.usage.input_tokens", 1024),
    attribute.Int("gen_ai.usage.output_tokens", 256),
)
defer span.End()
```

---

## Budget alerts

Create a budget rule to get notified (or halt) when a run exceeds a cost threshold:

```bash
curl -X POST http://localhost:8080/api/v1/projects/{projectId}/budget/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "per-run $0.10 alert",
    "threshold_usd": 0.10,
    "action": "notify",
    "scope": "run",
    "enabled": true
  }'
```

Actions:
- `notify` — writes an alert to `budget_alerts` and calls `webhook_url` if set
- `halt` — same as notify, plus stamps `agentpulse.budget.halted=true` on subsequent spans (your SDK wrapper can poll for this attribute to stop the run)

Scopes:
- `run` — cumulative cost for a single run
- `agent` — cumulative cost for a single agent within a run

Rules are refreshed from Postgres every 30 seconds without restarting the collector.

---

## API reference

```
GET    /healthz

GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/:id

GET    /api/v1/projects/:projectId/runs?limit=50&offset=0

GET    /api/v1/projects/:projectId/sessions
GET    /api/v1/projects/:projectId/sessions/:sessionId
GET    /api/v1/projects/:projectId/sessions/:sessionId/runs

GET    /api/v1/projects/:projectId/budget/rules
POST   /api/v1/projects/:projectId/budget/rules
PUT    /api/v1/projects/:projectId/budget/rules/:ruleId
DELETE /api/v1/projects/:projectId/budget/rules/:ruleId
GET    /api/v1/projects/:projectId/budget/alerts

GET    /api/v1/projects/:projectId/alerts/rules
POST   /api/v1/projects/:projectId/alerts/rules
PUT    /api/v1/projects/:projectId/alerts/rules/:ruleId
DELETE /api/v1/projects/:projectId/alerts/rules/:ruleId
GET    /api/v1/projects/:projectId/alerts/events

GET    /api/v1/projects/:projectId/analytics/tools?window=24h|7d
GET    /api/v1/projects/:projectId/analytics/agents?window=24h|7d

GET    /api/v1/projects/:projectId/evals/summary

GET    /api/v1/runs/:runId
GET    /api/v1/runs/:runId/spans
GET    /api/v1/runs/:runId/topology
GET    /api/v1/runs/:runId/evals
GET    /api/v1/runs/:runId/loops

WS     /api/v1/ws/alerts?project_id=:projectId
```

---

## Makefile reference

```bash
make dev-up          # start ClickHouse, Postgres, MinIO
make dev-down        # stop infrastructure
make dev-logs        # tail Docker logs

make migrate-up      # apply all migrations
make migrate-down    # rollback Postgres migrations

make collector-run   # run collector locally (requires dev-up)
make backend-run     # run API server (requires dev-up + migrate-up)
make web-dev         # start Next.js dev server

make seed            # send 5 synthetic runs (scenario=all, requires collector-run)
make test            # run all tests (collector + backend + web)
make lint            # run all linters
```

---

## Project structure

```
agentpulse/
├── collector/                  # OTel Collector (Go)
│   ├── cmd/collector/          # main + component wiring
│   ├── processor/
│   │   ├── agentsemanticproc/  # span classification + cost computation
│   │   └── budgetenforceproc/  # budget threshold enforcement
│   └── exporter/
│       ├── clickhouseexporter/ # spans → ClickHouse (with session_id)
│       └── topologyexporter/   # spans → Postgres DAG
├── backend/                    # REST API (Go)
│   └── internal/
│       ├── api/handler/        # Chi handlers (runs, sessions, budget, alerts, analytics, evals)
│       ├── store/              # ClickHouse + Postgres repositories
│       ├── domain/             # core types (Run, Session, AlertRule, ...)
│       ├── alerteval/          # 60s signal evaluator (error rate, latency, quality, loop)
│       └── loopdetect/         # two-tier loop + topology cycle detection
├── web/                        # Next.js 15 frontend
│   └── src/
│       ├── app/
│       │   └── projects/[projectId]/
│       │       ├── page.tsx            # tabs: Overview, Services, Budget, Alerts, Evals, Sessions
│       │       ├── runs/[runId]/       # run detail (spans, topology, evals, loops)
│       │       └── sessions/[sessionId]/  # session detail (metric cards, timeline, turns)
│       ├── components/
│       │   ├── runs/           # RunRow, RunList, LoopBanner
│       │   ├── sessions/       # SessionList, SessionTimeline, SessionBadge
│       │   ├── topology/       # React Flow DAG graph
│       │   ├── budget/         # budget rules + alert feed
│       │   ├── alerts/         # signal-based alert rules + history
│       │   ├── analytics/      # tool reliability + agent cost breakdown
│       │   └── evals/          # eval summary + trend chart
│       └── lib/                # API client + TypeScript types
├── sdk/
│   ├── python/                 # Python SDK (OTel wrapper + LangChain integration)
│   └── typescript/             # TypeScript SDK (OTel JS + Vercel AI + OpenAI wrappers)
├── eval/
│   ├── enqueuer/               # polls ClickHouse, enqueues eval jobs
│   └── worker/                 # Claude Haiku judge; scores relevance 0–1
├── migrations/
│   ├── clickhouse/             # spans, metrics_agg, run_metrics, span_evals, session_agg
│   └── postgres/               # projects, topology, budget, alerts, loops
├── config/
│   ├── agent_attributes.yaml   # OTel attribute extraction rules
│   └── model_pricing.yaml      # per-token cost table (all major models)
└── tools/
    └── tracegen/               # synthetic trace generator (5 demo projects + sessions)
```

---

## Development

### Running tests

```bash
make test
# or individually:
make test-collector   # Go unit tests (no DB required)
make test-backend     # Go unit + integration tests
make test-web         # Next.js tests
```

### Adding a new model to pricing

Edit `config/model_pricing.yaml` — no code changes needed:

```yaml
models:
  - id: "my-new-model"
    input_cost_per_million: 3.00
    output_cost_per_million: 15.00
```

### Extending span attributes

Edit `config/agent_attributes.yaml` to add new OTel GenAI SIG attributes as they are ratified — again, no code changes needed. The collector reloads on restart.
