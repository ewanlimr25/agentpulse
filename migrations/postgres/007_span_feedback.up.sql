-- Human-in-the-loop eval feedback per span.
-- One feedback record per (project_id, span_id) — upsert semantics.

BEGIN;

CREATE TABLE IF NOT EXISTS span_feedback (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       TEXT        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    span_id          TEXT        NOT NULL,
    run_id           TEXT        NOT NULL,
    rating           TEXT        NOT NULL CHECK (rating IN ('good', 'bad')),
    -- corrected_output is plain text only; length capped here as defence-in-depth.
    corrected_output TEXT        CHECK (corrected_output IS NULL OR length(corrected_output) <= 10000),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, span_id)
);

CREATE INDEX IF NOT EXISTS span_feedback_project_run
    ON span_feedback (project_id, run_id);

CREATE INDEX IF NOT EXISTS span_feedback_project_created
    ON span_feedback (project_id, created_at DESC);

COMMIT;
