package clickhouseexporter

import "time"

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
}

func defaultConfig() *Config {
	return &Config{
		Endpoint:      "clickhouse://agentpulse:agentpulse@localhost:9000/agentpulse",
		Database:      "agentpulse",
		Table:         "spans",
		BatchSize:     1000,
		FlushInterval: 5 * time.Second,
		MaxRetries:    3,
	}
}
