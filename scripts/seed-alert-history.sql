-- Seed historical alert events and budget alerts for demo projects.
-- Must run after tracegen --demo so that projects, alert_rules, and budget_rules exist.
-- Does NOT require ClickHouse access; run_id is NULL where spans are not addressable from Postgres.

-- ── Alert events ──────────────────────────────────────────────────────────────
-- 4 historical firings per rule, spaced 12 minutes apart (~48 min of history).
-- current_value is tuned to each rule's compare_op so every firing is a genuine violation:
--   gt rules: value escalates slightly above threshold over time (newest firing is worst)
--   lt rules: value drops slightly below threshold over time

INSERT INTO alert_events (
    rule_id, project_id,
    triggered_at, signal_type,
    current_value, threshold, compare_op,
    action_taken, metadata
)
SELECT
    ar.id,
    ar.project_id,
    now() - (n.i * INTERVAL '12 minutes'),
    ar.signal_type,
    CASE
        WHEN ar.compare_op = 'gt' THEN ar.threshold * (1.08 + (n.i - 1) * 0.05)
        ELSE                           ar.threshold * (0.92 - (n.i - 1) * 0.04)
    END,
    ar.threshold,
    ar.compare_op,
    'notify',
    '{}'::jsonb
FROM alert_rules ar
CROSS JOIN (VALUES (1), (2), (3), (4)) AS n(i)
WHERE ar.enabled = true;

-- ── Budget alerts ─────────────────────────────────────────────────────────────
-- 2 synthetic budget alerts per project (once 15 min ago, once 30 min ago).
-- run_id is NULL: we cannot query ClickHouse spans from a Postgres-only script
-- and the column is nullable, so this is safe.

INSERT INTO budget_alerts (
    rule_id, project_id,
    run_id,
    triggered_at, current_cost, threshold_usd,
    action_taken, metadata
)
SELECT
    br.id,
    br.project_id,
    NULL,
    now() - (n.i * INTERVAL '15 minutes'),
    br.threshold_usd * (1.15 + (n.i - 1) * 0.12),
    br.threshold_usd,
    br.action,
    '{}'::jsonb
FROM budget_rules br
CROSS JOIN (VALUES (1), (2)) AS n(i);
