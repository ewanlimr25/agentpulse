BEGIN;
CREATE TABLE IF NOT EXISTS eval_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    span_id     TEXT NOT NULL,
    run_id      TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    eval_name   TEXT NOT NULL DEFAULT 'relevance',
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','running','done','failed')),
    attempts    INT NOT NULL DEFAULT 0,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (span_id, eval_name)
);
CREATE INDEX IF NOT EXISTS idx_eval_jobs_pending ON eval_jobs (created_at ASC)
    WHERE status IN ('pending', 'failed');
COMMIT;
