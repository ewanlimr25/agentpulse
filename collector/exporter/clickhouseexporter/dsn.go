package clickhouseexporter

import (
	"fmt"
	"net/url"
	"strings"
)

type dsnParts struct {
	Host string
	User *url.Userinfo
}

// parseDSN parses a clickhouse:// DSN into its components.
func parseDSN(dsn string) (*dsnParts, error) {
	// Normalise: clickhouse:// -> http:// for url.Parse
	normalised := strings.Replace(dsn, "clickhouse://", "http://", 1)
	u, err := url.Parse(normalised)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN %q: %w", dsn, err)
	}
	return &dsnParts{Host: u.Host, User: u.User}, nil
}
