// Package duckdb provides indie-mode columnar storage for spans + aggregates,
// backed by an embedded DuckDB database (marcboeker/go-duckdb/v2).
//
// DuckDB requires CGO. Build with -tags=duckdb to enable; otherwise indie mode
// will fail to start with a clear error from the bootstrap layer.
//
//go:build duckdb

package duckdb

import (
	"database/sql"
	"fmt"

	_ "github.com/marcboeker/go-duckdb/v2" // register the "duckdb" driver

	"github.com/agentpulse/agentpulse/backend/internal/migrations"
)

// Open opens a DuckDB database at path (creating it if missing) and applies
// embedded migrations. The DB is configured for typical indie workloads.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("duckdb open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("duckdb ping: %w", err)
	}
	// DuckDB supports concurrent readers; cap writers at 1 to keep things simple.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	if err := migrations.Apply(db, migrations.DuckDB()); err != nil {
		return nil, fmt.Errorf("duckdb migrate: %w", err)
	}
	return db, nil
}

// Available reports whether DuckDB support is compiled into this binary.
// Always true when the duckdb build tag is set; otherwise false (see fallback.go).
func Available() bool { return true }
