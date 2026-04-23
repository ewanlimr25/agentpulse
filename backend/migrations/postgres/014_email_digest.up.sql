CREATE TABLE IF NOT EXISTS email_digest_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE UNIQUE,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    recipient_email TEXT NOT NULL DEFAULT '',
    schedule        TEXT NOT NULL DEFAULT 'daily',
    last_sent_at    TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
