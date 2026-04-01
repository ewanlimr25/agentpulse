package domain

import "time"

// ProjectPIIConfig holds per-project PII/secret redaction settings.
// A row is created on first PUT; a missing row implies defaults (disabled, no rules).
type ProjectPIIConfig struct {
	ProjectID           string
	PIIRedactionEnabled bool
	PIICustomRules      []PIICustomRule
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// PIICustomRule is a single named regex pattern for PII redaction.
// Patterns are stored as strings; the collector compiles them at load time.
type PIICustomRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
}
