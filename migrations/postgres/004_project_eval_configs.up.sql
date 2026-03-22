-- Per-project eval type configuration.
-- Built-in eval types (relevance, hallucination, faithfulness, toxicity, tool_correctness)
-- use NULL prompt_template (the Go registry owns their prompts).
-- Custom eval types store their prompt template here.

CREATE TABLE IF NOT EXISTS project_eval_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    eval_name       TEXT NOT NULL,   -- 'relevance', 'hallucination', 'custom:brand_voice', …
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    span_kind       TEXT NOT NULL DEFAULT 'llm.call'
                    CHECK (span_kind IN ('llm.call', 'tool.call')),
    -- Custom evals only: prompt template with {{input}} / {{output}} placeholders.
    -- NULL means use the built-in Go implementation.
    prompt_template TEXT,
    prompt_version  INT NOT NULL DEFAULT 1,
    -- Optional JSON filter: {"agent_name": "researcher"}
    scope_filter    JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, eval_name)
);

CREATE INDEX IF NOT EXISTS idx_pec_project_enabled
    ON project_eval_configs (project_id) WHERE enabled = TRUE;
