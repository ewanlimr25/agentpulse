// Package domain contains the core business types for AgentPulse.
package domain

import "time"

// EmailDigestConfig holds per-project email digest delivery settings.
type EmailDigestConfig struct {
	ID             string
	ProjectID      string
	Enabled        bool
	RecipientEmail string
	Schedule       string // "daily" or "hourly"
	LastSentAt     *time.Time
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
