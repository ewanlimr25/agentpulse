# Feasibility analysis — AgentPulse for independent developers

**Question:** *Can a solo developer running multi-agent workflows (Claude Code, CrewAI, LangGraph, LangChain, OpenAI Agents SDK, …) clone this repo and use it as a lightweight, secure observability platform for their own work?*

**Short answer:** *Yes for local use today, with caveats. No if you mean lightweight, zero-config, or internet-exposed. The gap between what AgentPulse is today and what a solo dev actually wants is bridgeable — see [recommendations.md](./recommendations.md) — but worth understanding before committing.*

This doc is an honest audit: what works, what hurts, where the real dragons are.

---

## Target user

The analysis is written for this user specifically:

> A developer (0–3 teammates) running AI agents for their own work or a small product. They want to see which prompts are expensive, which tools fail silently, which multi-agent runs went off the rails, and whether a prompt change made things better or worse. They do not have a security team, a Kubernetes cluster, or a budget for $199/month SaaS.

Everything below is evaluated against that profile. Enterprise / team concerns are noted but not weighted.

---

## Feasibility verdict

| Dimension | Verdict | Nuance |
|---|---|---|
| Runs on a laptop | ✅ yes | ~4 GB RAM in use, 16 GB recommended |
| Lightweight | ❌ no | 3 databases, 3 Go services, 1 Next.js server |
| Secure for local, single-user | ✅ mostly | Bearer auth + hashed tokens; no HTTPS by default |
| Secure for internet exposure | ❌ no | No TLS, no RBAC, collector unauth'd |
| Feature-rich | ✅ very | Already ahead of Phoenix, parity with Langfuse on most axes |
| Documented | 🟡 mixed | README + SDK docs solid; deployment, troubleshooting, OpenAPI missing |
| Stable | 🟡 pre-1.0 | No CI gate on backend; several UI surfaces are backend-only |

**Bottom line.** AgentPulse today is a viable *local* observability stack for an individual developer, producing real signal their existing tools don't. It is not yet a drop-in replacement for a hosted SaaS, because the operational overhead (Docker, migrations, environment variables, port juggling) and missing UI surfaces (token rotation, retention config, PII config, onboarding wizard) leak through. Both problems are fixable. Neither is a blocker for someone comfortable with Docker and Go.

---

## What solo devs get out of the box

These are concrete, real wins — features no single OSS alternative gives you today in one install.

1. **Unified dashboard for cost + latency + evals + topology + alerts.** Every competitor forces at least a two-tool setup (e.g., Helicone for cost + Langfuse for traces). AgentPulse's project page has all five tiles.
2. **Topology DAG — not a Gantt.** A real React Flow graph of which agent called which, with handoff edges. For multi-agent runs this is the #1 debugging view, and nobody else renders it well.
3. **Real-time budget halts.** You can cap a run at $0.50 and have AgentPulse stamp a halt flag on subsequent spans so your SDK wrapper stops calling LLMs. Most tools tell you you overspent *after* the fact.
4. **Prompt playground.** Click any `llm.call` span, edit the prompt, re-send with A/B variants, compare outputs and cost side-by-side. Braintrust charges for this; AgentPulse ships it.
5. **Claude Code hook.** One command (`agentpulse hook install`) and every future Claude Code session becomes a run with full tool-call + TodoWrite + FileEdit timeline. Zero code changes.
6. **Deterministic replay.** `agentpulse replay <run-id>` packages the full LLM/tool interaction trace so you can re-run locally with recorded responses. Useful for debugging agent behaviour without re-paying for the LLM calls.
7. **Loop detection.** Two-tier — repeated tool calls + topology cycles — catches runaway agents before your bill does.
8. **Multi-judge evals.** Relevance, hallucination, faithfulness, toxicity, tool-correctness, and custom judges with multi-model consensus. Configurable per project.
9. **Framework auto-instrumentors** for CrewAI, AutoGen, LlamaIndex, LangChain (Python) + Vercel AI SDK, OpenAI JS, LangChain (TS). No manual span wrapping.
10. **Quality gates for CI.** `agentpulse eval check --min-score 0.8` as a single step in GitHub Actions blocks a merge if quality regressed.

---

## What it actually costs to run

| Component | RAM | Disk at start | Disk growth |
|---|---|---|---|
| ClickHouse (Docker) | 1–2 GB | 500 MB | ~500 MB per 1M spans |
| Postgres (Docker) | 100–200 MB | 50 MB | trivial |
| MinIO (Docker) | 200–400 MB | 50 MB | bounded by 35-day lifecycle |
| Collector (Go) | 50–100 MB | 20 MB binary | 0 |
| Backend API (Go) | 50–100 MB | 25 MB binary | 0 |
| Next.js dev server | 300–500 MB | ~1.5 GB (node_modules) | 0 |
| **Total** | **~3–4 GB** | **~4 GB** | **~500 MB / 1M spans** |

On a modern laptop (16 GB RAM, M1+) this is comfortable. On an 8 GB machine, tight — you'll want to skip MinIO and close Chrome.

**Cloud sizing** (rough):

- $5/month DigitalOcean droplet: tight for the full stack, workable in single-binary mode (see [recommendations.md §indie mode](./recommendations.md#indie-mode-blueprint)).
- $20/month DO droplet (4 GB RAM, 2 vCPU): comfortable, ~1M spans/day.
- EC2 t3.medium + RDS db.t3.small + S3: ~$40-60/month at solo scale.

---

## Friction points (ordered by pain)

1. **First-run wizard doesn't exist.** After `make dev-up && make migrate-up`, the UI is blank. You must `curl POST /api/v1/projects` from a terminal and copy the `api_key` output. This is the #1 wall-hit for new users.
2. **`make migrate-up` is out of date.** It's missing the three newest Postgres and two newest ClickHouse migrations. The backend fails at startup with a cryptic `relation "run_tags" does not exist` error. Workaround is documented in [getting-started.md §3](./getting-started.md#applying-migrations-manually); actual fix is a 5-line Makefile edit.
3. **Too many processes to keep track of.** Collector, backend, frontend, three Docker containers, optional CLI. `tmux` / `overmind` / `goreman` patterns work, but the README doesn't acknowledge this.
4. **Env var sprawl.** 20+ env vars, most with defaults. There was no `.env.example` until this analysis; one has been added. Still, discovering which vars matter for which feature requires code grep.
5. **Collector is not in default docker compose.** Hidden behind `--profile full`. If you're happy running the collector in Docker rather than `go run`, you have to add `--profile full` to `make dev-up`, which isn't documented.
6. **Several features are backend-only.** Ingest token rotation, retention policies, PII redaction configs, email digest frequency — all work via `curl`, none have a UI. This is fine for a CLI-first user, annoying for a click-and-drag user.
7. **No `.env` override for collector endpoints.** Collector endpoints are hardcoded in `collector/config.dev.yaml`. To ingest from a remote machine, you edit the YAML or add an `-f` flag.
8. **No deployment story.** Nothing tells you how to run this on Fly.io, Railway, Cloud Run, or a single $5 VPS. The docker-compose setup is for local only.
9. **Documentation is sparse in specific corners.** Python SDK README is 12 lines (vs. TypeScript's 136). No OpenAPI. No troubleshooting guide outside [getting-started.md §Troubleshooting](./getting-started.md#troubleshooting).
10. **No LICENSE file.** Technically all-rights-reserved until one lands. Non-commercial-use language would be adequate; OSS-friendly preferred.

---

## Security posture (local use)

Good enough for a single developer on a laptop. The threats are basically: *"can another process on my machine read my traces?"* and *"can a malicious OTLP sender on my local network inject fake spans?"*

**Strengths:**
- Bearer tokens for the REST API, SHA-256 hashed in Postgres (never stored in plaintext)
- Per-project authorization (you can't read another project's runs with another project's key)
- IDOR-fixed on run-scoped routes (commit `323a6ec`)
- Admin mutations require a separate admin key
- WebSocket connections scoped to project ID
- Rate limiting on ingestion (100 spans/s default)

**Weaknesses (local use):**
- Default DB credentials (`agentpulse:agentpulse`) everywhere. If you expose Postgres port 5432 to your LAN by accident, anyone on the network can read all your data. The server *does* log a WARN at startup if you're using defaults — pay attention to it.
- LLM API keys (`ANTHROPIC_API_KEY`, etc.) are plain env vars. Any process running as your user can `read /proc/<pid>/environ`.
- MinIO uses the same default `agentpulse:agentpulse`. MinIO console runs on `:9091` — if you expose it, treat your trace payloads as public.

**Weaknesses (internet exposure):**
Do not expose AgentPulse to the internet as-is without:

1. **TLS termination** in front — nginx, Caddy, Cloudflare Tunnel, or Tailscale Funnel. The backend speaks plaintext HTTP only.
2. **Collector auth.** The OTLP receiver accepts spans from anyone; an attacker with your project ID can inject fake spans, pollute costs, and spam alerts. Block `:4317`/`:4318` at the firewall or front with an authenticating proxy until the planned collector-side token check lands.
3. **Rotated default credentials** in Postgres, ClickHouse, MinIO.
4. **CORS tightening.** Set `CORS_ALLOWED_ORIGINS=https://your-domain.com` and `APP_ENV=production` to disable the dev-mode wildcard.
5. **Webhook signing.** Alert webhooks POST unsigned bodies — if you forward them to a public endpoint, sign them yourself.

See [docs/recommendations.md §P0](./recommendations.md#p0-ship-in-the-next-8-weeks) for the shortlist of what would make AgentPulse safe to expose by default.

---

## Minimum viable deployment for one developer

If you want the leanest possible setup:

**Can drop:**
- MinIO — only needed if any span payload exceeds 8 KB. Small projects never hit that.
- Browser push (`VAPID_*`) — only needed for desktop push notifications.
- Email digest (`RESEND_API_KEY`) — only needed for daily email summaries.
- Evals — require an LLM API key; skip them if you don't care about quality scores.

**Can't drop:**
- ClickHouse — every span query hits it.
- Postgres — every config/budget/alert lives there.
- Collector — the ingestion entry point.
- Backend — the API.
- Frontend — unless you're happy with `curl`.

**Leanest working install:**

```yaml
# docker-compose.minimal.yml
services:
  clickhouse:
    image: clickhouse/clickhouse-server:24.8-alpine
    # ... (unchanged from main file)
  postgres:
    image: postgres:16-alpine
    # ... (unchanged)
# No MinIO. Expect "payload too large" warnings in the collector if your
# prompts/completions exceed 8 KB; they'll be truncated instead of offloaded.
```

Runs in ~1.5 GB RAM. Suitable for a $10/month VPS or an always-on dev machine.

A true *single-binary* deployment is on the P0 recommendation list (see [recommendations.md](./recommendations.md#p0-1-single-binary-indie-mode-sqlite--duckdb-backend)) — doesn't exist yet.

---

## Missing pieces for fresh-clone usability

Everything in this checklist is something a new user will trip over. They are listed in rough priority order.

### Setup blockers (fix before telling anyone to clone it)

- [x] `.env.example` in repo root — done as part of this analysis
- [ ] `make migrate-up` applies migrations 011-013 Postgres and 015-016 ClickHouse
- [ ] LICENSE file (MIT, Apache 2, AGPL, BUSL — pick something)
- [ ] First-run wizard: if no projects exist, the UI prompts to create one and returns the API key
- [ ] `make bootstrap` or `scripts/init.sh` that does: dev-up, migrate-up, create demo project, print API key, write to `.env.local`

### Important friction reducers

- [ ] Collector validates ingest tokens against `project_ingest_tokens` (not just the backend)
- [ ] Ingest-token management UI (generate, list, rotate, delete)
- [ ] Retention policy UI under Settings
- [ ] PII-redaction config UI under Settings
- [ ] Email-digest frequency/recipients UI under Settings
- [ ] TLS support built into the backend (`HTTP_TLS_CERT` + `HTTP_TLS_KEY` env vars), even if most people front with a proxy
- [ ] Health check returns DB connection status, not just `ok`

### Production-ready nice-to-haves

- [ ] Backend GitHub Actions workflow (`go test ./...`, `go vet ./...`, `staticcheck`, `gosec`)
- [ ] Frontend GitHub Actions workflow (type check, lint, unit tests)
- [ ] At least a handful of Playwright E2E tests
- [ ] Pre-built Docker image on a public registry (ghcr.io/agentpulse/…)
- [ ] Fly.io / Railway / Cloud Run deploy recipes
- [ ] OpenAPI spec generated from the Go handlers

### Documentation

- [ ] `CONTRIBUTING.md`
- [ ] `CODE_OF_CONDUCT.md`
- [ ] Deployment guide (self-host on VPS, K8s manifests, Fly.io `fly.toml`)
- [ ] Troubleshooting doc broken out from `getting-started.md`
- [ ] Python SDK README expanded to match TypeScript SDK's depth
- [ ] Architecture decision records for the ClickHouse+Postgres split, OTel-native choice, React Flow choice

---

## Comparison with alternatives for this user

| | AgentPulse (self-host) | Langfuse (self-host) | Phoenix | Helicone (proxy) | Langfuse Cloud Hobby | LangSmith |
|---|---|---|---|---|---|---|
| Cost | $0 + infra | $0 + infra | $0 + infra | $0 up to 10k req/mo | $0 up to 50k units/mo | $39/seat/mo |
| Setup complexity | 3 DBs, 4 services | 4 DBs (PG, CH, Redis, S3) | 1 container | 2-line SDK swap | sign up | sign up |
| Multi-agent DAG view | ✅ | ❌ waterfall | ❌ | ❌ | ❌ waterfall | ❌ |
| OTel-native | ✅ | partial ingest | ✅ | ❌ proxy | partial | partial |
| LLM-as-judge | 6 types | ✅ | ✅ | ❌ | ✅ | ✅ |
| Budget halts mid-run | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Claude Code hook | ✅ | community | ❌ | ❌ | community | ❌ |
| MCP observability | 🟡 (span kind only) | ❌ | ❌ | ❌ | ❌ | ❌ |
| Prompt playground | ✅ | ✅ | ✅ | partial | ✅ | ✅ |
| Data stays on your machine | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Truly lightweight | ❌ | ❌ | ✅ | ✅ (proxy) | N/A | N/A |
| Vendor lock-in | none | none | none | moderate (proxy) | moderate | high |

**Where AgentPulse wins today:** if you already have multi-agent workflows and want the DAG view, budget halts, Claude Code hook, or playground, no single alternative gives you all four.

**Where alternatives win today:** if you want *one container* and don't care about the DAG view, Phoenix is less work. If you want *no setup at all*, Langfuse Hobby or Helicone free tier save you an evening.

---

## The honest verdict

If you are a developer who:

- Builds multi-agent workflows with CrewAI, LangGraph, LangChain, OpenAI Agents SDK, or handwritten OTel
- Uses Claude Code for day-to-day development and wants to see where your tool time actually goes
- Cares about your prompts and traces not leaving your machine
- Is comfortable with Docker Compose and a 20-line Makefile

Then yes, AgentPulse is a viable daily driver today, subject to the caveats in this document. Plan for ~45 minutes of setup the first time, including reading [getting-started.md](./getting-started.md) and working around the stale Makefile.

If you want *zero-config*, *single-binary*, or *$5 VPS-ready*, AgentPulse is not there yet — but the architecture is set up so that an "indie mode" build is tractable (see [recommendations.md §indie mode](./recommendations.md#indie-mode-blueprint)). The winning play for the project is probably to ship that mode next.

---

## See also

- [getting-started.md](./getting-started.md) — go from clone to first trace
- [architecture.md](./architecture.md) — how it all fits together
- [recommendations.md](./recommendations.md) — where it should go next, backed by competitive research
- [roadmap.md](./roadmap.md) — what the maintainer has actually prioritized
