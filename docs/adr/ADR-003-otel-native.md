# ADR-003: OpenTelemetry-native protocol instead of a bespoke ingestion protocol

**Status:** Accepted  
**Date:** 2025-Q1

## Context

AgentPulse needed an ingestion protocol: a wire format and transport by which Python and TypeScript SDK users (and framework auto-instrumentors) send spans to the collector. The choice is foundational — it determines what existing instrumentation is compatible without code changes, what SDK surface developers interact with, and how easy it is to add new integrations.

At decision time, the leading alternative approaches were: (1) design a custom JSON HTTP API purpose-built for AI agent spans, (2) adopt OpenTelemetry OTLP as the wire protocol and extend it with agent-specific semantic conventions, (3) proxy an existing vendor protocol (Datadog wire format), or (4) use Prometheus remote write for metric-like data.

The target user runs multi-agent frameworks (CrewAI, LangGraph, LangChain, OpenAI Agents SDK) that already have OTel instrumentation or can emit OTLP with minimal configuration. The market trend in 2026 is toward OTel GenAI semantic conventions as the standard (`gen_ai.*`, `agentpulse.*` attribute namespaces), with Datadog, Logfire, OpenLIT, and Traceloop all converging on OTLP ingest.

## Decision

Use **OpenTelemetry OTLP** (gRPC on `:4317`, HTTP on `:4318`) as the sole ingestion protocol. Add agent-specific semantics via a custom collector processor (`agentsemanticproc`) rather than by extending the wire protocol.

The `agentsemanticproc` processor classifies spans in-flight using a combination of standard OTel GenAI attributes (`gen_ai.system`, `gen_ai.request.model`) and AgentPulse-specific attributes (`agentpulse.span.kind`, `agentpulse.tool.name`, `agentpulse.handoff.target`, `agentpulse.memory.key`). This means any standard OTel sender can emit spans; AgentPulse-specific enrichment is applied at the collector, not forced onto SDK users. The processor also computes `agentpulse.cost.usd` from `config/model_pricing.yaml` so clients never need to embed pricing logic.

The collector pipeline is: `otlp receiver → batch → agentsemanticproc → piimaskerproc → budgetenforceproc → [clickhouseexporter, topologyexporter]`. It is built on the stock OpenTelemetry Collector contrib framework with AgentPulse-specific components registered as custom processors and exporters.

## Consequences

### Positive

- Any framework with OTel instrumentation (LangChain, LlamaIndex, CrewAI, Vercel AI SDK, OpenAI JS) works with zero AgentPulse-specific code changes beyond setting the OTLP endpoint.
- Framework auto-instrumentors in `sdk/python/agentpulse/instrumentors/` and `sdk/typescript/src/instrumentors/` are thin wrappers over standard OTel SDKs — new framework support is typically under 100 lines.
- The OTel collector ecosystem provides battle-tested receivers, processors (batching, retry, tail-sampling), and exporters that AgentPulse inherits without reimplementation.
- OTel GenAI semantic conventions (`invoke_agent`, `execute_tool` span operations) are the emerging industry standard. As the spec matures, AgentPulse gains compatibility with tools implementing it without protocol changes.
- Collector auth tightening (requiring `Authorization: Bearer <ingest_token>` at the OTLP receiver) is a well-understood extension pattern; the existing `project_ingest_tokens` table in Postgres provides the lookup.

### Negative / Trade-offs

- The OTel collector adds a separate process to the deployment. The planned "indie mode" (single binary) will embed the OTLP receiver directly in the Go backend, but the current team-mode deployment requires operating a collector alongside the API server.
- OTLP over gRPC requires protobuf tooling familiarity for contributors debugging raw ingestion.
- The collector's default configuration (`collector/config.yaml`) is not exposed in the standard Docker Compose setup — it is hidden behind `--profile full`, a friction point noted in the feasibility analysis.

## Alternatives Considered

**Custom JSON HTTP API.** Maximum control over the schema, no collector dependency. However, it would immediately break compatibility with every existing OTel-instrumented framework and require AgentPulse-specific SDK wrappers for all language/framework combinations. The maintenance burden would be proportional to the number of frameworks supported — not feasible for a small team.

**Datadog wire protocol.** A sizeable base of existing tooling already emits Datadog-formatted traces. However, adopting the Datadog format would create an implicit dependency on a commercial vendor's schema decisions, complicate the relationship with the OTel ecosystem, and exclude users who are specifically avoiding Datadog. No major OSS observability platform has adopted this path.

**Prometheus remote write.** Designed for metric time-series, not distributed traces. Spans have a fundamentally different model (parent/child relationships, variable-length attribute maps, large payloads). Using Prometheus remote write would require mapping span trees into metric labels in a lossy way.

**OpenInference (Arize Phoenix spec).** A viable OTel-compatible option used by Phoenix. However, it is less widely adopted than the OTel GenAI semconv emerging from the CNCF/OTel working group, and adopting it would tie AgentPulse to Phoenix's spec evolution rather than the broader standard.
