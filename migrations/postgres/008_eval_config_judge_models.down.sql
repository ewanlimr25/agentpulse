ALTER TABLE project_eval_configs
    DROP CONSTRAINT IF EXISTS check_judge_models_length;

ALTER TABLE project_eval_configs
    DROP COLUMN IF EXISTS judge_models;
