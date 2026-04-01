package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all runtime configuration for the backend API.
// Values are read from environment variables.
type Config struct {
	HTTP            HTTPConfig
	Postgres        PostgresConfig
	ClickHouse      ClickHouseConfig
	S3              S3Config
	AnthropicAPIKey string
	OpenAIAPIKey    string
	GoogleAIAPIKey  string
	CORS            CORSConfig
}

// String returns a human-readable representation of Config with all API key fields
// redacted so that the struct is safe to pass to loggers.
func (c *Config) String() string {
	redact := func(s string) string {
		if s == "" {
			return "<unset>"
		}
		return "<redacted>"
	}
	return fmt.Sprintf(
		"Config{HTTP:%+v Postgres:{DSN:<redacted>} ClickHouse:{Addr:%s Database:%s User:%s Password:<redacted>} "+
			"S3:{Endpoint:%s Bucket:%s AccessKey:<redacted> SecretKey:<redacted>} "+
			"AnthropicAPIKey:%s OpenAIAPIKey:%s GoogleAIAPIKey:%s CORS:%+v}",
		c.HTTP,
		c.ClickHouse.Addr, c.ClickHouse.Database, c.ClickHouse.User,
		c.S3.Endpoint, c.S3.Bucket,
		redact(c.AnthropicAPIKey), redact(c.OpenAIAPIKey), redact(c.GoogleAIAPIKey),
		c.CORS,
	)
}

// CORSConfig controls which origins are permitted to make cross-origin requests.
type CORSConfig struct {
	// AllowedOrigins is the explicit list of permitted origins.
	// When empty and DevMode is true the server sends Access-Control-Allow-Origin: *.
	AllowedOrigins []string
	// DevMode is true when APP_ENV is "" or "development".
	DevMode bool
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
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		GoogleAIAPIKey:  getEnv("GOOGLE_AI_API_KEY", ""),
		CORS:            loadCORSConfig(),
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

func loadCORSConfig() CORSConfig {
	appEnv := os.Getenv("APP_ENV")
	devMode := appEnv == "" || appEnv == "development"

	var allowedOrigins []string
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		for _, origin := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}

	return CORSConfig{
		AllowedOrigins: allowedOrigins,
		DevMode:        devMode,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
