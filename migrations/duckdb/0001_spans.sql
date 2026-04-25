-- AgentPulse: DuckDB spans schema (indie mode)
-- Mirrors migrations/clickhouse/001_spans.sql but uses DuckDB-compatible types.
-- Drops MergeTree-specific syntax (PARTITION BY, ORDER BY, TTL, LowCardinality, FixedString, MATERIALIZED).
-- DuckDB MAP(VARCHAR, VARCHAR) replaces ClickHouse Map(String, String).

CREATE TABLE IF NOT EXISTS spans (
    -- OTel identifiers
    trace_id          VARCHAR NOT NULL,
    span_id           VARCHAR NOT NULL,
    parent_span_id    VARCHAR DEFAULT '',

    -- Logical grouping
    run_id            VARCHAR NOT NULL,
    project_id        VARCHAR NOT NULL,
    session_id        VARCHAR DEFAULT '',
    user_id           VARCHAR DEFAULT '',

    -- Denormalized agent-semantic fields (extracted at ingest by agentsemanticproc)
    agent_span_kind   VARCHAR DEFAULT 'unknown',
    agent_name        VARCHAR DEFAULT '',
    model_id          VARCHAR DEFAULT '',

    -- Standard OTel fields
    span_name         VARCHAR NOT NULL,
    service_name      VARCHAR DEFAULT '',
    status_code       VARCHAR DEFAULT 'UNSET',
    status_message    VARCHAR DEFAULT '',

    -- Timing (TIMESTAMPTZ stores UTC instants in DuckDB)
    start_time        TIMESTAMPTZ NOT NULL,
    end_time          TIMESTAMPTZ NOT NULL,

    -- Token usage & cost
    input_tokens      UINTEGER DEFAULT 0,
    output_tokens     UINTEGER DEFAULT 0,
    cost_usd          DOUBLE DEFAULT 0.0,

    -- Streaming metric
    ttft_ms           DOUBLE DEFAULT 0.0,

    -- Flexible attribute bags (stored as JSON strings; AgentPulse code Marshal/Unmarshal them)
    -- We use VARCHAR (not DuckDB native MAP) because:
    --  - The collector / exporter today writes Map(String,String) and AgentPulse reads as map[string]string;
    --    DuckDB's MAP serialization adds parsing complexity. Storing as JSON is simpler and round-trips cleanly.
    attributes        VARCHAR DEFAULT '{}',
    resource_attrs    VARCHAR DEFAULT '{}',
    events            VARCHAR DEFAULT '[]',

    -- Payload offload pointer (filesystem key in indie mode, S3 key in team mode)
    payload_s3_key    VARCHAR DEFAULT ''
);

-- Auxiliary indexes for common access patterns. DuckDB uses min-max zone maps + ART indexes.
CREATE INDEX IF NOT EXISTS idx_spans_run_id     ON spans (run_id);
CREATE INDEX IF NOT EXISTS idx_spans_project_id ON spans (project_id);
CREATE INDEX IF NOT EXISTS idx_spans_start_time ON spans (start_time);
CREATE INDEX IF NOT EXISTS idx_spans_span_id    ON spans (span_id);
