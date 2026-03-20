-- AgentPulse: per-run aggregated metrics
-- Materialized view over spans; updated automatically on insert.

CREATE TABLE IF NOT EXISTS metrics_agg
(
    project_id            String,
    run_id                String,

    total_spans           AggregateFunction(count, UInt8),
    total_llm_calls       AggregateFunction(count, UInt8),
    total_tool_calls      AggregateFunction(count, UInt8),
    total_handoffs        AggregateFunction(count, UInt8),
    error_count           AggregateFunction(count, UInt8),

    total_input_tokens    AggregateFunction(sum, UInt32),
    total_output_tokens   AggregateFunction(sum, UInt32),
    total_cost_usd        AggregateFunction(sum, Float64),

    latency_quantiles     AggregateFunction(quantiles(0.5, 0.95, 0.99), UInt64),
    total_duration_ns     AggregateFunction(sum, UInt64),

    first_span_time       AggregateFunction(min, DateTime64(9, 'UTC')),
    last_span_time        AggregateFunction(max, DateTime64(9, 'UTC')),

    _date                 Date
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(_date)
ORDER BY (project_id, run_id);

-- Materialized view that populates metrics_agg from spans
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_agg_mv
TO metrics_agg
AS
SELECT
    project_id,
    run_id,
    countState()                                                    AS total_spans,
    countStateIf(agent_span_kind = 'llm.call')                     AS total_llm_calls,
    countStateIf(agent_span_kind = 'tool.call')                    AS total_tool_calls,
    countStateIf(agent_span_kind = 'agent.handoff')                AS total_handoffs,
    countStateIf(status_code = 'ERROR')                            AS error_count,
    sumState(input_tokens)                                         AS total_input_tokens,
    sumState(output_tokens)                                        AS total_output_tokens,
    sumState(cost_usd)                                             AS total_cost_usd,
    quantilesState(0.5, 0.95, 0.99)(duration_ns)                  AS latency_quantiles,
    sumState(duration_ns)                                          AS total_duration_ns,
    minState(start_time)                                           AS first_span_time,
    maxState(end_time)                                             AS last_span_time,
    toDate(start_time)                                             AS _date
FROM spans
GROUP BY project_id, run_id, _date;
