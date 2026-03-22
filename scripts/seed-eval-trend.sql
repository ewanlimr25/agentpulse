-- Seed 5 synthetic runs per project for eval trend visualization.
-- Runs are spaced 1 minute apart ending at now().
-- Scores show diverging trends: relevance declining, hallucination worsening,
-- tool_correctness recovering — so the per-type chart tells a clear story.

-- Step 1: Insert one llm.call span per run-point so run_metrics picks them up.
INSERT INTO spans (
    trace_id, span_id, parent_span_id,
    run_id, project_id,
    agent_span_kind, span_name, service_name,
    start_time, end_time,
    input_tokens, output_tokens, cost_usd,
    attributes
)
SELECT
    lower(hex(MD5(concat(p.project_id, '-trace-', toString(n.pt)))))                  AS trace_id,
    lower(substring(hex(MD5(concat(p.project_id, '-llm-', toString(n.pt)))), 1, 16))  AS span_id,
    lower(substring(hex(MD5(concat(p.project_id, '-root-', toString(n.pt)))), 1, 16)) AS parent_span_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt))                          AS run_id,
    p.project_id,
    'llm.call'        AS agent_span_kind,
    'chat_completion' AS span_name,
    p.svc             AS service_name,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60)     AS start_time,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60 - 2) AS end_time,
    toUInt32(200 + n.pt * 40) AS input_tokens,
    toUInt32(80  + n.pt * 15) AS output_tokens,
    round((200 + n.pt * 40 + 80 + n.pt * 15) * 0.000003, 6) AS cost_usd,
    map('gen_ai.prompt', 'Seed trend prompt.', 'gen_ai.completion', 'Seed trend response.') AS attributes
FROM (
    SELECT project_id, anyLast(service_name) AS svc
    FROM spans
    GROUP BY project_id
) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(5)) AS n;

-- Step 2: Insert one tool.call span per run-point (for tool_correctness evals).
INSERT INTO spans (
    trace_id, span_id, parent_span_id,
    run_id, project_id,
    agent_span_kind, span_name, service_name,
    start_time, end_time,
    attributes
)
SELECT
    lower(hex(MD5(concat(p.project_id, '-trace-', toString(n.pt)))))                   AS trace_id,
    lower(substring(hex(MD5(concat(p.project_id, '-tool-', toString(n.pt)))), 1, 16))  AS span_id,
    lower(substring(hex(MD5(concat(p.project_id, '-llm-',  toString(n.pt)))), 1, 16))  AS parent_span_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt))                           AS run_id,
    p.project_id,
    'tool.call'  AS agent_span_kind,
    'web_search' AS span_name,
    p.svc        AS service_name,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60 - 1) AS start_time,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60 - 2) AS end_time,
    map('tool.name', 'web_search', 'tool.input', 'Seed query') AS attributes
FROM (
    SELECT project_id, anyLast(service_name) AS svc
    FROM spans
    GROUP BY project_id
) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(5)) AS n;

-- Step 3: Eval scores — visible diverging trends across 5 points.
--   pt=0 oldest (4 min ago), pt=4 most recent (now).
--
--   relevance:        0.88 → 0.82 → 0.75 → 0.70 → 0.64  (gradual decline)
--   hallucination:    0.81 → 0.72 → 0.62 → 0.53 → 0.46  (sharp decline — alertable)
--   tool_correctness: 0.58 → 0.65 → 0.72 → 0.79 → 0.85  (improving)

-- relevance
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-llm-', toString(n.pt)))), 1, 16)),
    'relevance',
    [0.88, 0.82, 0.75, 0.70, 0.64][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(5)) AS n;

-- hallucination
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-llm-', toString(n.pt)))), 1, 16)),
    'hallucination',
    [0.81, 0.72, 0.62, 0.53, 0.46][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(5)) AS n;

-- tool_correctness
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-tool-', toString(n.pt)))), 1, 16)),
    'tool_correctness',
    [0.58, 0.65, 0.72, 0.79, 0.85][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((4 - n.pt) * 60 - 1)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(5)) AS n;
