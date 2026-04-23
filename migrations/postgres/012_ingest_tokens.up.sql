BEGIN;

CREATE TABLE IF NOT EXISTS project_ingest_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    label       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ingest_tokens_project ON project_ingest_tokens (project_id);
CREATE INDEX IF NOT EXISTS idx_ingest_tokens_hash ON project_ingest_tokens (token_hash);

COMMIT;
