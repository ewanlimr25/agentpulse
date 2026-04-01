-- Multi-model eval scoring: allow each eval config to specify which judge models
-- to use for scoring. Defaults to claude-haiku-4-5 to preserve existing behaviour.
-- At most 3 models per config to bound fan-out at enqueue time.

ALTER TABLE project_eval_configs
    ADD COLUMN judge_models TEXT[] NOT NULL DEFAULT ARRAY['claude-haiku-4-5'];

ALTER TABLE project_eval_configs
    ADD CONSTRAINT check_judge_models_length CHECK (array_length(judge_models, 1) <= 3);
