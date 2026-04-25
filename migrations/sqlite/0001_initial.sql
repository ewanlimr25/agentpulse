-- AgentPulse: SQLite metadata schema (indie mode)
-- Mirrors migrations/postgres but uses SQLite-compatible types.
-- UUIDs are generated in Go (google/uuid) and passed as parameters.

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

-- ── Projects ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS projects (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    api_key_hash    TEXT NOT NULL UNIQUE,
    admin_key_hash  TEXT NOT NULL UNIQUE,
    loop_config     TEXT, -- JSON; NULL = use defaults
    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- ── Ingest tokens (per-project OTLP ingest credentials) ───────────────────────

CREATE TABLE IF NOT EXISTS project_ingest_tokens (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    label       TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_ingest_tokens_project ON project_ingest_tokens (project_id);
CREATE INDEX IF NOT EXISTS idx_ingest_tokens_hash    ON project_ingest_tokens (token_hash);

-- ── Topology: nodes + edges ───────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS topology_nodes (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    trace_id        TEXT NOT NULL,
    span_id         TEXT NOT NULL,

    node_type       TEXT NOT NULL CHECK (node_type IN ('agent', 'tool', 'llm', 'memory')),
    node_name       TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('ok', 'error', 'running', 'unset')),

    start_time      DATETIME,
    end_time        DATETIME,
    cost_usd        REAL NOT NULL DEFAULT 0,
    token_count     INTEGER NOT NULL DEFAULT 0,

    metadata        TEXT NOT NULL DEFAULT '{}',

    UNIQUE (project_id, run_id, span_id)
);

CREATE INDEX IF NOT EXISTS idx_topology_nodes_project_run ON topology_nodes (project_id, run_id);

CREATE TABLE IF NOT EXISTS topology_edges (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    run_id          TEXT NOT NULL,

    source_node_id  TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    target_node_id  TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,

    edge_type       TEXT NOT NULL CHECK (edge_type IN ('invocation', 'handoff', 'memory_access')),
    metadata        TEXT NOT NULL DEFAULT '{}',

    UNIQUE (project_id, run_id, source_node_id, target_node_id, edge_type)
);

CREATE INDEX IF NOT EXISTS idx_topology_edges_project_run ON topology_edges (project_id, run_id);

-- ── Budget rules + alerts ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS budget_rules (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    name            TEXT NOT NULL,
    threshold_usd   REAL NOT NULL CHECK (threshold_usd > 0),
    action          TEXT NOT NULL CHECK (action IN ('notify', 'halt')),

    scope           TEXT NOT NULL DEFAULT 'run' CHECK (scope IN ('run', 'agent', 'window', 'user')),
    scope_user_id   TEXT,
    window_seconds  INTEGER CHECK (window_seconds IS NULL OR window_seconds > 0),

    webhook_url     TEXT,
    enabled         INTEGER NOT NULL DEFAULT 1,

    created_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_budget_rules_project_enabled ON budget_rules (project_id) WHERE enabled = 1;

CREATE TABLE IF NOT EXISTS budget_alerts (
    id              TEXT PRIMARY KEY,
    rule_id         TEXT NOT NULL REFERENCES budget_rules(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id          TEXT,

    triggered_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    current_cost    REAL NOT NULL,
    threshold_usd   REAL NOT NULL,
    action_taken    TEXT NOT NULL CHECK (action_taken IN ('notify', 'halt')),

    metadata        TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_budget_alerts_project ON budget_alerts (project_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_budget_alerts_rule    ON budget_alerts (rule_id, triggered_at DESC);
