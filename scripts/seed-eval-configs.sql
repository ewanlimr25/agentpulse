-- Seed project_eval_configs so each demo project has realistic eval type coverage.
-- Maps project names (from tracegen) to appropriate eval types for their use case.

-- customer-support-bot: relevance + toxicity (customer-facing safety matters)
INSERT INTO project_eval_configs (project_id, eval_name, enabled, span_kind)
SELECT p.id, e.eval_name, true, e.span_kind
FROM projects p
CROSS JOIN (VALUES
    ('relevance',  'llm.call'),
    ('toxicity',   'llm.call')
) AS e(eval_name, span_kind)
WHERE p.name = 'customer-support-bot'
ON CONFLICT (project_id, eval_name) DO NOTHING;

-- research-assistant: relevance + hallucination + faithfulness (RAG accuracy)
INSERT INTO project_eval_configs (project_id, eval_name, enabled, span_kind)
SELECT p.id, e.eval_name, true, e.span_kind
FROM projects p
CROSS JOIN (VALUES
    ('relevance',    'llm.call'),
    ('hallucination','llm.call'),
    ('faithfulness', 'llm.call')
) AS e(eval_name, span_kind)
WHERE p.name = 'research-assistant'
ON CONFLICT (project_id, eval_name) DO NOTHING;

-- code-review-agent: relevance + hallucination + tool_correctness
INSERT INTO project_eval_configs (project_id, eval_name, enabled, span_kind)
SELECT p.id, e.eval_name, true, e.span_kind
FROM projects p
CROSS JOIN (VALUES
    ('relevance',        'llm.call'),
    ('hallucination',    'llm.call'),
    ('tool_correctness', 'tool.call')
) AS e(eval_name, span_kind)
WHERE p.name = 'code-review-agent'
ON CONFLICT (project_id, eval_name) DO NOTHING;

-- data-pipeline-monitor: relevance + tool_correctness (tool reliability focus)
INSERT INTO project_eval_configs (project_id, eval_name, enabled, span_kind)
SELECT p.id, e.eval_name, true, e.span_kind
FROM projects p
CROSS JOIN (VALUES
    ('relevance',        'llm.call'),
    ('tool_correctness', 'tool.call')
) AS e(eval_name, span_kind)
WHERE p.name = 'data-pipeline-monitor'
ON CONFLICT (project_id, eval_name) DO NOTHING;

-- loop-detection-demo: relevance only (minimal config, focus is on loop signals)
INSERT INTO project_eval_configs (project_id, eval_name, enabled, span_kind)
SELECT p.id, 'relevance', true, 'llm.call'
FROM projects p
WHERE p.name = 'loop-detection-demo'
ON CONFLICT (project_id, eval_name) DO NOTHING;
