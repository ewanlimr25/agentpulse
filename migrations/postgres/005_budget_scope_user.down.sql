-- AgentPulse: revert budget_rules scope constraint — remove 'user' scope.

ALTER TABLE budget_rules DROP CONSTRAINT budget_rules_scope_check;
ALTER TABLE budget_rules ADD CONSTRAINT budget_rules_scope_check CHECK (scope IN ('run', 'agent', 'window'));
