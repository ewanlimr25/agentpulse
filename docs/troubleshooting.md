# AgentPulse Troubleshooting Guide

This guide covers common issues when running AgentPulse locally or in production. Start with the general checklist, then jump to the relevant section.

---

## General Debugging Checklist

Run through this list before diving into a specific section. Most issues are caught here.

```bash
# 1. Are the core services running?
docker compose ps

# 2. Is the backend healthy?
curl -s http://localhost:8080/healthz
# Expected: {"status":"ok"}

# 3. Is the collector healthy?
curl -s http://localhost:13133/
# Expected: {"status":"Server available","uptime":"..."}

# 4. Did the collector actually accept spans?
docker compose logs collector | grep "accepted"

# 5. Are migrations up to date?
docker compose logs backend | grep -i "migration"

# 6. Is there a Postgres connectivity issue?
docker compose logs backend | grep -i "connection"

# 7. Is ClickHouse accepting writes?
docker compose logs backend | grep -i "clickhouse"

# 8. Check all service logs for ERROR lines
docker compose logs --tail=100 | grep -i "error\|fatal\|panic"
```

If everything looks healthy but the UI is still empty, jump to [UI shows "No runs yet" forever](#ui-shows-no-runs-yet-forever).

---

## Local Dev Setup

### `make dev-up` hangs / ports in use

**Symptoms:** `docker compose up` hangs, a service fails to start, or you see `bind: address already in use`.

**Diagnosis:**

```bash
lsof -i :5432   # Postgres — often a system install conflicts
lsof -i :9000   # ClickHouse — collides with MinIO and some other tools
lsof -i :9090   # MinIO
lsof -i :4317   # OTel collector gRPC
lsof -i :8080   # Backend API
```

**Fix:** Kill the offending process, or override the port in `docker-compose.yml` by changing the host-side port mapping (left side of the colon). For example, to move Postgres to 5433:

```yaml
ports:
  - "5433:5432"
```

Then update `DATABASE_URL` in your `.env` or environment accordingly.

---

### Backend exits with `relation "run_tags" does not exist`

**Symptoms:** Backend crashes at startup with a Postgres error referencing a missing table.

**Cause:** Your database is behind on migrations.

**Fix:**

```bash
# Re-run all migrations
make migrate
# or, if using the backend binary directly
go run ./cmd/server/... migrate
```

If the migration command itself fails, check the migration files in `backend/migrations/` are present and in the correct sequence. A gap in numbering will stop the migration runner.

---

### `make seed` wipes my data

That is the intended behaviour. `make seed` is a destructive reset tool for scratch installs. Never run it against a database containing real data.

---

## Collector

### Collector shows `invalid API key` on every span

**Symptoms:** Collector logs contain repeated `invalid API key` or `unauthenticated` errors for incoming spans.

**Cause:** You are sending spans to the wrong endpoint. The OTel receiver does **not** require a bearer token in this release — it accepts any valid OTLP payload on port `4317` (gRPC) or `4318` (HTTP). Port `8080` is the backend REST API and does enforce auth.

**Fix:** Point your SDK or framework instrumentation at the collector, not the backend:

```python
# Python SDK — correct
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
exporter = OTLPSpanExporter(endpoint="http://localhost:4317", insecure=True)
```

Verify with:

```bash
docker compose logs collector | tail -50
```

---

### Collector lag behind agents — spans appear delayed or with stale timestamps

**Symptoms:** Spans show up in the UI minutes after they were emitted, or the "last seen" timestamp in the topology view lags behind real time.

**Diagnosis:**

```bash
# Check collector queue depth and export metrics
curl -s http://localhost:8888/metrics | grep -E "otelcol_exporter_queue|otelcol_processor_batch"
```

Key metrics to watch:
- `otelcol_exporter_queue_size` — if this is persistently above zero, the exporter cannot keep up.
- `otelcol_processor_batch_timeout_trigger_send` — high values mean the batch processor is flushing on timeout rather than size, which adds latency.

**Fix:** Tune the batch processor and queue in your collector config (`collector/config.yaml`):

```yaml
processors:
  batch:
    send_batch_size: 1024      # increase from default 512
    timeout: 1s                # decrease from default 5s

exporters:
  otlphttp/backend:
    sending_queue:
      num_consumers: 10        # increase parallel senders
      queue_size: 5000
```

You can also pass these as environment variables if the config is templated:

```bash
OTEL_BATCH_SIZE=1024
OTEL_BATCH_TIMEOUT=1s
```

Restart the collector after any config change:

```bash
docker compose restart collector
```

---

### `go run ./tools/tracegen/... --project-id demo-project` succeeds but nothing appears in the UI

`demo-project` is the literal default string in the tracegen tool — it is not a valid project ID. You must pass your real project UUID (visible in the project settings page or via `GET /api/v1/projects`).

```bash
go run ./tools/tracegen/... --project-id <your-real-uuid>
```

---

## Storage

### ClickHouse disk full — spans stop ingesting

**Symptoms:**
- New spans stop appearing in the UI.
- Collector logs show errors such as `Code: 243. DB::Exception: No space left on device` or `NOT_ENOUGH_SPACE`.
- Backend logs show ClickHouse write failures.

**Diagnosis:**

```bash
# Check disk usage reported by ClickHouse itself
docker compose exec clickhouse clickhouse-client \
  --query "SELECT name, formatReadableSize(total_space), formatReadableSize(free_space), formatReadableSize(keep_free_space) FROM system.disks"

# Check how much each table is using
docker compose exec clickhouse clickhouse-client \
  --query "SELECT database, table, formatReadableSize(sum(bytes_on_disk)) AS size FROM system.parts GROUP BY database, table ORDER BY sum(bytes_on_disk) DESC"

# Check Docker volume usage from the host
docker system df -v | grep clickhouse
```

**Fix — purge old data:**

```bash
# Delete spans older than 30 days (adjust the interval to suit your retention policy)
docker compose exec clickhouse clickhouse-client \
  --query "ALTER TABLE agentpulse.spans DELETE WHERE timestamp < now() - INTERVAL 30 DAY"

# Wait for the mutation to complete
docker compose exec clickhouse clickhouse-client \
  --query "SELECT * FROM system.mutations WHERE is_done = 0"
```

**Fix — add a TTL to the spans table (recommended for production):**

```sql
ALTER TABLE agentpulse.spans
  MODIFY TTL toDateTime(timestamp) + INTERVAL 30 DAY;
```

**Fix — expand the Docker volume:** Increase the volume size in `docker-compose.yml` or mount a larger host directory. After expanding, restart ClickHouse:

```bash
docker compose restart clickhouse
```

---

### Postgres pool exhaustion — 429s or "too many connections"

**Symptoms:**
- Backend returns HTTP 429 or 503 with a message like `sorry, too many clients already` or `too many connections`.
- Backend logs contain `pq: sorry, too many clients already` or `pgx: acquire timeout`.
- The UI shows intermittent errors on pages that query project/eval data.

**Diagnosis:**

```bash
# Count active Postgres connections
docker compose exec postgres psql -U agentpulse -c \
  "SELECT count(*), state FROM pg_stat_activity GROUP BY state ORDER BY count DESC"

# See which applications are holding connections
docker compose exec postgres psql -U agentpulse -c \
  "SELECT application_name, state, count(*) FROM pg_stat_activity GROUP BY application_name, state ORDER BY count DESC"
```

**Fix:** Increase the maximum number of open connections available to the backend via environment variable:

```bash
# In your .env or docker-compose.yml environment block
DB_MAX_OPEN_CONNS=50      # default is 25
DB_MAX_IDLE_CONNS=10      # default is 5
DB_CONN_MAX_LIFETIME=5m   # default is 0 (no limit)
```

You must also ensure Postgres itself allows enough connections. Check `max_connections` in Postgres:

```bash
docker compose exec postgres psql -U agentpulse -c "SHOW max_connections"
```

If `DB_MAX_OPEN_CONNS` approaches `max_connections`, increase Postgres's limit by adding to `docker-compose.yml`:

```yaml
services:
  postgres:
    command: postgres -c max_connections=200
```

Restart after any change:

```bash
docker compose restart backend postgres
```

---

## Evaluations

### Stuck eval jobs — evals never complete, stay in "pending" state

**Symptoms:**
- The Evals page shows jobs with status `pending` or `running` that have not changed in more than a few minutes.
- No eval results appear even after triggering a manual eval run.

**Diagnosis:**

```bash
# List stuck jobs (pending for more than 10 minutes)
docker compose exec postgres psql -U agentpulse -c \
  "SELECT id, status, created_at, updated_at, error FROM eval_jobs WHERE status IN ('pending','running') AND updated_at < now() - INTERVAL '10 minutes' ORDER BY created_at DESC"

# Check whether the eval worker is running
docker compose ps eval-worker

# Check eval worker logs for errors
docker compose logs eval-worker --tail=100
```

**Fix — reset stuck jobs so they are retried:**

```bash
docker compose exec postgres psql -U agentpulse -c \
  "UPDATE eval_jobs SET status = 'pending', updated_at = now(), error = NULL WHERE status = 'running' AND updated_at < now() - INTERVAL '10 minutes'"
```

**Fix — restart the eval worker:**

```bash
docker compose restart eval-worker
```

If the worker crashes repeatedly, check for missing environment variables (LLM provider API key, `EVAL_MODEL`, etc.) and ensure ClickHouse and Postgres are both reachable from the worker container.

---

## Real-Time / WebSocket

### WebSocket disconnects — real-time alerts don't update

**Symptoms:**
- The alerts panel does not refresh without a page reload.
- Browser DevTools → Network → WS shows the connection closes shortly after opening, or reconnects in a loop.
- Browser console shows errors like `WebSocket is closed before the connection is established` or `1006 Abnormal Closure`.

**Diagnosis:**

1. Check the WebSocket endpoint directly from the browser console:

   ```javascript
   const ws = new WebSocket("ws://localhost:8080/api/v1/ws/alerts?project_id=<your-project-uuid>");
   ws.onmessage = e => console.log(e.data);
   ws.onerror = e => console.error(e);
   ws.onclose = e => console.warn("closed", e.code, e.reason);
   ```

2. Check backend logs for upgrade errors:

   ```bash
   docker compose logs backend | grep -i "websocket\|upgrade\|101"
   ```

3. If running behind a reverse proxy (nginx, Caddy, AWS ALB, etc.), check that WebSocket upgrade headers are forwarded correctly.

**Common causes and fixes:**

| Cause | Fix |
|---|---|
| Reverse proxy strips `Upgrade` / `Connection` headers | Add `proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade";` (nginx) or equivalent |
| Proxy read/write timeout is too short (default 60 s) | Set `proxy_read_timeout 3600s; proxy_send_timeout 3600s;` (nginx) |
| AWS ALB idle timeout | Increase the idle timeout to 3600 s in the ALB settings, or use NLB instead |
| Missing `project_id` query param | The `/api/v1/ws/alerts` endpoint requires `?project_id=<uuid>` |
| Ad-blocker or browser extension blocking the WS | Test in an incognito window without extensions |

After updating a reverse proxy config, reload it and reconnect:

```bash
# nginx
sudo nginx -t && sudo nginx -s reload
```

---

## Frontend

### UI shows "No runs yet" forever

**Symptoms:** The runs list is permanently empty even after sending test spans.

**Step-by-step diagnosis:**

```bash
# 1. Backend health
curl -s http://localhost:8080/healthz

# 2. Did the collector accept spans?
docker compose logs collector | grep "accepted"

# 3. Did the backend write them to ClickHouse?
docker compose logs backend | grep -i "clickhouse\|insert\|error"

# 4. Query ClickHouse directly
docker compose exec clickhouse clickhouse-client \
  --query "SELECT project_id, count() FROM agentpulse.spans GROUP BY project_id ORDER BY count() DESC LIMIT 10"
```

If spans are in ClickHouse but the UI is still empty: the `project_id` in the spans does not match the UUID in the URL bar. Use the project UUID shown in project settings, not a human-readable slug.

---

## Service Availability

### Can I disable MinIO?

Yes, if no individual span payload exceeds 8 KB. Remove the `minio` and `minio-init` services from `docker-compose.yml` and set the backend environment variable:

```bash
OBJECT_STORE_ENABLED=false
```

### Can I disable ClickHouse or Postgres?

No. Every feature in AgentPulse depends on both stores:
- **ClickHouse** stores span-scale trace and metric data.
- **Postgres** stores projects, topology, budget config, and eval results.

Removing either will cause the backend to fail at startup.
