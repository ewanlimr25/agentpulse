package domain

import "time"

// Project is a tenant in AgentPulse. Each project has one API key.
type Project struct {
	ID         string
	Name       string
	APIKeyHash string `json:"-"` // SHA-256 hex of the raw API key; never stored in plaintext
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
