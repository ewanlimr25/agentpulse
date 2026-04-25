# ADR-004: Go instead of Rust or Python for the backend and collector

**Status:** Accepted  
**Date:** 2025-Q1

## Context

AgentPulse has two performance-sensitive Go processes: the **backend API** (chi router, serving REST, WebSocket, and SSE endpoints) and the **OTel collector** (custom processors and exporters handling the span ingestion pipeline). Both require concurrent I/O, low memory overhead at idle, and fast startup times — especially relevant for the planned single-binary "indie mode" that should start in under 60 seconds.

The collector in particular is on the critical path for every span: spans pass through `agentsemanticproc` (classification + cost computation), `piimaskerproc`, and `budgetenforceproc` before being fanned out to ClickHouse and Postgres. Latency added here affects observed trace freshness.

The choice of language also determines what library ecosystem is available. The OpenTelemetry Collector is written in Go; building custom processors and exporters in a different language would require bridging over IPC or HTTP, adding latency and operational complexity.

## Decision

Use **Go** for the backend API server and the OTel collector.

The backend is structured as a Chi router with handler → store layering (`backend/internal/api/handler/`, `backend/internal/store/{chstore,pgstore,s3store}/`). The available Go libraries for this stack are mature: `go-chi/chi` for routing, `jackc/pgx` for Postgres, `ClickHouse/clickhouse-go` for ClickHouse, and the AWS SDK for S3/MinIO. Three background goroutines (eval worker, alert evaluator, retention enforcer) run in the same process and coordinate via Go channels without a message broker.

The collector uses the `go.opentelemetry.io/collector` framework directly. Custom processors (`agentsemanticproc`, `piimaskerproc`, `budgetenforceproc`) and exporters (`clickhouseexporter`, `topologyexporter`) are registered as standard collector components. This is the idiomatic path for OTel Collector customization — all reference implementations and contrib components are in Go.

## Consequences

### Positive

- The OTel Collector SDK is Go-first. Building custom processors and exporters in Go means no language boundary, no IPC overhead, and direct access to the `pdata` span model. All collector contrib examples and documentation assume Go.
- Go's goroutine model maps naturally to the collector's concurrent span-processing pipeline and the backend's background workers. The budget-enforcement cache (in-memory `(project_id, run_id) → cumulative_cost_usd` map with periodic Postgres sync) is straightforward to implement safely with `sync.Map` and periodic refresh goroutines.
- Compiled Go binaries are self-contained, small (~20–25 MB), and start in milliseconds. This is essential for the "indie mode" blueprint where the entire stack must start in under 60 seconds on a $5 VPS.
- Go's static typing, fast compile times, and `go vet` / `staticcheck` / `gosec` tooling support the CI workflow without a heavy toolchain.
- The ecosystem libraries (`chi`, `pgx`, `clickhouse-go`) are actively maintained, well-documented, and used in production by major projects.
- The backend's handler and store files are each under 300 lines; Go's explicit error handling and lack of hidden control flow make the codebase navigable for new contributors.

### Negative / Trade-offs

- Go's type system is less expressive than Rust's or Haskell's — certain invariants that could be enforced at compile time in Rust must instead be verified at runtime with explicit checks.
- The eval worker calls LLM APIs and waits on I/O; if it grows into a bottleneck, it can be split into a separate binary consuming the `eval_jobs` table. The code is already isolated behind a `RunOnce()` entrypoint, but splitting it adds an operational process to manage.
- Go does not have a native async/await model; contributors coming from Python or TypeScript need to learn goroutines and channels. In practice the patterns used in this codebase (goroutine-per-request, channel-based worker queues) are idiomatic and well-documented.

## Alternatives Considered

**Rust.** Would offer stronger memory safety guarantees at compile time and the highest raw throughput, which is attractive for the span-processing hot path. However, Rust's OTel ecosystem is less mature than Go's — the collector SDK is Go-only, meaning a Rust collector would need to reimplement the pipeline framework or bridge over gRPC. Rust's steeper learning curve and longer compile times also raise the barrier for external contributors. The performance ceiling of Go is more than adequate for the target scale (single-node ClickHouse handles ~10k spans/second; Go is not the bottleneck).

**Python.** The Python SDK is already Python, and the team has familiarity with the language. However, Python's GIL and async performance characteristics make it a poor fit for the collector's concurrent span-processing pipeline. Running the backend in Python would require either an ASGI framework with worker processes (adding operational complexity comparable to the current Go setup) or accepting throughput limits that would become visible at a few hundred concurrent agent runs.

**Node.js / TypeScript.** The frontend is Next.js 15, so TypeScript expertise exists. However, Node.js's event-loop model and V8 memory overhead (the Next.js dev server alone uses 300–500 MB) are a poor match for a high-throughput span pipeline. The collector SDK is not available for Node.js; a TypeScript backend would need to implement its own OTLP receiver, losing the collector contrib ecosystem.
