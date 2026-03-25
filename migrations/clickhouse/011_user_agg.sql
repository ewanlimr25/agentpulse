-- AgentPulse: user_agg materialized view
-- Incrementally aggregates user-level metrics from spans.
-- Replaces the need for a background Go aggregator.
-- Queried via -Merge combinators: uniqMerge, sumMerge, etc.

CREATE MATERIALIZED VIEW IF NOT EXISTS user_agg
ENGINE = AggregatingMergeTree()
ORDER BY (project_id, user_id)
POPULATE
AS SELECT
    project_id,
    user_id,
    uniqState(run_id)                          AS run_count_state,
    sumState(cost_usd)                         AS total_cost_state,
    sumState(toInt64(total_tokens))            AS total_tokens_state,
    sumState(toInt64(input_tokens))            AS input_tokens_state,
    sumState(toInt64(output_tokens))           AS output_tokens_state,
    countIfState(status_code = 'ERROR')        AS error_count_state,
    minState(start_time)                       AS first_run_at_state,
    maxState(end_time)                         AS last_run_at_state
FROM spans
WHERE user_id != ''
GROUP BY project_id, user_id;
