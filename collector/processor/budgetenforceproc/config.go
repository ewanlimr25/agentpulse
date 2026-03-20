package budgetenforceproc

import "time"

// Config holds configuration for the budgetenforceproc processor.
type Config struct {
	// PostgresDSN is the connection string for the Postgres database where
	// budget rules and alerts are stored.
	PostgresDSN string `mapstructure:"postgres_dsn"`

	// RuleRefreshInterval controls how often budget rules are re-read from Postgres.
	// Defaults to 30s.
	RuleRefreshInterval time.Duration `mapstructure:"rule_refresh_interval"`

	// WebhookTimeout is the deadline for webhook HTTP calls.
	// Defaults to 5s.
	WebhookTimeout time.Duration `mapstructure:"webhook_timeout"`
}

func defaultConfig() *Config {
	return &Config{
		PostgresDSN:         "postgres://agentpulse:agentpulse@localhost:5432/agentpulse?sslmode=disable",
		RuleRefreshInterval: 30 * time.Second,
		WebhookTimeout:      5 * time.Second,
	}
}
