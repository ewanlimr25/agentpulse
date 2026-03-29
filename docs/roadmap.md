# AgentPulse Roadmap

Last updated: 2026-03-29

Items #1–#12 are complete. This document covers everything remaining, consolidated from the original tier 2/3 items and the enterprise/security feedback round.

**Progress since last update (2026-03-29):**
- Quality Gates CI gate (automated half of item C) shipped: `agentpulse eval check` CLI, `GET /evals/baseline` endpoint, rate limiting on project-scoped routes, GitHub Actions docs
- Semantic search security hardened: `escapeLike` bracket fix, rune-aware `extractSnippet` (UTF-8 panic), `span_kind` enum validation, load-more accumulation bug fixed
- Streaming Span Support (#12) audited and confirmed fully complete

---

## Pre-Roadmap: Fix Now (security bug)

### IDOR on `/runs/{runID}` routes

The run-scoped API routes (`GET /runs/{runID}`, `/spans`, `/evals`, `/loops`, `/topology`) are currently unauthenticated. Anyone who discovers a run ID can read proprietary prompts and completions — a classic Insecure Direct Object Reference vulnerability.

**Fix:** resolve `project_id` from the run record in a middleware, verify the Bearer token matches that project, 403 otherwise. Same pattern already used by all project-scoped routes.

**Effort:** ~2 hours. Should happen before shipping to any external user.

---

## Tier 1 — Production Safety (blockers)

### A. API Rate Limiting

**Why first:** a stuck agent in an infinite loop will hammer the collector and API right now. No throttling exists anywhere in the stack.

**Surfaces to protect:**
- `POST /v1/traces` in the collector — token-bucket per `agentpulse.project_id` attribute; drop/429 excess batches
- Backend API — per-IP or per-Bearer-token limits using `go-chi/httprate`; tighter limits on expensive endpoints (search, compare)

**Effort:** ~1 day.

---

### B. PII / Secret Redaction

**Why second:** the #1 enterprise adoption blocker. LLM prompts carry user names, emails, customer data, and accidentally stringified API keys. Without redaction, no enterprise team can legally point their agents at AgentPulse.

**Implementation:** new `piimaskerproc` collector processor inserted between `agentsemanticproc` and `clickhouseexporter`. Config-driven YAML rules:
- Built-in patterns: credit card numbers, SSNs, email addresses, JWT tokens, API key prefixes (`sk-`, `Bearer `, etc.)
- Customer-extensible: additional regex rules in `config/pii_rules.yaml`
- Field allowlist: certain attributes (`agentpulse.run_id`, `agentpulse.span_kind`) are never scanned
- Replacement format: `[REDACTED:<type>]` (e.g. `[REDACTED:email]`)

**Effort:** ~3 days.

---

## Tier 2 — Core Product Completeness

### C. Quality Gates + Human-in-the-Loop Evals

These are the same system from two angles — automated CI gates need human feedback as ground truth.

**Quality Gates — ✅ DONE (2026-03-29)**
- `GET /projects/{id}/evals/baseline` endpoint — avg score over last N runs, per eval type
- `agentpulse eval check` CLI — exit 0=pass, 1=fail, 2=error; `--threshold`, `--eval-type`, `--runs`, `--min-runs`, `--fail-open`, `--json` flags
- GitHub Actions example in `docs/github-actions-eval.yml`
- Rate limiting (60 req/min per project) on all project-scoped routes

**Human-in-the-Loop — still needed:**
- `POST /projects/{id}/runs/{runID}/spans/{spanID}/feedback` — `{rating: "good"|"bad", corrected_output?: string}`
- New `span_feedback` Postgres table
- Frontend: thumbs up/down on span detail drawer, correction text box
- Feedback counts and weighted scores surfaced in eval summary; quality gate baseline incorporates ratings

**Effort:** ~2 days (HitL portion only).

---

### D. Payload Offloading (Object Storage)

**Why:** `gen_ai.prompt` and `gen_ai.completion` are stored as string columns in ClickHouse. At production traffic with long-context models, these become the dominant storage cost and degrade query performance. The `S3Config` struct already exists in `config.go`.

**Implementation:**
- Above a configurable threshold (default 8 KB), the ClickHouse exporter writes the payload to S3 under `{project_id}/{run_id}/{span_id}.json` and stores a `payload_ref` pointer in ClickHouse instead
- Span detail API fetches from S3 only when a user opens a specific span — transparent to the query layer
- Zero change to SDK or collector processor pipeline

**Effort:** ~3 days.

---

### E. Team & Enterprise Auth Migration

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

### F. Agent Replay / Sandbox Debugging

**Why it's a moat:** no competitor does this. A developer can take a failed production trace and replay it locally with mocked tool responses (sourced from the original `tool.input`/`tool.output` span attributes), reproducing a prod failure deterministically.

**Architecture:**
- Replay engine: reads a run's span tree from the API, reconstructs the execution graph
- SDK "replay mode": intercepts real tool/LLM calls and substitutes recorded responses
- Configurable overrides: swap one tool's response to test a hypothesis
- UI: "Replay this run" button on run detail page, diff view showing original vs replay spans

**Effort:** ~2–3 weeks.

---

### G. Multi-Model Eval Scoring

**Why it matters:** a single LLM judge (currently Claude Haiku) introduces model-specific bias — Haiku may consistently score its own outputs higher, miss subtle reasoning errors, or have blind spots for certain domains. Letting users configure multiple judge models and comparing their verdicts gives a much more reliable signal. Disagreement between models is itself a useful signal — high variance across judges flags spans worth human review.

**User-facing behaviour:**
- Per-project setting: toggle "Multi-model evals" on/off; when on, select 2–3 judge models from a dropdown (e.g. Claude Haiku, GPT-4o Mini, Gemini Flash)
- Each configured model independently scores the span using the same eval prompt
- `span_evals` stores one row per `(span_id, eval_name, judge_model)` — the existing `ReplacingMergeTree` dedup key needs `judge_model` added
- Eval summary shows per-model scores side-by-side with each model's full reasoning text, plus a computed consensus score (mean) and a disagreement indicator when scores diverge by more than a configurable threshold (default ±0.2)
- Quality Gates (item C) use the consensus score for pass/fail decisions

**Implementation pieces:**
- `eval_configs` Postgres table gains a `judge_models` `text[]` column (default `["claude-haiku-4-5"]`)
- Eval worker spawns parallel judge calls, one goroutine per model; writes a row for each
- New `EvalJudgeModel` field on `SpanEval` domain type and ClickHouse schema (`015_eval_judge_model.sql`)
- `GET /runs/{runID}/evals` response groups scores by eval name, returns array of `{model, score, reasoning}` per eval
- Frontend: eval panel in span detail drawer expands to show a model-comparison table; consensus badge on collapsed view

**Effort:** ~4 days.

---

### H. Hardcoded Defaults Warning

Log a `WARN` at startup if `DATABASE_URL` contains `localhost` or the default `agentpulse:agentpulse` credentials. One-line change; do it opportunistically while touching config files for another item.

---

## Summary

| # | Item | Effort | Priority |
|---|------|--------|----------|
| 0 | Fix IDOR on run routes | ~2h | 🔴 Fix now |
| A | API Rate Limiting | 1d | 🔴 Tier 1 |
| B | PII / Secret Redaction | 3d | 🔴 Tier 1 |
| C | Quality Gates + HitL Evals | 4d | 🟠 Tier 2 |
| D | Payload Offloading (S3) | 3d | 🟠 Tier 2 |
| E | Team & Enterprise Auth (phases 1+3 first) | 1.5–4w | 🟠 Tier 2 |
| F | Agent Replay | 2–3w | 🟡 Tier 3 |
| G | Multi-Model Eval Scoring | 4d | 🟡 Tier 3 |
| H | Hardcoded defaults warning | 30m | 🟢 Opportunistic |
