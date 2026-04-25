# ADR-001: ClickHouse + Postgres dual-storage instead of a single database

**Status:** Accepted  
**Date:** 2025-Q1

## Context

AI agent workloads generate spans at a volume that ordinary OLTP databases handle poorly. A single agent that reasons through 5 steps generates 40–75 spans; at even modest usage rates (a few hundred runs per day) this grows to millions of rows per month. At the same time, AgentPulse needs relational, strongly-consistent storage for operational data: projects, ingest tokens, budget rules, alert rules, topology graphs, eval configs, PII settings, and human annotations. These two workloads have fundamentally different shapes.

A single database would require compromising on one of the two. Postgres with TimescaleDB can handle time-series queries, but its row-store layout means scanning millions of spans for a dashboard aggregate is orders of magnitude slower than a columnar store. A single ClickHouse cluster can ingest spans efficiently, but its weak transaction model, lack of foreign-key enforcement, and awkward UPDATE/DELETE semantics make it a poor fit for mutable config data that needs ACID guarantees.

## Decision

Split storage along the line of workload character:

- **ClickHouse** holds everything that grows linearly with agent traffic and is queried analytically: `spans`, `metrics_agg`, `run_metrics`, `session_agg`, `user_agg`, `span_evals`, and full-text/semantic search indexes. The collector writes to ClickHouse in batches every few seconds using the native protocol; the backend reads from it for dashboard charts and trace exploration. A single ClickHouse node handles ~10k spans/second on modest hardware.

- **Postgres** holds operational state: `projects`, `project_ingest_tokens` (SHA-256 hashed), `topology_nodes`, `topology_edges`, `budget_rules`, `budget_alerts`, `alert_rules`, `alert_events`, `project_eval_configs`, `run_tags`, `run_annotations`, `span_feedback`, `prompt_playground_versions`, `retention_policies`, `push_subscriptions`. This data is small (roughly 100 MB for 100 projects over a year of alerts), changes infrequently, and benefits from transactions and joins.

- **S3 / MinIO** handles raw payloads exceeding 8 KB (LLM prompts, tool outputs, generation responses). ClickHouse stores only the `payload_ref`; MinIO lifecycle rules expire objects after 35 days.

## Consequences

### Positive

- ClickHouse scans 10M spans for a dashboard aggregate in under 200 ms — impossible with a row-store at this volume.
- Postgres gives full ACID semantics for budget rule enforcement and token management without paying OLAP-style overhead on a three-row settings page.
- Each store is scaled and operated independently: Postgres stays small; ClickHouse can be partitioned by `(project_id, toYYYYMM(timestamp))` if needed.
- The split is a natural seam — the repository interfaces (`chstore`, `pgstore`) mean neither the collector nor the backend couples directly to a particular storage engine, which makes the planned SQLite + DuckDB "indie mode" tractable.
- Langfuse (the closest competitor) also uses ClickHouse + Postgres; this stack is well-understood in the observability space.

### Negative / Trade-offs

- Two databases to deploy, migrate, and operate. Docker Compose setup requires both containers; a first-time user runs ~4 GB of RAM before they write a line of agent code.
- Queries that join analytical data with relational config (e.g., "show me runs that exceeded their budget rule") must be assembled in Go application code rather than a single SQL query.
- No single database transaction spans both stores. If a ClickHouse write succeeds but a Postgres write fails, the system must tolerate temporary inconsistency (this is handled by the collector's batch-retry logic).

## Alternatives Considered

**Single Postgres with TimescaleDB.** Removes operational complexity, but TimescaleDB row-store throughput is insufficient for span-scale queries. Dashboards requiring aggregation over millions of rows would require pre-materialized tables and significant engineering to stay fast — effectively reinventing ClickHouse inside Postgres.

**Single ClickHouse.** Eliminates the dual-store complexity, but ClickHouse's eventual-consistency model and absence of real transactions make it a poor home for mutable config (budget rules, ingest tokens, eval configs). Referential integrity and multi-row atomic updates would require application-level compensation logic throughout.

**Apache Cassandra.** Strong horizontal write throughput, but query flexibility is poor (no ad-hoc aggregations, no full-text search), operational complexity is high, and the JVM overhead is a poor fit for a lightweight self-hosted product.

**DuckDB (indie mode, future work).** Viable as an embedded alternative to ClickHouse for single-binary deployments. The recommendations.md P0 blueprint describes exactly this path. The interface-based storage layer means it can be added without changing application logic.
