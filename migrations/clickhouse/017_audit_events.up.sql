CREATE TABLE IF NOT EXISTS audit_events (
    timestamp   DateTime64(3, 'UTC'),
    token_hash  String,
    ip          LowCardinality(String),
    method      LowCardinality(String),
    endpoint    String,
    status_code UInt16,
    outcome     LowCardinality(String)
) ENGINE = MergeTree()
ORDER BY timestamp
TTL timestamp + INTERVAL 90 DAY;
