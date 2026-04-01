BEGIN;

ALTER TABLE eval_jobs
    DROP CONSTRAINT IF EXISTS eval_jobs_span_id_eval_name_judge_model_key;

ALTER TABLE eval_jobs
    ADD CONSTRAINT eval_jobs_span_id_eval_name_key
        UNIQUE (span_id, eval_name);

ALTER TABLE eval_jobs
    DROP COLUMN IF EXISTS judge_model;

COMMIT;
