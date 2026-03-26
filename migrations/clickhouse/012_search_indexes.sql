-- Add pre-lowercased materialized columns for full-text search.
-- Using materialized columns (not Map value expressions) because tokenbf_v1
-- skip indexes are silently ignored on Map element access expressions.
-- Using lower() at materialization time so queries can use case-sensitive LIKE
-- with lower(?), which IS accelerated by tokenbf_v1 (unlike ILIKE which is not).

ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_prompt      String MATERIALIZED lower(attributes['gen_ai.prompt']);
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_completion  String MATERIALIZED lower(attributes['gen_ai.completion']);
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_tool_input  String MATERIALIZED lower(attributes['tool.input']);
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_tool_output String MATERIALIZED lower(attributes['tool.output']);

ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_search_prompt
    search_prompt TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;
ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_search_completion
    search_completion TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;
ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_search_tool_input
    search_tool_input TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;
ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_search_tool_output
    search_tool_output TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;

-- Background operation — does not block reads/writes.
-- May take minutes on large tables; safe to run during normal operation.
ALTER TABLE spans MATERIALIZE INDEX idx_search_prompt;
ALTER TABLE spans MATERIALIZE INDEX idx_search_completion;
ALTER TABLE spans MATERIALIZE INDEX idx_search_tool_input;
ALTER TABLE spans MATERIALIZE INDEX idx_search_tool_output;
