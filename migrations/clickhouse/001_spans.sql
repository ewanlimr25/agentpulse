-- AgentPulse: spans table
-- Stores all OTel spans with agent semantic enrichment.
-- Attributes stored as Map(String, String) for OTel GenAI SIG extensibility.

CREATE TABLE IF NOT EXISTS spans
(
    -- OTel identifiers
    trace_id          FixedString(32),
    span_id           FixedString(16),
    parent_span_id    FixedString(16) DEFAULT '',

    -- Logical grouping
    run_id            String,
    project_id        String,

    -- Denormalized agent-semantic fields (extracted at ingest by agentsemanticproc)
    agent_span_kind   LowCardinality(String) DEFAULT 'unknown',
    agent_name        String DEFAULT '',
    model_id          String DEFAULT '',

    -- Standard OTel fields
    span_name         String,
    service_name      LowCardinality(String) DEFAULT '',
    status_code       LowCardinality(String) DEFAULT 'UNSET', -- OK | ERROR | UNSET
    status_message    String DEFAULT '',

    -- Timing
    start_time        DateTime64(9, 'UTC'),
    end_time          DateTime64(9, 'UTC'),
    duration_ns       UInt64 MATERIALIZED toUnixTimestamp64Nano(end_time) - toUnixTimestamp64Nano(start_time),

    -- Token usage & cost (computed by agentsemanticproc)
    input_tokens      UInt32 DEFAULT 0,
    output_tokens     UInt32 DEFAULT 0,
    total_tokens      UInt32 MATERIALIZED input_tokens + output_tokens,
    cost_usd          Float64 DEFAULT 0.0,

    -- Flexible attribute bags (preserves all OTel attributes regardless of schema)
    attributes        Map(String, String),
    resource_attrs    Map(String, String),
    events            String DEFAULT '[]', -- JSON array of span events

    -- Partition helper
    _date             Date DEFAULT toDate(start_time)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(_date)
ORDER BY (project_id, _date, trace_id, start_time)
TTL _date + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Bloom filter index for direct run_id lookups
ALTER TABLE spans ADD INDEX idx_run_id run_id TYPE bloom_filter GRANULARITY 1;

-- Bloom filter index for span kind filtering
ALTER TABLE spans ADD INDEX idx_span_kind agent_span_kind TYPE bloom_filter GRANULARITY 1;

-- Bloom filter index for error filtering
ALTER TABLE spans ADD INDEX idx_status status_code TYPE bloom_filter GRANULARITY 1;
