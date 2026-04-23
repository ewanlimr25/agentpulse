CREATE TABLE IF NOT EXISTS push_subscriptions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    endpoint         TEXT NOT NULL,
    p256dh_key       TEXT NOT NULL,
    auth_key         TEXT NOT NULL,
    vapid_public_key TEXT NOT NULL,
    user_agent       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, endpoint)
);
CREATE INDEX ON push_subscriptions(project_id);
