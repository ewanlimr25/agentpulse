CREATE TABLE run_tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id     TEXT NOT NULL,
    tag        TEXT NOT NULL CHECK (length(tag) BETWEEN 1 AND 64 AND tag ~ '^[a-zA-Z0-9_:\-\.]+$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, run_id, tag)
);
CREATE INDEX ON run_tags (project_id, run_id);
CREATE INDEX ON run_tags (project_id, tag);

CREATE TABLE run_annotations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id     TEXT NOT NULL,
    note       TEXT NOT NULL CHECK (length(note) <= 5000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, run_id)
);
CREATE INDEX ON run_annotations (project_id, run_id);
