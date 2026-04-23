BEGIN;

CREATE TABLE IF NOT EXISTS project_retention_config (
    project_id      UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    retention_days  INTEGER NOT NULL DEFAULT 30 CHECK (retention_days >= 7 AND retention_days <= 365),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS purge_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id          TEXT,
    cutoff_date     TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed')),
    include_evals   BOOLEAN NOT NULL DEFAULT false,
    spans_deleted   BIGINT NOT NULL DEFAULT 0,
    s3_keys_deleted BIGINT NOT NULL DEFAULT 0,
    pg_rows_deleted BIGINT NOT NULL DEFAULT 0,
    partial_failure BOOLEAN NOT NULL DEFAULT false,
    error_msg       TEXT,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_purge_jobs_project ON purge_jobs (project_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_purge_jobs_status ON purge_jobs (status) WHERE status IN ('pending','running');

COMMIT;
