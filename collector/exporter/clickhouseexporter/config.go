package clickhouseexporter

import (
	"fmt"
	"strings"
	"time"
)

// Config holds configuration for the ClickHouse exporter.
type Config struct {
	// Endpoint is the ClickHouse DSN, e.g. "clickhouse://user:pass@host:9000/db"
	Endpoint string `mapstructure:"endpoint"`

	// Database to write spans into.
	Database string `mapstructure:"database"`

	// Table to write spans into.
	Table string `mapstructure:"table"`

	// BatchSize is the maximum number of spans to buffer before flushing.
	BatchSize int `mapstructure:"batch_size"`

	// FlushInterval is the maximum time to wait before flushing a partial batch.
	FlushInterval time.Duration `mapstructure:"flush_interval"`

	// MaxRetries is the number of times to retry a failed batch insert.
	MaxRetries int `mapstructure:"max_retries"`

	// S3 holds optional payload offloading configuration.
	S3 S3Config `mapstructure:"s3"`
}

// S3Config holds configuration for offloading large span payloads to S3.
type S3Config struct {
	Enabled        bool          `mapstructure:"enabled"`
	Endpoint       string        `mapstructure:"endpoint"`
	Bucket         string        `mapstructure:"bucket"`
	Region         string        `mapstructure:"region"`
	AccessKey      string        `mapstructure:"access_key"`
	SecretKey      string        `mapstructure:"secret_key"`
	ThresholdBytes int           `mapstructure:"threshold_bytes"`
	UploadTimeout  time.Duration `mapstructure:"upload_timeout"`
	EnforceHTTPS   bool          `mapstructure:"enforce_https"`
}

// String returns a human-readable representation of S3Config with credentials redacted.
func (c S3Config) String() string {
	return fmt.Sprintf(
		"S3Config{Endpoint:%s Bucket:%s Region:%s AccessKey:<redacted> SecretKey:<redacted> ThresholdBytes:%d Enabled:%v}",
		c.Endpoint, c.Bucket, c.Region, c.ThresholdBytes, c.Enabled,
	)
}

// Validate checks config constraints.
func (c *Config) Validate() error {
	if c.S3.Enabled && c.S3.EnforceHTTPS && strings.HasPrefix(c.S3.Endpoint, "http://") {
		return fmt.Errorf("S3 endpoint must use HTTPS (set enforce_https: false to override in development)")
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		Endpoint:      "clickhouse://agentpulse:agentpulse@localhost:9000/agentpulse",
		Database:      "agentpulse",
		Table:         "spans",
		BatchSize:     1000,
		FlushInterval: 5 * time.Second,
		MaxRetries:    3,
		S3: S3Config{
			Enabled:        false,
			ThresholdBytes: 8192,
			UploadTimeout:  5 * time.Second,
			EnforceHTTPS:   true,
		},
	}
}
