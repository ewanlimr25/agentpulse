-- Track which prompt version produced each eval score.
-- For built-in eval types, prompt_version stays 0.
-- For custom eval types, it mirrors project_eval_configs.prompt_version at eval time.
-- ADD COLUMN IF NOT EXISTS is non-blocking in ClickHouse.
ALTER TABLE span_evals ADD COLUMN IF NOT EXISTS prompt_version UInt16 DEFAULT 0;
