INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, created_at)
SELECT
    project_id,
    run_id,
    span_id,
    'relevance'                                                                                        AS eval_name,
    round(0.62 + (reinterpretAsUInt32(reverse(unhex(substring(span_id, 1, 8)))) % 38) / 100.0, 2)   AS score,
    'Mocked eval score for seed data.'                                                                 AS reasoning,
    'seed-mock'                                                                                        AS judge_model,
    1                                                                                                  AS eval_version,
    now64()                                                                                            AS created_at
FROM spans
WHERE agent_span_kind = 'llm.call'
  AND attributes['gen_ai.prompt'] != ''
