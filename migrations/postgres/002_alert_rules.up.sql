-- Multi-signal alert rules and events

BEGIN;

-- ── Alert rules ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS alert_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    name            TEXT NOT NULL,
    signal_type     TEXT NOT NULL CHECK (signal_type IN ('error_rate', 'latency_p95', 'quality_score', 'tool_failure')),
    threshold       DOUBLE PRECISION NOT NULL CHECK (threshold >= 0),
    compare_op      TEXT NOT NULL CHECK (compare_op IN ('gt', 'lt')),

    window_seconds  INTEGER NOT NULL CHECK (window_seconds > 0),
    scope_filter    TEXT,   -- tool name; required when signal_type = 'tool_failure'

    webhook_url     TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT true,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_project ON alert_rules (project_id) WHERE enabled = true;

-- ── Alert events ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS alert_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    triggered_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    signal_type     TEXT NOT NULL,
    current_value   DOUBLE PRECISION NOT NULL,
    threshold       DOUBLE PRECISION NOT NULL,
    compare_op      TEXT NOT NULL,
    action_taken    TEXT NOT NULL DEFAULT 'notify',

    metadata        JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_alert_events_project ON alert_events (project_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_rule    ON alert_events (rule_id, triggered_at DESC);

COMMIT;
