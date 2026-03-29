-- Seed 10 synthetic runs per project for eval trend visualization.
-- Runs are spaced 2 minutes apart ending at now() (covering ~18 minutes of history).
-- Scores show diverging trends across four eval types so the per-type chart tells
-- a clear story and the baseline endpoint has a meaningful 10-run window to average.
--
--   relevance:        0.88 → 0.84 → 0.80 → 0.77 → 0.74 → 0.71 → 0.68 → 0.65 → 0.62 → 0.58  (gradual decline)
--   hallucination:    0.85 → 0.80 → 0.74 → 0.68 → 0.61 → 0.55 → 0.50 → 0.46 → 0.43 → 0.40  (sharp decline — alertable)
--   tool_correctness: 0.52 → 0.57 → 0.62 → 0.67 → 0.71 → 0.75 → 0.79 → 0.82 → 0.85 → 0.88  (recovering)
--   faithfulness:     0.72 → 0.74 → 0.73 → 0.76 → 0.75 → 0.74 → 0.77 → 0.75 → 0.78 → 0.76  (stable with noise)

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
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120)     AS start_time,
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120 - 2) AS end_time,
    toUInt32(200 + n.pt * 40) AS input_tokens,
    toUInt32(80  + n.pt * 15) AS output_tokens,
    round((200 + n.pt * 40 + 80 + n.pt * 15) * 0.000003, 6) AS cost_usd,
    map('gen_ai.prompt', 'Seed trend prompt.', 'gen_ai.completion', 'Seed trend response.') AS attributes
FROM (
    SELECT project_id, anyLast(service_name) AS svc
    FROM spans
    GROUP BY project_id
) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(10)) AS n;

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
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120 - 1) AS start_time,
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120 - 2) AS end_time,
    map('tool.name', 'web_search', 'tool.input', 'Seed query') AS attributes
FROM (
    SELECT project_id, anyLast(service_name) AS svc
    FROM spans
    GROUP BY project_id
) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(10)) AS n;

-- Step 3: Eval scores — 10 data points per type, clearly visible trends.
--   pt=0 oldest (~18 min ago), pt=9 most recent (now).

-- relevance: gradual decline 0.88 → 0.58
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-llm-', toString(n.pt)))), 1, 16)),
    'relevance',
    [0.88, 0.84, 0.80, 0.77, 0.74, 0.71, 0.68, 0.65, 0.62, 0.58][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(10)) AS n;

-- hallucination: sharp decline 0.85 → 0.40 (alertable)
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-llm-', toString(n.pt)))), 1, 16)),
    'hallucination',
    [0.85, 0.80, 0.74, 0.68, 0.61, 0.55, 0.50, 0.46, 0.43, 0.40][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(10)) AS n;

-- tool_correctness: recovering 0.52 → 0.88
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-tool-', toString(n.pt)))), 1, 16)),
    'tool_correctness',
    [0.52, 0.57, 0.62, 0.67, 0.71, 0.75, 0.79, 0.82, 0.85, 0.88][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120 - 1)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(10)) AS n;

-- faithfulness: stable with noise 0.72–0.78
INSERT INTO span_evals (project_id, run_id, span_id, eval_name, score, reasoning, judge_model, eval_version, prompt_version, created_at)
SELECT
    p.project_id,
    concat('seed-trend-', p.project_id, '-', toString(n.pt)),
    lower(substring(hex(MD5(concat(p.project_id, '-llm-', toString(n.pt)))), 1, 16)),
    'faithfulness',
    [0.72, 0.74, 0.73, 0.76, 0.75, 0.74, 0.77, 0.75, 0.78, 0.76][n.pt + 1],
    'Seed trend data.',
    'seed-mock', 1, 0,
    now64(9, 'UTC') - toIntervalSecond((9 - n.pt) * 120)
FROM (SELECT DISTINCT project_id FROM spans) AS p
CROSS JOIN (SELECT number AS pt FROM numbers(10)) AS n;
