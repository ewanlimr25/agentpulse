-- Multi-model eval scoring: add judge_model to the ORDER BY key of span_evals.
-- ReplacingMergeTree deduplication is keyed on ORDER BY, so judge_model must be
-- part of the key to allow multiple models to score the same (span, eval_name)
-- without overwriting each other.
--
-- TODO: Production migration requires: ADD COLUMN + backfill + controlled table swap.
-- For dev/staging with no prod data, drop-recreate is safe.

DROP TABLE IF EXISTS span_evals;

CREATE TABLE IF NOT EXISTS span_evals
(
    project_id    String,
    run_id        String,
    span_id       String,
    eval_name     LowCardinality(String),
    judge_model   LowCardinality(String) DEFAULT '',
    score         Float32,
    reasoning     String DEFAULT '',
    eval_version  UInt16 DEFAULT 1,
    prompt_version UInt16 DEFAULT 0,
    created_at    DateTime64(3, 'UTC') DEFAULT now64()
)
ENGINE = ReplacingMergeTree(created_at)
ORDER BY (project_id, run_id, span_id, eval_name, judge_model)
TTL toDate(created_at) + INTERVAL 30 DAY;
