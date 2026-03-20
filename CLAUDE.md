# AgentPulse

**"Datadog for AI Agents"** — unified observability platform for multi-agent AI systems.

## What We're Building

AgentPulse is an OpenTelemetry-native observability platform purpose-built for AI agent workloads. It solves the fragmentation problem in the current market: teams today use Helicone for cost, Langfuse for traces, and Braintrust for evals — three separate tools with no correlation. AgentPulse unifies all three into a single pane.

## Core Differentiators

1. **Multi-agent topology view** — directed graph visualization of agent execution (not a linear waterfall). Nodes = agents/tools, edges = handoffs/invocations, color = status. Every competitor shows a Gantt chart; we show the actual execution graph.
2. **OpenTelemetry-native** — framework-agnostic collector using OTel with agent-specific semantic extensions. We're the destination, not another SDK.
3. **Three pillars unified** — traces (execution flow), metrics (cost/tokens/latency), and eval quality scores on a single run view.
4. **Real-time budget enforcement** — cost alerting and hard caps that can halt or notify mid-run (nobody does this today).

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Layer 3: Intelligence (Eval + Alerts + SLOs)   │
│  - Quality scoring (hallucination, relevance)   │
│  - Budget enforcement (mid-run cost alerts)     │
│  - Agent SLOs (success rate, p95 latency)       │
├─────────────────────────────────────────────────┤
│  Layer 2: Unified Storage + Query               │
│  - ClickHouse for traces + metrics              │
│  - Graph DB for agent topology                  │
│  - S3 for raw log blobs                         │
├─────────────────────────────────────────────────┤
│  Layer 1: Collection (OTel-native)              │
│  - OTel collector with agent semantic exts      │
│  - Auto-instrumentation for major frameworks    │
│  - One-line SDK wrapper for custom agents       │
└─────────────────────────────────────────────────┘
```

## MVP Scope (v1)

1. OTel collector that understands agent spans: `llm.call`, `tool.call`, `agent.handoff`, `memory.read`, `memory.write`
2. Graph-based trace UI — render multi-agent execution as a DAG
3. Unified cost + latency + quality on a single run view
4. Budget cap alerting (notify or halt mid-run)

**Out of scope for v1:** eval platform, pre-production simulation, enterprise compliance (BYOC, FedRAMP).

## Market Context

- Agent observability market is fragmented — 10+ tools but all have the same gaps
- Primary gap: multi-agent trace correlation and non-linear execution visualization
- Key competitors: LangSmith (LangChain-locked), Langfuse (open-source, shallow), Arize Phoenix (OTel, limited UI), Braintrust (eval-focused), AgentOps (Python-only)
- OpenTelemetry GenAI SIG is actively writing agent conventions — being a contributor is a strategic moat

## Monetization

| Tier | Model | Price |
|---|---|---|
| OSS / Self-hosted | Free, full features | $0 |
| Cloud | Managed, per-span ingestion | ~$0.10–0.50/100k spans + seat fee |
| Pro | Higher retention, alerting, eval | $99–299/mo |
| Enterprise | BYOC, SSO, SOC2, SLAs | $50k+/yr ACV |

## Tech Stack Decisions (TBD)

- Backend: TBD
- OTel Collector: OpenTelemetry Collector (Go-based, extend with custom processors)
- Storage: ClickHouse (traces + metrics), S3 (blobs)
- Graph visualization: TBD (D3.js, Cytoscape, or React Flow)
- Frontend: TBD

## Key Risks

- OTel GenAI SIG agent conventions are still being written — building on a moving standard
- Mitigation: actively contribute to the spec to gain influence and early awareness
