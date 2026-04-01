# AgentPulse Roadmap

Last updated: 2026-03-30

Items #1–#12 are complete. This document covers everything remaining, consolidated from the original tier 2/3 items and the enterprise/security feedback round.

**Completed since last update (2026-03-30):**
- **B. PII / Secret Redaction** shipped: `piimaskerproc` collector processor with 14 built-in patterns; per-project toggle via `project_pii_configs` Postgres table; `GET/PUT /projects/{id}/settings` API (BearerAuth + AdminKeyAuth); Settings tab in frontend; Postgres LISTEN/NOTIFY for near-instant propagation; fail-closed on DB unavailability; customer-extensible regex rules with tautological/invalid rejection; `pii_redactions_count` span attribute; `make seed` seeds `customer-support-bot` with PII demo data

**Completed since last update (2026-03-29):**
- **Quality Gates** (item C below) shipped: `agentpulse eval check` CLI, `GET /evals/baseline` endpoint, rate limiting on project-scoped routes, GitHub Actions docs
- Semantic search security hardened: `escapeLike` bracket fix, rune-aware `extractSnippet`, `span_kind` enum validation, load-more accumulation bug fixed
- Streaming Span Support (#12) audited and confirmed fully complete
- **IDOR on run-scoped routes** fixed: `RunAuth` middleware, budget/alert rule ownership checks, webhook SSRF validation, `ListRecent` limit cap
- **A1** `ListRecent` data leak fixed: routes moved into authenticated project group, SQL scoped to `project_id`
- **A2** WebSocket project-scope gap fixed: `ServeWS` verifies Bearer token ownership before upgrading; shared `authutil` package extracted
- **A3** Rate limiter memory leak fixed: struct-based `RateLimiter` with background eviction ticker; run-scoped routes now rate-limited via `ProjectFromContext` fallback
- **A4** CORS wildcard tightened: `NewCORS()` factory reads `CORS_ALLOWED_ORIGINS` + `APP_ENV`; wildcard only in dev mode; prod uses per-request origin matching with `Vary: Origin`
- **A5** Collector rate limiting: `ratelimitproc` token-bucket processor (per `agentpulse.project_id`); first in pipeline before `batch`; fail-open for spans with no project ID; stale-bucket eviction

---

## Tier 1 — Production Safety (blockers)

### A. Security Hardening (remaining surfaces)

The IDOR fix shipped the highest-severity issues. Four lower-severity items remain, ordered by risk:

**A1. `ListRecent` endpoints unauthenticated** _(~2h)_
`GET /api/v1/budget/alerts/recent` and `GET /api/v1/alerts/events/recent` return cross-project alert data with no authentication. Any unauthenticated caller can enumerate alert names, thresholds, and webhook URLs across all projects.
- Fix: apply `BearerAuth` (project-scoped) or require a valid token and filter to that project only.

**A2. WebSocket auth project-scope gap** _(~2h)_
`GET /ws/alerts` validates a Bearer token inline but does not verify the token belongs to the project being streamed. A valid key for project-A can subscribe to project-B's real-time budget alerts.
- Fix: extract `project_id` query param, verify token ownership before upgrading the connection.

**A3. Rate limiter memory leak** _(~1h)_
The in-memory token-bucket map (`ratelimit.go`) grows unboundedly — one entry per project ID, never evicted. Under churn (many short-lived projects or fuzz input), this is an OOM vector.
- Fix: add a background ticker that evicts buckets idle for >10 minutes, or use a size-bounded LRU.

**A4. CORS wildcard** ✅ COMPLETE
- `NewCORS(allowedOrigins, devMode)` factory; `CORS_ALLOWED_ORIGINS` env var (comma-separated); `APP_ENV=development` (default) preserves wildcard; prod mode echoes matched origin + `Vary: Origin`; non-matching origins receive no ACAO header; startup warning when prod mode and origins unset; 5 tests

**A5. API Rate Limiting — collector** ✅ COMPLETE
- `ratelimitproc` processor: token-bucket per `agentpulse.project_id`; default 100/s, burst 200; first in pipeline (before `batch`); drops excess `ResourceSpan`s via `RemoveIf`; fail-open for spans with no project ID; background stale-bucket eviction; dev config uses 1000/s; 12 tests (race-clean)

**Priority order:** A1 and A2 before any external users see the product. A3 before sustained load. A4 deferred until JWT auth. A5 before cloud launch.

---

### B. PII / Secret Redaction ✅ COMPLETE

- `piimaskerproc` processor: 14 built-in patterns (credit card, SSN, email, JWT, OpenAI/Anthropic/AWS/GitHub/Stripe/Google/Slack API keys, PEM headers, US phone); combined alternation regex for early-exit; per-pattern `[REDACTED:<name>]` replacement
- Per-project toggle: `project_pii_configs` Postgres table; lazy row creation on first `PUT /settings`; `pii_redaction_enabled` + `pii_custom_rules JSONB`
- Separate `admin_key_hash` on `projects` (SDK Bearer token ≠ compliance settings key); `X-Admin-Key` header; `AdminKeyAuth` middleware
- `GET /projects/{id}/settings` (BearerAuth) + `PUT /projects/{id}/settings` (AdminKeyAuth); tautological/invalid regex rejected 400; max 20 custom rules; structured `slog` audit log; `pg_notify('pii_settings_changed')` for near-instant propagation
- Fail-closed: if Postgres unreachable at startup, built-in patterns applied to all spans
- Collector reads enabled projects via Postgres LISTEN/NOTIFY + 30s fallback poll; field allowlist skips `agentpulse.*` structural attributes and resource service attrs; stamps `agentpulse.pii_redactions_count` on redacted spans
- Frontend: Settings tab on project page; toggle with amber irreversibility warning; built-in pattern grid; custom rules table with add/remove + inline validation; read-only mode when admin key not in localStorage
- `make seed`: `customer-support-bot` project has PII redaction enabled; `scenarioSupportTriagePII` seeds realistic prompts/completions containing emails, credit cards, SSNs, API keys, phone numbers
- Migration: `migrations/postgres/006_project_pii_configs.up.sql`; pipeline order: `ratelimitproc → batch → agentsemanticproc → piimaskerproc → budgetenforceproc → clickhouseexporter`

---

## Tier 2 — Core Product Completeness

### C. Quality Gates ✅ DONE (2026-03-29)

- `GET /projects/{id}/evals/baseline` — avg score over last N runs, per eval type, rate-limited
- `agentpulse eval check` CLI — exit 0=pass, 1=fail, 2=error; `--threshold`, `--eval-type`, `--runs`, `--min-runs`, `--fail-open`, `--json` flags; SSRF-safe endpoint validation
- GitHub Actions example in `docs/github-actions-eval.yml` (3 job variants)
- 44 tests across CLI, handler, rate limiter, ClickHouse store

---

### D. Human-in-the-Loop Evals ✅ COMPLETE

**Why here:** the quality gate baseline today is driven purely by the LLM judge. Human thumbs-up/down on individual span outputs is the ground truth that corrects judge drift and builds a per-project calibration dataset. This is the natural follow-on to quality gates — it makes the scores you're gating on trustworthy.

**Implementation:**
- `POST /projects/{id}/runs/{runID}/spans/{spanID}/feedback` — `{rating: "good"|"bad", corrected_output?: string}`
- New `span_feedback` Postgres table (span_id, project_id, rating, corrected_output, created_at)
- Frontend: thumbs up/down buttons on span detail drawer; optional correction text box
- Feedback counts and weighted scores surfaced in eval summary
- `GET /evals/baseline` incorporates human ratings: human "bad" overrides judge score to 0; human "good" floors score at 0.8

**Effort:** ~2 days.

---

### E. Multi-Model Eval Scoring ✅ COMPLETE

**Why after HitL:** once human ratings exist, you have ground truth to compare judge models against. Running multiple judges and surfacing disagreement is most useful when you can already see which judge is wrong. Disagreement between judges is itself a signal — high variance flags spans for human review.

**User-facing behaviour:**
- Per-project setting: toggle "Multi-model evals"; select 2–3 judge models (e.g. Claude Haiku, GPT-4o Mini, Gemini Flash)
- Each model independently scores the span using the same eval prompt
- Eval summary shows per-model scores side-by-side with reasoning, plus consensus score (mean) and disagreement indicator (default threshold ±0.2)
- Quality Gates use the consensus score for pass/fail decisions

**Implementation pieces:**
- `eval_configs` gains `judge_models text[]` column (default `["claude-haiku-4-5"]`)
- Eval worker spawns parallel judge calls, one goroutine per model
- `015_eval_judge_model.sql` adds `judge_model` to `span_evals` ReplacingMergeTree key
- `GET /runs/{runID}/evals` groups scores by eval name, returns `[{model, score, reasoning}]` per eval
- Frontend: model-comparison table in span detail eval panel; consensus badge on collapsed view

**Effort:** ~4 days.

---

### F. Payload Offloading (Object Storage)

**Why:** `gen_ai.prompt` and `gen_ai.completion` are stored as string columns in ClickHouse. At production traffic with long-context models, these become the dominant storage cost and degrade query performance. The `S3Config` struct already exists in `config.go`.

**Implementation:**
- Above a configurable threshold (default 8 KB), the ClickHouse exporter writes the payload to S3 under `{project_id}/{run_id}/{span_id}.json` and stores a `payload_ref` pointer in ClickHouse instead
- Span detail API fetches from S3 only when a user opens a specific span — transparent to the query layer
- Zero change to SDK or collector processor pipeline

**Effort:** ~3 days.

---

### G. Team & Enterprise Auth Migration

**Goal:** migrate from single-user API-key model to multi-tenant org/team structure. Bearer tokens (SDK service credentials) and human JWT auth run as two permanent parallel auth paths — SDK integration never requires a login.

**Phase 1 + 3 (deliver together — these are the enterprise sales gate):**
- Phase 1: `users` table, JWT login/register, projects get `owner_user_id`, frontend replaces localStorage key with JWT session
- Phase 3: `viewer | editor | admin` roles enforced at API middleware; project-level role overrides; API key scopes (read-only vs full-write)

**Phase 2 (after Phase 1+3):**
- `organizations` + `org_members` tables, projects get `org_id` FK, org dashboard with cross-project cost rollup, member invite flow

**Phase 4 + 5 (enterprise contracts gate):**
- Phase 4: OIDC/SAML 2.0 (Okta, Google Workspace, Azure AD), JIT user provisioning, domain-based org auto-assignment
- Phase 5: audit log table, row-level security per org in Postgres, IP allowlisting, SOC2 controls, data retention policies

**Effort:** Phase 1+3 ~1.5 weeks; full 5 phases ~4 weeks.

---

## Tier 3 — Strategic Differentiators

### H. Agent Replay / Sandbox Debugging

**Why it's a moat:** no competitor does this. A developer can take a failed production trace and replay it locally with mocked tool responses (sourced from the original `tool.input`/`tool.output` span attributes), reproducing a prod failure deterministically.

**Architecture:**
- Replay engine: reads a run's span tree from the API, reconstructs the execution graph
- SDK "replay mode": intercepts real tool/LLM calls and substitutes recorded responses
- Configurable overrides: swap one tool's response to test a hypothesis
- UI: "Replay this run" button on run detail page, diff view showing original vs replay spans

**Effort:** ~2–3 weeks.

---

### I. Hardcoded Defaults Warning

Log a `WARN` at startup if `DATABASE_URL` contains `localhost` or the default `agentpulse:agentpulse` credentials. One-line change; do it opportunistically while touching config files for another item.

---

## Summary

| # | Item | Effort | Priority |
|---|------|--------|----------|
| 0 | IDOR on run routes + budget/alert ownership + webhook SSRF | ✅ Done | — |
| A1 | `ListRecent` unauthenticated (cross-project data leak) | ✅ Done | — |
| A2 | WebSocket auth project-scope gap | ✅ Done | — |
| A3 | Rate limiter memory leak | ✅ Done | — |
| A4 | CORS wildcard | ✅ Done | — |
| A5 | Collector rate limiting (`POST /v1/traces`) | ✅ Done | — |
| B | PII / Secret Redaction | ✅ Done | — |
| C | Quality Gates | ✅ Done | — |
| D | Human-in-the-Loop Evals | ✅ Done | — |
| E | Multi-Model Eval Scoring | ✅ Done | — |
| F | Payload Offloading (S3) | 3d | 🟠 Tier 2 |
| G | Team & Enterprise Auth (phases 1+3 first) | 1.5–4w | 🟠 Tier 2 |
| H | Agent Replay / Sandbox Debugging | 2–3w | 🟡 Tier 3 |
| I | Hardcoded defaults warning | 30m | 🟢 Opportunistic |
