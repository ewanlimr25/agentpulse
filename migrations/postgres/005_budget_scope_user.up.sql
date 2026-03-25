-- AgentPulse: extend budget_rules scope to support per-user budget enforcement.
-- Adds 'user' as a valid scope value alongside the existing 'run', 'agent', 'window'.

ALTER TABLE budget_rules DROP CONSTRAINT budget_rules_scope_check;
ALTER TABLE budget_rules ADD CONSTRAINT budget_rules_scope_check CHECK (scope IN ('run', 'agent', 'window', 'user'));
