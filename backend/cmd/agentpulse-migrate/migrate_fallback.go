// When built without the `duckdb` build tag, the migrate tool can't run —
// it requires CGO + DuckDB to read the indie spans store. This stub returns a
// clear error so cmd builds for both tag combinations.
//
//go:build !duckdb

package main

import (
	"context"
	"errors"
)

// MigrationOptions is the build-tag-agnostic options shape, mirrored in
// migrate.go (duckdb build).
type MigrationOptions struct {
	IndieDataDir  string
	PostgresURL   string
	ClickHouseDSN string
	BatchSize     int
	DryRun        bool
	Resume        bool
}

// MigrationReport mirrors the duckdb-build version.
type MigrationReport struct {
	SQLiteRowsCopied int64
	DuckDBRowsCopied int64
	RowsSkipped      int64
}

// ErrDuckDBMissing is returned when the binary was built without -tags=duckdb.
var ErrDuckDBMissing = errors.New("agentpulse-migrate: rebuild with -tags=duckdb (CGO_ENABLED=1) to enable migration; the no-CGO build cannot read indie-mode spans.duckdb")

// Migrate is a no-op stub when DuckDB support is not compiled in.
func Migrate(ctx context.Context, opts MigrationOptions) (*MigrationReport, error) {
	return nil, ErrDuckDBMissing
}
