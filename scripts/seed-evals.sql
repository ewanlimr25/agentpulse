-- Mock eval scores for seeded demo data.
-- Uses span_id bytes as a deterministic hash source so scores are stable across re-seeds.
-- Each eval type uses a different byte window to produce independent score distributions.

-- relevance: 0.62–0.99  (generally good, occasional dips to show alerts)
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    project_id, run_id, span_id,
    'relevance',
    round(0.62 + (reinterpretAsUInt32(reverse(unhex(substring(span_id, 1, 8)))) % 38) / 100.0, 2),
    'Mocked eval score for seed data.',
    'seed-mock', 1, 0, now64()
FROM spans
WHERE agent_span_kind = 'llm.call'
  AND attributes['gen_ai.prompt'] != '';

-- hallucination: 0.55–0.94  (wider spread — some runs have real problems)
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    project_id, run_id, span_id,
    'hallucination',
    round(0.55 + (reinterpretAsUInt32(reverse(unhex(substring(span_id, 9, 8)))) % 40) / 100.0, 2),
    'Mocked eval score for seed data.',
    'seed-mock', 1, 0, now64()
FROM spans
WHERE agent_span_kind = 'llm.call'
  AND attributes['gen_ai.prompt'] != '';

-- faithfulness: 0.60–0.95  (RAG grounding — research-assistant project only)
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    project_id, run_id, span_id,
    'faithfulness',
    round(0.60 + (reinterpretAsUInt32(reverse(unhex(substring(span_id, 17, 8)))) % 36) / 100.0, 2),
    'Mocked eval score for seed data.',
    'seed-mock', 1, 0, now64()
FROM spans
WHERE agent_span_kind = 'llm.call'
  AND attributes['gen_ai.prompt'] != ''
  AND service_name LIKE '%research%';

-- toxicity: 0.78–0.99  (customer-facing — usually safe, occasional dip)
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    project_id, run_id, span_id,
    'toxicity',
    round(0.78 + (reinterpretAsUInt32(reverse(unhex(substring(span_id, 25, 8)))) % 22) / 100.0, 2),
    'Mocked eval score for seed data.',
    'seed-mock', 1, 0, now64()
FROM spans
WHERE agent_span_kind = 'llm.call'
  AND attributes['gen_ai.prompt'] != ''
  AND service_name LIKE '%support%';

-- tool_correctness: 0.50–0.92  (tool.call spans — more variance to show interesting charts)
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    project_id, run_id, span_id,
    'tool_correctness',
    round(0.50 + (reinterpretAsUInt32(reverse(unhex(substring(span_id, 1, 8)))) % 43) / 100.0, 2),
    'Mocked eval score for seed data.',
    'seed-mock', 1, 0, now64()
FROM spans
WHERE agent_span_kind = 'tool.call';
