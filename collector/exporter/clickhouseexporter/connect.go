package clickhouseexporter

import (
	"github.com/ClickHouse/clickhouse-go/v2"
)

// connect opens a native ClickHouse connection from config.
func connect(cfg *Config) (clickhouse.Conn, error) {
	return clickhouse.Open(&clickhouse.Options{
		Addr: []string{extractHost(cfg.Endpoint)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: extractUser(cfg.Endpoint),
			Password: extractPassword(cfg.Endpoint),
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})
}

// extractHost parses the host:port from a clickhouse:// DSN.
// Falls back to "localhost:9000" on any parse error.
func extractHost(dsn string) string {
	u, err := parseDSN(dsn)
	if err != nil || u.Host == "" {
		return "localhost:9000"
	}
	return u.Host
}

func extractUser(dsn string) string {
	u, err := parseDSN(dsn)
	if err != nil || u.User == nil {
		return ""
	}
	return u.User.Username()
}

func extractPassword(dsn string) string {
	u, err := parseDSN(dsn)
	if err != nil || u.User == nil {
		return ""
	}
	p, _ := u.User.Password()
	return p
}
