// Package domain contains the core business types for AgentPulse.
package domain

import "time"

// PushSubscription represents a browser Web Push subscription for a project.
type PushSubscription struct {
	ID             string
	ProjectID      string
	Endpoint       string
	P256DHKey      string
	AuthKey        string
	VAPIDPublicKey string // the public key that was used to create this subscription
	UserAgent      string
	CreatedAt      time.Time
}
