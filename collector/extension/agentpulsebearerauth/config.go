package agentpulsebearerauth

import (
	"errors"
	"time"
)

// Config controls the AgentPulse Bearer-token auth extension.
//
// The extension reads `Authorization: Bearer <token>` from inbound OTLP/HTTP
// requests (and OTLP/gRPC `authorization` metadata), SHA-256-hashes the token,
// and looks it up in the same `project_ingest_tokens` Postgres table the
// AgentPulse REST API uses.
type Config struct {
	// DSN to the Postgres database holding `project_ingest_tokens`.
	DSN string `mapstructure:"dsn"`

	// Required puts the extension in fail-closed mode: missing or invalid
	// Bearer headers are rejected with 401. Default is false (warn-only) so
	// existing deployments can adopt the extension without an immediate
	// breaking change. Flip on once your SDKs are emitting tokens.
	Required bool `mapstructure:"required"`

	// CacheTTL caches positive token lookups for this duration to avoid
	// hitting Postgres on every request. Set to 0 to disable.
	CacheTTL time.Duration `mapstructure:"cache_ttl"`
}

func (c *Config) Validate() error {
	if c.DSN == "" {
		return errors.New("agentpulsebearerauth: dsn is required")
	}
	return nil
}
