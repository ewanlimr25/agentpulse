-- AgentPulse: initial Postgres schema
-- Projects, topology (nodes + edges), budget rules, alert history.

BEGIN;

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ── Projects ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    api_key_hash    TEXT NOT NULL UNIQUE,  -- SHA-256 hex of the raw API key
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Topology: nodes ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS topology_nodes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      TEXT NOT NULL,   -- plain string from span attributes; no FK so collector can write freely
    run_id          TEXT NOT NULL,
    trace_id        TEXT NOT NULL,
    span_id         TEXT NOT NULL,

    node_type       TEXT NOT NULL CHECK (node_type IN ('agent', 'tool', 'llm', 'memory')),
    node_name       TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('ok', 'error', 'running', 'unset')),

    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    cost_usd        DOUBLE PRECISION NOT NULL DEFAULT 0,
    token_count     INTEGER NOT NULL DEFAULT 0,

    metadata        JSONB NOT NULL DEFAULT '{}',

    UNIQUE (project_id, run_id, span_id)
);

CREATE INDEX IF NOT EXISTS idx_topology_nodes_project_run ON topology_nodes (project_id, run_id);

-- ── Topology: edges ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS topology_edges (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      TEXT NOT NULL,   -- plain string; matches topology_nodes.project_id
    run_id          TEXT NOT NULL,

    source_node_id  UUID NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    target_node_id  UUID NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,

    edge_type       TEXT NOT NULL CHECK (edge_type IN ('invocation', 'handoff', 'memory_access')),

    metadata        JSONB NOT NULL DEFAULT '{}',

    UNIQUE (project_id, run_id, source_node_id, target_node_id, edge_type)
);

CREATE INDEX IF NOT EXISTS idx_topology_edges_project_run ON topology_edges (project_id, run_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_source ON topology_edges (source_node_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_target ON topology_edges (target_node_id);

-- ── Budget rules ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS budget_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    name            TEXT NOT NULL,
    threshold_usd   DOUBLE PRECISION NOT NULL CHECK (threshold_usd > 0),
    action          TEXT NOT NULL CHECK (action IN ('notify', 'halt')),

    scope           TEXT NOT NULL DEFAULT 'run' CHECK (scope IN ('run', 'agent', 'window')),
    window_seconds  INTEGER CHECK (window_seconds IS NULL OR window_seconds > 0),

    webhook_url     TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT true,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_budget_rules_project ON budget_rules (project_id) WHERE enabled = true;

-- ── Budget alerts ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS budget_alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         UUID NOT NULL REFERENCES budget_rules(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id          TEXT,

    triggered_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    current_cost    DOUBLE PRECISION NOT NULL,
    threshold_usd   DOUBLE PRECISION NOT NULL,
    action_taken    TEXT NOT NULL CHECK (action_taken IN ('notify', 'halt')),

    metadata        JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_budget_alerts_project ON budget_alerts (project_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_budget_alerts_rule ON budget_alerts (rule_id, triggered_at DESC);

COMMIT;
