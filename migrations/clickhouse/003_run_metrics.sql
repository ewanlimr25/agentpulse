-- AgentPulse: run_metrics view
-- Simple aggregating view over spans used by the backend RunStore.
-- Provides one row per (project_id, run_id) with cost/latency/token rollups.

CREATE VIEW IF NOT EXISTS run_metrics AS
SELECT
    run_id,
    project_id,
    anyLast(trace_id)                              AS trace_id,
    min(start_time)                                AS min_start,
    max(end_time)                                  AS max_end,
    count()                                        AS span_count,
    countIf(agent_span_kind = 'llm.call')          AS llm_calls,
    countIf(agent_span_kind = 'tool.call')         AS tool_calls,
    sum(toUInt64(input_tokens))                    AS input_tokens,
    sum(toUInt64(output_tokens))                   AS output_tokens,
    sum(toUInt64(total_tokens))                    AS total_tokens,
    sum(cost_usd)                                  AS total_cost_usd,
    countIf(status_code = 'ERROR')                 AS error_count
FROM spans
GROUP BY run_id, project_id;
