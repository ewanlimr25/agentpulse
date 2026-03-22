BEGIN;
DROP TABLE IF EXISTS run_loops;
ALTER TABLE alert_rules DROP CONSTRAINT IF EXISTS alert_rules_signal_type_check;
ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_signal_type_check
    CHECK (signal_type IN ('error_rate', 'latency_p95', 'quality_score', 'tool_failure'));
COMMIT;
