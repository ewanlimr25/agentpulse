# Followups

Single consolidated punch list of everything this audit surfaced that hasn't been fixed yet. Ordered by priority. Each item links to the doc that discusses it in depth.

Already completed in this pass:

- [x] `.env.example` added with every env var + defaults
- [x] `make migrate-up` now applies all 13 Postgres + 16 ClickHouse migrations
- [x] `LICENSE` added (PolyForm Noncommercial 1.0.0) — protects against commercial resale / hosted services
- [x] `README.md` rewritten for current state
- [x] Tutorial, architecture, feasibility analysis, and recommendations docs written

Everything below is open.

---

## Tier 1 — do before telling anyone else to clone it

These are fresh-clone friction points that a new user will hit in the first 20 minutes.

- [ ] **First-run wizard.** After migrations, the UI shows a blank dashboard. Detect "no projects exist" and render a "Create your first project" form that calls `POST /api/v1/projects` and displays the one-time `api_key`. Today the user must use `curl`.
- [ ] **`scripts/init.sh` bootstrap.** One command that runs `make dev-up && make migrate-up`, creates a demo project, prints the API key, and writes it to `.env.local`. Prior art: every indie tool that wants adoption (Plausible, Umami, Supabase CLI).
- [ ] **Collector in default docker-compose.** Remove `profiles: ["full"]` from the collector service so `make dev-up` starts everything. Discussed in [feasibility-analysis.md](./feasibility-analysis.md#friction-points-ordered-by-pain).
- [ ] **`CONTRIBUTING.md`.** Dev setup (mirror tutorial), commit conventions (already used but not documented), test + lint expectations, PR checklist.
- [ ] **Python SDK README expansion.** Currently 12 lines (vs TypeScript's 136). Mirror the TS SDK's structure: install → minimal example → framework auto-instrumentors (CrewAI/AutoGen/LlamaIndex/LangChain) → environment variables → run-ID context.

## Tier 2 — production safety

Don't expose AgentPulse to the internet until these land. See [architecture.md §ingest auth](./architecture.md#ingest-authentication) and [feasibility-analysis.md §security](./feasibility-analysis.md#security-posture-local-use) for detail.

- [ ] **TLS in the backend.** `HTTP_TLS_CERT` + `HTTP_TLS_KEY` env vars bind HTTPS if both are set. Otherwise HTTP as today. Even if most people front with nginx/Caddy/Cloudflare, shipping the native path is table-stakes.
- [ ] **Collector validates ingest tokens.** The REST API enforces Bearer auth; the OTel receiver on `:4317/:4318` does not. Add a gRPC/HTTP interceptor that looks up `Authorization: Bearer` against `project_ingest_tokens`. Opt-out via env var for single-machine dev.
- [ ] **Webhook signing.** Budget/alert webhooks POST unsigned bodies. Add `X-AgentPulse-Signature: sha256=...` HMAC over the body, keyed per alert rule.
- [ ] **Rotate `agentpulse:agentpulse` defaults at setup.** `scripts/init.sh` generates random credentials and writes them to `.env` + `docker-compose.override.yml`, instead of shipping with the same default everywhere.
- [ ] **Audit log.** Append-only ClickHouse table `audit_events` with (timestamp, actor_token_hash, ip, endpoint, method, resource_id, outcome). The WarnDefaults mechanism in `backend/internal/config/config.go:147` is the right idea but stops short of actual audit.
- [ ] **CSRF protection for state-changing endpoints.** Cookie-based sessions don't exist today (Bearer only), but if the UI ever gains cookie auth this becomes required.

## Tier 3 — CI / testing gaps

Backend and SDKs have decent unit tests. CI only runs the TypeScript SDK.

- [ ] **`.github/workflows/backend.yml`** — `go test -race ./...`, `go vet ./...`, `staticcheck`, `gosec`, `gofmt -l` guard. Required-check on main.
- [ ] **`.github/workflows/web.yml`** — type check, ESLint, Vitest. Required-check on main.
- [ ] **`.github/workflows/collector.yml`** — build, test, smoke test (send a span, assert it appears in ClickHouse).
- [ ] **E2E tests.** `tests/e2e/` directory exists but is empty. Bare minimum with Playwright: create project → send trace via SDK → see it in UI → create budget rule → trigger alert. Under 5 tests covers the critical path.
- [ ] **Migration smoke test in CI.** Fresh Postgres + ClickHouse → `make migrate-up` → assert expected tables exist. Catches the regression we just fixed.

## Tier 4 — missing UI surfaces

Backend works, frontend doesn't. Each is a small React page talking to existing endpoints.

- [ ] **Ingest-token management page** (generate / list / revoke)
- [ ] **Retention-policy settings page** (`PUT /api/v1/projects/:projectId/storage/retention`)
- [ ] **PII-redaction config page** (regex patterns per project)
- [ ] **Email-digest settings page** (frequency + recipients + on/off)
- [ ] **Eval custom-judge editor** (prompt template authoring, dry-run against sample spans)
- [ ] **Loop-detection tuning** (threshold + window per project, currently hardcoded)
- [ ] **Playground A/B statistical significance** display (sample size + p-value + winner indicator)

## Tier 5 — documentation

- [ ] **`docs/deployment.md`** — Fly.io, Railway, Cloud Run, bare-VPS recipes. At minimum, a working `fly.toml` + `Dockerfile` for the backend.
- [ ] **`docs/troubleshooting.md`** — extracted and expanded from [getting-started.md §Troubleshooting](./getting-started.md#troubleshooting). Cover: ClickHouse disk full, Postgres pool exhaustion, collector lag behind agents, stuck eval jobs, websocket disconnects.
- [ ] **OpenAPI / Swagger** — generate from Chi handlers (or hand-write). Replaces the hand-maintained API reference in the README. Lets SDK authors (and Claude Code) reason about endpoints.
- [ ] **Architecture Decision Records.** Short docs explaining: why ClickHouse + Postgres (not one store), why React Flow for topology, why OTel-native instead of bespoke protocol, why Go instead of Rust/Python.
- [ ] **`docs/cost-sizing.md`** — empirical resource footprint + cloud sizing. "1M spans/day on a $20 DO droplet" kind of numbers.
- [ ] **`docs/sdk-versions.md`** — tested Python / Node / framework matrix. TypeScript SDK already has CI matrix; publish the actual supported combinations.

## Tier 6 — product direction

Covered in depth in [recommendations.md](./recommendations.md). Summary here for planning purposes.

P0 (next 8 weeks):

- [ ] **Single-binary "indie mode"** (SQLite + DuckDB) — [recommendations.md §P0-1](./recommendations.md#p0-1-single-binary-indie-mode-sqlite--duckdb-backend)
- [ ] **MCP-native observability** + auth tightening — [§P0-2](./recommendations.md#p0-2-mcp-native-observability-plus-auth-tightening)
- [ ] **Agent run replay** (time-travel) — [§P0-3](./recommendations.md#p0-3-agent-run-replay-time-travel)

P1 (3 months):

- [ ] Trajectory evals / agent-as-judge — [§P1-1](./recommendations.md#p1-1-trajectory-evals--agent-as-judge)
- [ ] Online eval (sampled prod scoring) — [§P1-2](./recommendations.md#p1-2-online-evals-sampled-production-scoring)
- [ ] Guardrails telemetry as a span kind — [§P1-3](./recommendations.md#p1-3-guardrails-telemetry-as-a-first-class-span-kind)
- [ ] Silent tool-failure detector — [§P1-4](./recommendations.md#p1-4-silent-tool-failure-detector)
- [ ] Cost-per-end-user attribution polish — [§P1-5](./recommendations.md#p1-5-cost-per-end-user--per-customer-attribution-polished)

P2 (strategic):

- [ ] Reasoning-trace analysis — [§P2-1](./recommendations.md#p2-1-reasoning-trace-analysis-for-extended-thinking-models)
- [ ] Synthetic dataset generation from prod — [§P2-2](./recommendations.md#p2-2-synthetic-dataset-generation-from-production-traces)
- [ ] HITL annotation queue — [§P2-3](./recommendations.md#p2-3-human-in-the-loop-annotation-queue)
- [ ] Agent memory observability — [§P2-4](./recommendations.md#p2-4-agent-memory-observability)
- [ ] Pydantic AI / Mastra / Google ADK auto-instrumentors — [§P2-5](./recommendations.md#p2-5-framework-auto-instrumentation-for-pydantic-ai-mastra-google-adk)

## Housekeeping

- [ ] **Roadmap refresh.** [`roadmap.md`](./roadmap.md) is 19 lines and covers only "What We Cover Well." Merge Tiers 1–5 above into a live roadmap, mark P0/P1 lanes.
- [ ] **Codemap / file index.** A 1-page doc listing every significant Go package, React component directory, and migration. Helps new contributors orient. `doc-updater` agent can regenerate on each release.
- [ ] **Release process.** Semantic version tags, GitHub release notes, published Docker images for `ghcr.io/<owner>/agentpulse-{backend,collector,web}`.
- [ ] **`CODE_OF_CONDUCT.md`** if the repo is ever made public.
- [ ] **Security disclosure policy** (`SECURITY.md`) — how people report vulnerabilities.

---

## How to use this file

- Treat it as the backlog. Cross items off with `- [x]` as they ship.
- If an item grows into real scope, promote it to an issue or a dedicated doc and leave a link here.
- When you complete a tier, trim it down to a one-line summary — this file should stay readable as things get done.
