-- Multi-model eval scoring: track which judge model each eval job targets.
-- The existing UNIQUE (span_id, eval_name) constraint prevents fan-out; replace
-- it with UNIQUE (span_id, eval_name, judge_model) so each model gets its own job.

BEGIN;

ALTER TABLE eval_jobs
    ADD COLUMN judge_model TEXT NOT NULL DEFAULT 'claude-haiku-4-5';

ALTER TABLE eval_jobs
    DROP CONSTRAINT IF EXISTS eval_jobs_span_id_eval_name_key;

ALTER TABLE eval_jobs
    ADD CONSTRAINT eval_jobs_span_id_eval_name_judge_model_key
        UNIQUE (span_id, eval_name, judge_model);

COMMIT;
