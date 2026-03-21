package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration for the backend API.
// Values are read from environment variables.
type Config struct {
	HTTP             HTTPConfig
	Postgres         PostgresConfig
	ClickHouse       ClickHouseConfig
	S3               S3Config
	AnthropicAPIKey  string
}

type HTTPConfig struct {
	Host string
	Port string
}

type PostgresConfig struct {
	DSN string
}

type ClickHouseConfig struct {
	Addr     string
	Database string
	User     string
	Password string
}

type S3Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		HTTP: HTTPConfig{
			Host: getEnv("HTTP_HOST", "0.0.0.0"),
			Port: getEnv("HTTP_PORT", "8080"),
		},
		Postgres: PostgresConfig{
			DSN: getEnv("POSTGRES_DSN", "postgres://agentpulse:agentpulse@localhost:5432/agentpulse?sslmode=disable"),
		},
		ClickHouse: ClickHouseConfig{
			Addr:     getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
			Database: getEnv("CLICKHOUSE_DATABASE", "agentpulse"),
			User:     getEnv("CLICKHOUSE_USER", "agentpulse"),
			Password: getEnv("CLICKHOUSE_PASSWORD", "agentpulse"),
		},
		S3: S3Config{
			Endpoint:  getEnv("S3_ENDPOINT", "http://localhost:9090"),
			Bucket:    getEnv("S3_BUCKET", "agentpulse-spans"),
			AccessKey: getEnv("S3_ACCESS_KEY", "agentpulse"),
			SecretKey: getEnv("S3_SECRET_KEY", "agentpulse"),
		},
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.HTTP.Port == "" {
		return fmt.Errorf("HTTP_PORT must not be empty")
	}
	return nil
}

func (c *Config) HTTPAddr() string {
	return c.HTTP.Host + ":" + c.HTTP.Port
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
