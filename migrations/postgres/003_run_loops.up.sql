-- Agent loop detection: stores detected repeated tool calls and topology cycles.
CREATE TABLE IF NOT EXISTS run_loops (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         TEXT        NOT NULL,
    project_id     UUID        NOT NULL,
    detection_type TEXT        NOT NULL, -- "repeated_tool_call" | "topology_cycle"
    span_name      TEXT        NOT NULL DEFAULT '',
    input_hash     TEXT        NOT NULL DEFAULT '0',
    output_hash    TEXT        NOT NULL DEFAULT '0',
    confidence     TEXT        NOT NULL, -- "high" | "low"
    occurrence_count INT       NOT NULL DEFAULT 1,
    detected_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT run_loops_dedup UNIQUE (run_id, detection_type, span_name, input_hash, output_hash)
);

CREATE INDEX IF NOT EXISTS run_loops_project_detected ON run_loops (project_id, detected_at DESC);
CREATE INDEX IF NOT EXISTS run_loops_run_id           ON run_loops (run_id);

-- Watermark table for the loop detector background worker.
CREATE TABLE IF NOT EXISTS loop_detector_watermarks (
    project_id  UUID        PRIMARY KEY,
    last_scanned_at TIMESTAMPTZ NOT NULL
);

-- Extend alert_rules signal_type CHECK to include agent_loop.
ALTER TABLE alert_rules
    DROP CONSTRAINT IF EXISTS alert_rules_signal_type_check;

ALTER TABLE alert_rules
    ADD CONSTRAINT alert_rules_signal_type_check
    CHECK (signal_type IN ('error_rate', 'latency_p95', 'quality_score', 'tool_failure', 'agent_loop'));
