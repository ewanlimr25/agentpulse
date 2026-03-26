-- AgentPulse: convert run_metrics from a plain VIEW to an AggregatingMergeTree
-- materialized view, adding ttft_p50_ms, ttft_p95_ms, and streaming_span_count.
--
-- Pattern mirrors session_agg (008) and user_agg (011).
-- The old run_metrics VIEW is dropped and replaced by:
--   run_metrics_agg  — AggregatingMergeTree backing table
--   run_metrics_agg_mv — materialized view populating run_metrics_agg
--   run_metrics      — query VIEW over run_metrics_agg (backward-compatible)

-- Step 1: drop the existing plain VIEW so we can replace it.
DROP VIEW IF EXISTS run_metrics;

-- Step 2: backing AggregatingMergeTree table.
CREATE TABLE IF NOT EXISTS run_metrics_agg
(
    run_id          String,
    project_id      String,

    trace_id_state      AggregateFunction(anyLast, String),
    session_id_state    AggregateFunction(anyLast, String),
    user_id_state       AggregateFunction(anyLast, String),
    min_start_state     AggregateFunction(min, DateTime64(9, 'UTC')),
    max_end_state       AggregateFunction(max, DateTime64(9, 'UTC')),
    span_count_state    AggregateFunction(count, UInt8),
    llm_calls_state     AggregateFunction(countIf, UInt8),
    tool_calls_state    AggregateFunction(countIf, UInt8),
    input_tokens_state  AggregateFunction(sum, UInt64),
    output_tokens_state AggregateFunction(sum, UInt64),
    total_tokens_state  AggregateFunction(sum, UInt64),
    total_cost_state    AggregateFunction(sum, Float64),
    error_count_state   AggregateFunction(countIf, UInt8),
    ttft_quantiles_state AggregateFunction(quantilesIf(0.50, 0.95), Float64),
    streaming_span_count_state AggregateFunction(countIf, UInt8)
)
ENGINE = AggregatingMergeTree()
ORDER BY (project_id, run_id);

-- Step 3: materialized view that incrementally populates run_metrics_agg.
CREATE MATERIALIZED VIEW IF NOT EXISTS run_metrics_agg_mv
TO run_metrics_agg
AS SELECT
    run_id,
    project_id,
    anyLastState(trace_id)                                              AS trace_id_state,
    anyLastState(session_id)                                            AS session_id_state,
    anyLastState(user_id)                                               AS user_id_state,
    minState(start_time)                                                AS min_start_state,
    maxState(end_time)                                                  AS max_end_state,
    countState()                                                        AS span_count_state,
    countIfState(agent_span_kind = 'llm.call')                         AS llm_calls_state,
    countIfState(agent_span_kind = 'tool.call')                        AS tool_calls_state,
    sumState(toUInt64(input_tokens))                                    AS input_tokens_state,
    sumState(toUInt64(output_tokens))                                   AS output_tokens_state,
    sumState(toUInt64(total_tokens))                                    AS total_tokens_state,
    sumState(cost_usd)                                                  AS total_cost_state,
    countIfState(status_code = 'ERROR')                                 AS error_count_state,
    quantilesIfState(0.50, 0.95)(ttft_ms, ttft_ms > 0)                 AS ttft_quantiles_state,
    countIfState(ttft_ms > 0)                                           AS streaming_span_count_state
FROM spans
GROUP BY run_id, project_id;

-- Step 4: backward-compatible query VIEW over the aggregate table.
-- Uses ifNotFinite to handle NaN when no streaming spans exist in a run.
CREATE VIEW run_metrics AS
SELECT
    run_id,
    project_id,
    anyLastMerge(trace_id_state)                                        AS trace_id,
    anyLastMerge(session_id_state)                                      AS session_id,
    anyLastMerge(user_id_state)                                         AS user_id,
    minMerge(min_start_state)                                           AS min_start,
    maxMerge(max_end_state)                                             AS max_end,
    countMerge(span_count_state)                                        AS span_count,
    countMerge(llm_calls_state)                                         AS llm_calls,
    countMerge(tool_calls_state)                                        AS tool_calls,
    sumMerge(input_tokens_state)                                        AS input_tokens,
    sumMerge(output_tokens_state)                                       AS output_tokens,
    sumMerge(total_tokens_state)                                        AS total_tokens,
    sumMerge(total_cost_state)                                          AS total_cost_usd,
    countMerge(error_count_state)                                       AS error_count,
    ifNotFinite(quantilesIfMerge(0.50, 0.95)(ttft_quantiles_state)[1], 0.0) AS ttft_p50_ms,
    ifNotFinite(quantilesIfMerge(0.50, 0.95)(ttft_quantiles_state)[2], 0.0) AS ttft_p95_ms,
    countMerge(streaming_span_count_state)                              AS streaming_span_count
FROM run_metrics_agg
GROUP BY run_id, project_id;
