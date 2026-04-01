-- PII/Secret redaction configuration per project.
-- pii_custom_rules is a JSONB array of {name, pattern, enabled} objects.

BEGIN;

ALTER TABLE projects ADD COLUMN IF NOT EXISTS admin_key_hash TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS project_pii_configs (
    project_id            UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    pii_redaction_enabled BOOLEAN NOT NULL DEFAULT false,
    pii_custom_rules      JSONB NOT NULL DEFAULT '[]',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
