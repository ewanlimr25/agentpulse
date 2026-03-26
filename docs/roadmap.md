# AgentPulse Roadmap

Last updated: 2026-03-26

Items #1–#12 are complete. This document covers everything remaining, consolidated from the original tier 2/3 items and the enterprise/security feedback round.

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

These are the same system from two angles — automated CI gates need human feedback as ground truth. Build together.

**Human-in-the-Loop:**
- `POST /projects/{id}/runs/{runID}/spans/{spanID}/feedback` — `{rating: "good"|"bad", corrected_output?: string}`
- New `span_feedback` Postgres table
- Frontend: thumbs up/down on span detail drawer, correction text box
- Feedback counts and weighted scores surfaced in eval summary

**Quality Gates:**
- `GET /projects/{id}/evals/baseline` endpoint — avg score over last N runs, incorporating human feedback ratings
- `agentpulse eval check --project X --threshold 0.65` CLI command with CI-friendly exit codes (0 = pass, 1 = fail)
- GitHub Actions example in docs — fail PR if staging eval score drops >10% vs main baseline

**Effort:** ~4 days.

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

### G. Hardcoded Defaults Warning

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
| G | Hardcoded defaults warning | 30m | 🟢 Opportunistic |
