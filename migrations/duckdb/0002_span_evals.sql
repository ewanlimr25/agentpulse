-- AgentPulse: DuckDB span_evals + audit_events tables (indie mode, P0-1 follow-up)
-- Mirrors migrations/clickhouse/004_span_evals.sql + 017_audit_events.up.sql.
-- DuckDB has no ReplacingMergeTree; we emulate via PRIMARY KEY + UPSERT in the
-- store layer (INSERT OR REPLACE … ON CONFLICT). TTL is enforced by the
-- retention enforcer, not the engine.

CREATE TABLE IF NOT EXISTS span_evals (
    project_id   VARCHAR NOT NULL,
    run_id       VARCHAR NOT NULL,
    span_id      VARCHAR NOT NULL,
    eval_name    VARCHAR NOT NULL,
    judge_model  VARCHAR NOT NULL DEFAULT '',
    score        DOUBLE  NOT NULL,
    reasoning    VARCHAR NOT NULL DEFAULT '',
    eval_version USMALLINT NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, run_id, span_id, eval_name, judge_model)
);

CREATE INDEX IF NOT EXISTS idx_span_evals_run_id   ON span_evals (run_id);
CREATE INDEX IF NOT EXISTS idx_span_evals_span_id  ON span_evals (span_id);

-- Audit events: append-only stream of admin/auth actions for compliance.
CREATE TABLE IF NOT EXISTS audit_events (
    event_id   VARCHAR NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    project_id VARCHAR NOT NULL DEFAULT '',
    actor      VARCHAR NOT NULL DEFAULT '',
    action     VARCHAR NOT NULL,
    target     VARCHAR NOT NULL DEFAULT '',
    metadata   VARCHAR NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_audit_events_project ON audit_events (project_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_time    ON audit_events (occurred_at);

-- Search-friendly precomputed lowercase columns + DuckDB FTS extension
-- (loaded on demand by the SearchStore at construction time).
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_prompt      VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_completion  VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_tool_input  VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_tool_output VARCHAR DEFAULT '';
