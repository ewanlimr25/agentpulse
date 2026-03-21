CREATE TABLE IF NOT EXISTS span_evals
(
    project_id   String,
    run_id       String,
    span_id      String,
    eval_name    LowCardinality(String),
    score        Float32,
    reasoning    String DEFAULT '',
    judge_model  LowCardinality(String) DEFAULT '',
    eval_version UInt16 DEFAULT 1,
    created_at   DateTime64(3, 'UTC') DEFAULT now64()
)
ENGINE = ReplacingMergeTree(created_at)
ORDER BY (project_id, run_id, span_id, eval_name)
TTL toDate(created_at) + INTERVAL 30 DAY;
