package authenforceproc

// Config holds configuration for the authenforceproc processor.
type Config struct {
	// DSN is the Postgres connection string used to look up ingest tokens.
	DSN string `mapstructure:"dsn"`

	// Enabled controls whether token validation is performed.
	// When false, all spans pass through regardless of token presence.
	// Default: true.
	Enabled bool `mapstructure:"enabled"`

	// FailOpen controls the behaviour when the Postgres DB is unreachable.
	// When true (default), spans are passed through on DB errors.
	// When false, spans are dropped if the DB cannot be queried.
	FailOpen bool `mapstructure:"fail_open"`
}

func defaultConfig() *Config {
	return &Config{
		DSN:      "postgres://agentpulse:agentpulse@localhost:5432/agentpulse?sslmode=disable",
		Enabled:  true,
		FailOpen: true,
	}
}
