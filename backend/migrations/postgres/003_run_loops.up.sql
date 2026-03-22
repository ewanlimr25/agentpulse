BEGIN;

CREATE TABLE IF NOT EXISTS run_loops (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id           TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    detection_type   TEXT NOT NULL CHECK (detection_type IN ('repeated_call', 'topology_cycle')),
    span_name        TEXT NOT NULL DEFAULT '',
    input_hash       TEXT NOT NULL DEFAULT '',
    output_hash      TEXT NOT NULL DEFAULT '',
    confidence       TEXT NOT NULL DEFAULT 'low' CHECK (confidence IN ('high', 'low')),
    occurrence_count INTEGER NOT NULL DEFAULT 0,
    detected_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_run_loops_dedup
    ON run_loops (run_id, detection_type, span_name, input_hash, output_hash);

CREATE INDEX IF NOT EXISTS idx_run_loops_project
    ON run_loops (project_id, detected_at DESC);

CREATE INDEX IF NOT EXISTS idx_run_loops_run
    ON run_loops (run_id);

-- Extend the alert_rules signal_type constraint to allow 'agent_loop'
ALTER TABLE alert_rules DROP CONSTRAINT IF EXISTS alert_rules_signal_type_check;
ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_signal_type_check
    CHECK (signal_type IN ('error_rate', 'latency_p95', 'quality_score', 'tool_failure', 'agent_loop'));

COMMIT;
