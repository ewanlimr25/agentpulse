-- AgentPulse: full SQLite metadata schema (indie mode, P0-1 follow-up)
-- Mirrors migrations/postgres/002–016 in a SQLite-compatible dialect.
-- Conventions:
--   - UUIDs are TEXT (generated in Go, google/uuid).
--   - JSONB is TEXT (callers Marshal/Unmarshal in Go).
--   - bool is INTEGER (0/1).
--   - now() is strftime('%Y-%m-%dT%H:%M:%fZ','now') for ISO-8601 millisecond UTC.
--   - $1/$2/... → ? in store code; CHECK constraints kept verbatim where possible.

PRAGMA foreign_keys = ON;

-- ── Alert rules + events ──────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS alert_rules (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    name                TEXT NOT NULL,
    signal_type         TEXT NOT NULL CHECK (signal_type IN ('error_rate', 'latency_p95', 'quality_score', 'tool_failure', 'agent_loop')),
    threshold           REAL NOT NULL CHECK (threshold >= 0),
    compare_op          TEXT NOT NULL CHECK (compare_op IN ('gt', 'lt')),

    window_seconds      INTEGER NOT NULL CHECK (window_seconds > 0),
    scope_filter        TEXT,

    webhook_url         TEXT,
    webhook_secret      TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,

    slack_webhook_url   TEXT,
    discord_webhook_url TEXT,
    last_channel_error  TEXT,
    last_channel_error_at DATETIME,

    created_at          DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at          DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_project_enabled ON alert_rules (project_id) WHERE enabled = 1;

CREATE TABLE IF NOT EXISTS alert_events (
    id              TEXT PRIMARY KEY,
    rule_id         TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    triggered_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    signal_type     TEXT NOT NULL,
    current_value   REAL NOT NULL,
    threshold       REAL NOT NULL,
    compare_op      TEXT NOT NULL,
    action_taken    TEXT NOT NULL DEFAULT 'notify',

    metadata        TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_alert_events_project ON alert_events (project_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_rule    ON alert_events (rule_id, triggered_at DESC);

-- ── Eval jobs (work queue) ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS eval_jobs (
    id          TEXT PRIMARY KEY,
    span_id     TEXT NOT NULL,
    run_id      TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    eval_name   TEXT NOT NULL DEFAULT 'relevance',
    judge_model TEXT NOT NULL DEFAULT 'claude-haiku-4-5',
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','running','done','failed')),
    attempts    INTEGER NOT NULL DEFAULT 0,
    error       TEXT,
    created_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (span_id, eval_name, judge_model)
);

CREATE INDEX IF NOT EXISTS idx_eval_jobs_pending ON eval_jobs (created_at ASC) WHERE status IN ('pending', 'failed');

-- ── Project eval configs (per-project eval type configuration) ───────────────

CREATE TABLE IF NOT EXISTS project_eval_configs (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    eval_name       TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    span_kind       TEXT NOT NULL DEFAULT 'llm.call'
                    CHECK (span_kind IN ('llm.call', 'tool.call')),
    prompt_template TEXT,
    prompt_version  INTEGER NOT NULL DEFAULT 1,
    judge_models    TEXT NOT NULL DEFAULT '["claude-haiku-4-5"]', -- JSON array
    scope_filter    TEXT NOT NULL DEFAULT '{}',                    -- JSON
    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (project_id, eval_name)
);

CREATE INDEX IF NOT EXISTS idx_pec_project_enabled ON project_eval_configs (project_id) WHERE enabled = 1;

-- ── PII config per project ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS project_pii_configs (
    project_id            TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    pii_redaction_enabled INTEGER NOT NULL DEFAULT 0,
    pii_custom_rules      TEXT NOT NULL DEFAULT '[]', -- JSON array
    created_at            DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at            DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- ── Span feedback (HITL) ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS span_feedback (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    span_id          TEXT NOT NULL,
    run_id           TEXT NOT NULL,
    rating           TEXT NOT NULL CHECK (rating IN ('good', 'bad')),
    corrected_output TEXT CHECK (corrected_output IS NULL OR length(corrected_output) <= 10000),
    created_at       DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at       DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (project_id, span_id)
);

CREATE INDEX IF NOT EXISTS idx_span_feedback_project_run     ON span_feedback (project_id, run_id);
CREATE INDEX IF NOT EXISTS idx_span_feedback_project_created ON span_feedback (project_id, created_at DESC);

-- ── Prompt playground ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS prompt_playground_sessions (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    source_span_id TEXT,
    source_run_id  TEXT,
    created_at     DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at     DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_playground_sessions_project ON prompt_playground_sessions (project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS prompt_playground_variants (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES prompt_playground_sessions(id) ON DELETE CASCADE,
    label         TEXT NOT NULL,
    model_id      TEXT NOT NULL,
    system_prompt TEXT,
    messages      TEXT NOT NULL,                    -- JSON array
    temperature   REAL,
    max_tokens    INTEGER,
    updated_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_playground_variants_session ON prompt_playground_variants (session_id);

CREATE TABLE IF NOT EXISTS prompt_playground_executions (
    id            TEXT PRIMARY KEY,
    variant_id    TEXT NOT NULL REFERENCES prompt_playground_variants(id) ON DELETE CASCADE,
    output        TEXT,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd      REAL NOT NULL DEFAULT 0,
    latency_ms    INTEGER NOT NULL DEFAULT 0,
    error         TEXT,
    created_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_playground_executions_variant ON prompt_playground_executions (variant_id, created_at DESC);

-- ── Run tags + annotations ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS run_tags (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id     TEXT NOT NULL,
    tag        TEXT NOT NULL CHECK (length(tag) BETWEEN 1 AND 64),
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (project_id, run_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_run_tags_project_run ON run_tags (project_id, run_id);
CREATE INDEX IF NOT EXISTS idx_run_tags_project_tag ON run_tags (project_id, tag);

CREATE TABLE IF NOT EXISTS run_annotations (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id     TEXT NOT NULL,
    note       TEXT NOT NULL CHECK (length(note) <= 5000),
    created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (project_id, run_id)
);
CREATE INDEX IF NOT EXISTS idx_run_annotations_project_run ON run_annotations (project_id, run_id);

-- ── Retention + purge jobs ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS project_retention_config (
    project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    retention_days INTEGER NOT NULL DEFAULT 30 CHECK (retention_days >= 7 AND retention_days <= 365),
    updated_at     DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS purge_jobs (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id          TEXT,
    cutoff_date     DATETIME,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed')),
    include_evals   INTEGER NOT NULL DEFAULT 0,
    spans_deleted   INTEGER NOT NULL DEFAULT 0,
    s3_keys_deleted INTEGER NOT NULL DEFAULT 0,
    pg_rows_deleted INTEGER NOT NULL DEFAULT 0,
    partial_failure INTEGER NOT NULL DEFAULT 0,
    error_msg       TEXT,
    started_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    completed_at    DATETIME
);

CREATE INDEX IF NOT EXISTS idx_purge_jobs_project ON purge_jobs (project_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_purge_jobs_status  ON purge_jobs (status) WHERE status IN ('pending','running');

-- ── Web push + email digest ──────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    endpoint         TEXT NOT NULL,
    p256dh_key       TEXT NOT NULL,
    auth_key          TEXT NOT NULL,
    vapid_public_key TEXT NOT NULL,
    user_agent       TEXT,
    created_at       DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (project_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_project ON push_subscriptions(project_id);

CREATE TABLE IF NOT EXISTS email_digest_configs (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE UNIQUE,
    enabled         INTEGER NOT NULL DEFAULT 0,
    recipient_email TEXT NOT NULL DEFAULT '',
    schedule        TEXT NOT NULL DEFAULT 'daily',
    last_sent_at    DATETIME,
    last_error      TEXT,
    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- ── Run loops ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS run_loops (
    id               TEXT PRIMARY KEY,
    run_id           TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    detection_type   TEXT NOT NULL,
    span_name        TEXT NOT NULL DEFAULT '',
    input_hash       TEXT NOT NULL DEFAULT '0',
    output_hash      TEXT NOT NULL DEFAULT '0',
    confidence       TEXT NOT NULL,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    detected_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (run_id, detection_type, span_name, input_hash, output_hash)
);
CREATE INDEX IF NOT EXISTS idx_run_loops_project_detected ON run_loops (project_id, detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_run_loops_run_id           ON run_loops (run_id);

CREATE TABLE IF NOT EXISTS loop_detector_watermarks (
    project_id      TEXT PRIMARY KEY,
    last_scanned_at DATETIME NOT NULL
);
