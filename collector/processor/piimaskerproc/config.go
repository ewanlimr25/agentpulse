package piimaskerproc

import (
	"fmt"
	"time"
)

// Config holds configuration for the piimaskerproc processor.
type Config struct {
	// PostgresDSN is the connection string for Postgres, where project PII
	// settings (enabled flag + custom rules) are stored.
	PostgresDSN string `mapstructure:"postgres_dsn"`

	// SettingsRefreshInterval controls how often PII settings are re-read from
	// Postgres. Defaults to 30s.
	SettingsRefreshInterval time.Duration `mapstructure:"settings_refresh_interval"`

	// MaxPoolConns is the maximum number of Postgres connections in the pool.
	// Defaults to 2 (PII settings reads are infrequent).
	MaxPoolConns int `mapstructure:"max_pool_conns"`
}

func defaultConfig() *Config {
	return &Config{
		PostgresDSN:             "postgres://agentpulse:agentpulse@localhost:5432/agentpulse?sslmode=disable",
		SettingsRefreshInterval: 30 * time.Second,
		MaxPoolConns:            2,
	}
}

// Validate returns an error if the config is invalid.
func (c *Config) Validate() error {
	if c.PostgresDSN == "" {
		return fmt.Errorf("piimaskerproc: postgres_dsn is required")
	}
	return nil
}
