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
	WebPush         WebPushConfig
	Email           EmailConfig
}

// WebPushConfig holds VAPID credentials for browser push notifications.
type WebPushConfig struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	Subject         string // mailto: or https: URI, required by VAPID spec
}

// EmailConfig holds credentials for transactional email delivery via Resend.
type EmailConfig struct {
	ResendAPIKey string
	FromAddress  string
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
	Host    string
	Port    string
	TLSCert string // HTTP_TLS_CERT env var — path to TLS cert file
	TLSKey  string // HTTP_TLS_KEY env var — path to TLS key file
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
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	EnforceHTTPS bool
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		HTTP: HTTPConfig{
			Host:    getEnv("HTTP_HOST", "0.0.0.0"),
			Port:    getEnv("HTTP_PORT", "8080"),
			TLSCert: getEnv("HTTP_TLS_CERT", ""),
			TLSKey:  getEnv("HTTP_TLS_KEY", ""),
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
			Endpoint:     getEnv("S3_ENDPOINT", "http://localhost:9090"),
			Bucket:       getEnv("S3_BUCKET", "agentpulse-spans"),
			AccessKey:    getEnv("S3_ACCESS_KEY", "agentpulse"),
			SecretKey:    getEnv("S3_SECRET_KEY", "agentpulse"),
			EnforceHTTPS: getEnvBool("S3_ENFORCE_HTTPS", true),
		},
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		GoogleAIAPIKey:  getEnv("GOOGLE_AI_API_KEY", ""),
		CORS:            loadCORSConfig(),
		WebPush: WebPushConfig{
			VAPIDPublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			VAPIDPrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:         getEnv("VAPID_SUBJECT", ""),
		},
		Email: EmailConfig{
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			FromAddress:  getEnv("EMAIL_FROM_ADDRESS", "noreply@agentpulse.dev"),
		},
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

// WarnDefaults logs a WARN for each connection string that still uses localhost
// or the default "agentpulse:agentpulse" credentials. Call this once at server
// startup so operators know they haven't configured production credentials.
func (c *Config) WarnDefaults(warn func(msg string, args ...any)) {
	if strings.Contains(c.Postgres.DSN, "localhost") ||
		strings.Contains(c.Postgres.DSN, "agentpulse:agentpulse") {
		warn("POSTGRES_DSN uses default/local credentials — set POSTGRES_DSN for production")
	}
	if strings.Contains(c.ClickHouse.Addr, "localhost") {
		warn("CLICKHOUSE_ADDR uses localhost — set CLICKHOUSE_ADDR for production")
	}
	if c.ClickHouse.Password == "agentpulse" {
		warn("CLICKHOUSE_PASSWORD uses default credential — set CLICKHOUSE_PASSWORD for production")
	}
	if strings.Contains(c.S3.Endpoint, "localhost") {
		warn("S3_ENDPOINT uses localhost — set S3_ENDPOINT for production")
	}
	if c.S3.SecretKey == "agentpulse" {
		warn("S3_SECRET_KEY uses default credential — set S3_SECRET_KEY for production")
	}
}

func (c *Config) TLSEnabled() bool {
	return c.HTTP.TLSCert != "" && c.HTTP.TLSKey != ""
}

func (c *Config) ErrorDefaults(fatal func(msg string, args ...any)) {
	if strings.Contains(c.Postgres.DSN, "agentpulse:agentpulse") {
		fatal("POSTGRES_DSN uses default credentials in production — set POSTGRES_DSN")
	}
	if c.ClickHouse.Password == "agentpulse" {
		fatal("CLICKHOUSE_PASSWORD uses default credential in production — set CLICKHOUSE_PASSWORD")
	}
	if c.S3.SecretKey == "agentpulse" {
		fatal("S3_SECRET_KEY uses default credential in production — set S3_SECRET_KEY")
	}
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

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "false", "0", "no":
		return false
	default:
		return true
	}
}
