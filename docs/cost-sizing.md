# Cost and Sizing Guide

> **Disclaimer:** All numbers in this document are estimates based on typical resource
> characteristics of Go, ClickHouse, and Postgres workloads. They are not empirical
> benchmarks from a production AgentPulse deployment. Use them as order-of-magnitude
> guidance when planning infrastructure; measure your actual workload before committing
> to a configuration (see [How to benchmark yourself](#how-to-benchmark-yourself)).

---

## Data model

A single AgentPulse span is one row in ClickHouse's `spans` table. The row includes:

- Identity fields: `project_id`, `run_id`, `session_id`, `user_id`, `span_id`, `parent_span_id`
- Classification: `span_kind`, `agent_name`, `tool_name`, `gen_ai_system`, `model`
- Numeric metrics: `duration_ms`, `latency_ms`, `input_tokens`, `output_tokens`, `cost_usd`
- Timestamps: `start_time`, `end_time`, `timestamp`
- A `payload_ref` column used when the raw prompt/completion is offloaded to S3/MinIO

**Typical span size:**

| Form | Estimated size |
|---|---|
| Uncompressed (JSON-equivalent) | 2–4 KB |
| Compressed in ClickHouse MergeTree | 400–800 bytes |

MergeTree applies LZ4 compression by default and achieves 5–10x on JSON-like, column-oriented data. The columnar layout means numeric columns (tokens, cost, duration) compress especially well.

**Payload offloading:** LLM spans whose prompt+completion exceeds 8 KB are offloaded to MinIO/S3 by `clickhouseexporter`. Only the `payload_ref` (`bucket/object` path) is stored in ClickHouse. This keeps the hot-path row size predictable regardless of prompt length. MinIO lifecycle rules expire payloads at 35 days by default (configured in `docker-compose.yml`).

---

## Workload profiles

| Tier | Spans per day | Concurrent agents | Typical use case |
|---|---|---|---|
| Small | 100K | 1–2 | Hobby / local development |
| Medium | 1M | 5–10 | Small team / staging |
| Large | 10M | 50+ | Production deployment |

"Concurrent agents" refers to agents that are actively emitting spans at the same time. The collector and backend are stateless horizontally, so the bottleneck at scale is ClickHouse write throughput and query latency — not agent fan-out directly.

---

## Storage sizing

### ClickHouse

Using the compressed estimate of 600 bytes/span (midpoint of 400–800):

| Tier | Daily ingestion (uncompressed) | Daily ingestion (compressed) | 30-day storage | 90-day storage |
|---|---|---|---|---|
| Small (100K spans/day) | ~300 MB | ~60 MB | ~1.8 GB | ~5.4 GB |
| Medium (1M spans/day) | ~3 GB | ~600 MB | ~18 GB | ~54 GB |
| Large (10M spans/day) | ~30 GB | ~6 GB | ~180 GB | ~540 GB |

Additional ClickHouse tables (`metrics_agg`, `run_metrics`, `session_agg`, `user_agg`, `span_evals`) contribute materially only at the Large tier — add 10–20% overhead to the estimates above.

ClickHouse's MergeTree engine also stores primary index and mark files on disk. For the workloads above these are negligible (a few hundred MB at most).

**Partitioning note:** The `spans` table should be partitioned by `(project_id, toYYYYMM(timestamp))`. This lets the retention enforcer drop entire parts via `ALTER TABLE spans DROP PARTITION` rather than running row-level deletes, which is dramatically cheaper.

### Postgres

Postgres holds only operational state — projects, topology, budget rules, evals config, alert events, retention policies, and ingest tokens. This data does not grow with span volume; it grows with the number of projects and human-entered annotations.

| Use case | Estimated Postgres size |
|---|---|
| 1–10 projects, up to 1 year of alert history | 100–300 MB |
| 10–100 projects | 300 MB – 1 GB |
| 100+ projects with extensive alert/eval history | 1–5 GB |

For most self-hosted deployments Postgres storage is not a meaningful cost driver. A single managed instance or a small Droplet/RDS instance handles all tiers comfortably.

### MinIO / S3

Payload offloading is optional — it is only triggered when a span's prompt+completion exceeds 8 KB. A pure tool-orchestration workload (no large LLM payloads) may produce zero S3 objects.

For LLM-heavy workloads where most spans carry prompts:

| Tier | Approximate S3/MinIO usage (35-day window) |
|---|---|
| Small | 1–5 GB |
| Medium | 10–50 GB |
| Large | 100–500 GB |

S3 object storage is inexpensive (~$0.023/GB/month on AWS S3 Standard). At the Large tier, 90-day retention of payloads can be the single largest storage cost item; consider reducing the lifecycle expiry from 35 days if budget is a concern.

---

## Memory and CPU sizing

### Backend API (Go binary)

The backend is a stateless Go binary (`backend/cmd/server/main.go`). Its three background goroutines (eval worker, alert evaluator, retention enforcer) add minimal overhead.

| Tier | RAM | CPU |
|---|---|---|
| Small | 128–256 MB | 0.1–0.25 vCPU |
| Medium | 256–512 MB | 0.25–0.5 vCPU |
| Large | 512 MB – 1 GB | 0.5–1 vCPU |

The eval worker's RAM usage spikes briefly when fetching span payloads from S3 for LLM-as-judge calls, but these are serialized via the `eval_jobs` queue and bounded in practice.

### OTel Collector (Go binary)

The collector is also stateless except for the `budgetenforceproc` in-memory cost cache and the batch buffer.

| Tier | RAM | CPU |
|---|---|---|
| Small | 128–256 MB | 0.1–0.25 vCPU |
| Medium | 256–512 MB | 0.25–0.5 vCPU |
| Large | 512 MB – 1 GB | 0.5–2 vCPU |

CPU usage is dominated by attribute extraction in `agentsemanticproc` and protobuf decode. At 10M spans/day (~115 spans/second average with typical bursty traffic) a single collector instance running on a 2-vCPU node is adequate; scale horizontally if you hit CPU saturation (accepting that per-instance budget counters become approximate — see [architecture.md](./architecture.md#scaling-notes)).

### ClickHouse

ClickHouse is the resource-hungry component. It benefits significantly from more RAM because it uses memory for query-time merges and caches compressed block data.

| Tier | RAM | Storage | CPU |
|---|---|---|---|
| Small | 2 GB | 10 GB SSD | 1 vCPU |
| Medium | 4–8 GB | 50–100 GB SSD | 2–4 vCPU |
| Large | 16–32 GB | 200–600 GB SSD | 4–8 vCPU |

Dashboard queries at the Medium tier (scanning a day's worth of spans for one project) typically complete in under 200 ms on 4 GB RAM. Concurrent dashboard users and background retention jobs push RAM requirements up; 8 GB is the comfortable production floor.

ClickHouse stores data on local disk only in the self-hosted configuration. Use fast NVMe SSD at the Large tier; spinning disk causes noticeable merge pressure.

### Postgres

| Tier | RAM | Storage |
|---|---|---|
| All tiers | 512 MB – 1 GB | 10–20 GB SSD |

Postgres is lightly loaded in all tiers. Even a $5–7/month managed instance (DO Managed Postgres basic) handles the Small and Medium tiers without tuning.

---

## Cloud cost estimates

All figures are approximate monthly costs in USD. Assume 30-day retention for ClickHouse, minimal S3 usage (tool-only workload).

> These are order-of-magnitude estimates based on typical Go + ClickHouse resource
> profiles, not measured benchmarks.

### DigitalOcean (DO Droplet + Managed Postgres + self-hosted ClickHouse on Droplet)

| Component | Small | Medium | Large |
|---|---|---|---|
| Backend API (DO Droplet, shared) | $6 (1 vCPU / 1 GB) | $12 (2 vCPU / 2 GB) | $24 (2 vCPU / 4 GB) |
| Collector (same Droplet as backend or separate) | $0 (co-locate) | $6 (1 vCPU / 1 GB) | $12 (2 vCPU / 2 GB) |
| ClickHouse (DO Droplet) | $12 (2 vCPU / 2 GB) | $24 (2 vCPU / 4 GB) | $96 (4 vCPU / 16 GB) |
| Block storage for ClickHouse data | $1 (10 GB) | $5 (50 GB) | $25 (250 GB) |
| Managed Postgres (DO Basic) | $15 | $15 | $50 |
| Spaces (S3-compatible, if payloads offloaded) | ~$0 | ~$5 | ~$25 |
| **Estimated monthly total** | **~$34** | **~$67** | **~$232** |

### AWS (EC2 + RDS Postgres + ClickHouse Cloud)

ClickHouse Cloud removes the operational burden of managing ClickHouse but costs more at scale.

| Component | Small | Medium | Large |
|---|---|---|---|
| Backend + Collector (t3.small EC2) | $15 | $15 (t3.small) | $30 (t3.medium) |
| ClickHouse Cloud (development tier, ~16 GB RAM) | $50–70 | $100–150 | $400–600 |
| RDS Postgres (db.t3.micro) | $15 | $15 | $30 (db.t3.small) |
| S3 (if payloads offloaded) | ~$0 | ~$2 | ~$10 |
| **Estimated monthly total** | **~$80–100** | **~$130–180** | **~$470–670** |

Self-hosting ClickHouse on EC2 (instead of ClickHouse Cloud) significantly reduces the AWS cost at the Medium and Large tiers — expect roughly half the ClickHouse line item, with added operational overhead.

### Fly.io (backend + collector as Fly apps, managed DBs external)

Fly.io works well for the stateless backend and collector. You still need an external ClickHouse (self-hosted on a VM or ClickHouse Cloud) and a managed Postgres.

| Component | Small | Medium | Large |
|---|---|---|---|
| Backend (Fly Machine, 1 CPU / 512 MB) | $3–5 | $5–10 | $10–20 |
| Collector (Fly Machine, 1 CPU / 512 MB) | $3–5 | $5–10 | $10–20 |
| ClickHouse (external — self-hosted VM) | $12–20 | $30–50 | $80–150 |
| Managed Postgres (Fly Postgres or external) | $7 | $7 | $20 |
| S3/R2 (if payloads offloaded) | ~$0 | ~$2 | ~$10 |
| **Estimated monthly total** | **~$25–37** | **~$49–79** | **~$130–220** |

---

## How to benchmark yourself

Once you have a running deployment, use these commands to measure actual resource usage.

**ClickHouse — per-table disk usage:**

```sql
SELECT
    name,
    formatReadableSize(total_bytes) AS size
FROM system.tables
WHERE database = 'agentpulse'
ORDER BY total_bytes DESC;
```

**ClickHouse — overall database size:**

```sql
SELECT formatReadableSize(sum(total_bytes))
FROM system.tables
WHERE database = 'agentpulse';
```

**Postgres — database size:**

```sql
SELECT pg_size_pretty(pg_database_size('agentpulse'));
```

**Postgres — per-table breakdown:**

```sql
SELECT
    relname AS table,
    pg_size_pretty(pg_total_relation_size(relid)) AS total_size
FROM pg_catalog.pg_statio_user_tables
ORDER BY pg_total_relation_size(relid) DESC;
```

**Container memory and CPU (all services):**

```bash
docker stats --no-stream
```

**Span ingest rate — collector metrics:**

The collector exposes Prometheus metrics on port `8888`. The `otelcol_exporter_sent_spans_total` counter gives the cumulative spans exported to ClickHouse:

```bash
curl -s http://localhost:8888/metrics | grep otelcol_exporter_sent_spans_total
```

For a per-minute rate, poll twice 60 seconds apart and subtract. Alternatively, check the collector's structured logs:

```bash
docker logs agentpulse-collector 2>&1 | grep "Spans exported"
```

**ClickHouse write throughput:**

```sql
SELECT
    toStartOfMinute(event_time) AS minute,
    sum(written_rows) AS rows_written
FROM system.part_log
WHERE database = 'agentpulse'
  AND table = 'spans'
  AND event_time > now() - INTERVAL 1 HOUR
GROUP BY minute
ORDER BY minute DESC;
```

---

## Cost optimization tips

### Retention policies

The single most effective cost lever. Shorter retention = smaller ClickHouse disk footprint = smaller (cheaper) node.

Set a retention policy per project via the API or UI. The retention enforcer runs hourly and issues partition-drop operations against ClickHouse. Recommended starting points:

| Tier | Suggested retention |
|---|---|
| Small (hobby) | 14 days |
| Medium (team) | 30 days |
| Large (production) | 90 days, with archival to S3 Glacier for older data |

### ClickHouse TTL

You can enforce retention directly in the ClickHouse schema with a `TTL` clause as a backstop, independent of the application-level enforcer:

```sql
ALTER TABLE agentpulse.spans
MODIFY TTL timestamp + INTERVAL 90 DAY DELETE;
```

TTL merges run during background merge operations. Do not rely on TTL alone for cost management — partition drops via the retention enforcer are faster and more predictable.

### Payload offloading threshold

The 8 KB offloading threshold in `clickhouseexporter` is the default. For workloads where prompts are consistently small (e.g., structured tool calls), payloads rarely exceed 8 KB and MinIO storage is negligible. For workloads with long prompts (RAG pipelines, document summarization), the threshold keeps ClickHouse row size bounded.

If you want to reduce S3 costs, raise the threshold (accepting larger ClickHouse row sizes) or disable payload storage entirely by setting payload offloading to off in the exporter config.

### MinIO lifecycle rules

The default lifecycle rule in `docker-compose.yml` expires S3 objects after 35 days:

```yaml
mc ilm rule add --expiry-days 35 local/agentpulse-spans
```

Reduce this to 14 days if you only need payloads for active eval jobs. The eval worker fetches payloads during scoring; once scored, the payload is no longer needed by the platform (though you may want it for manual review).

### ClickHouse compression codec tuning

For high-volume deployments, explicit codec configuration can improve the compression ratio beyond LZ4's default. Text-heavy columns (agent names, model names, status strings) compress well with `ZSTD`:

```sql
ALTER TABLE agentpulse.spans
MODIFY COLUMN agent_name String CODEC(ZSTD(3));
```

Apply selectively — ZSTD decompression is slower than LZ4 at read time, so avoid it on columns used in hot-path WHERE filters.

### Horizontal scaling

At the Large tier, consider splitting the eval worker into its own process. It is already isolated behind a `RunOnce()` entrypoint in `backend/internal/eval/worker/`. Running it separately lets you scale the API and eval worker independently, and avoids LLM judge calls consuming backend RAM during traffic spikes.
