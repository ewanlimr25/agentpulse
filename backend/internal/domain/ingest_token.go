package domain

import "time"

// IngestToken is a per-project credential used to authenticate OTel span ingestion.
// The raw token is returned only once at creation time; only the SHA-256 hash is stored.
type IngestToken struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	// TokenHash is never returned in JSON — only the raw token is shown at creation time.
	TokenHash string `json:"-"`
}
