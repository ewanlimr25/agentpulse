// Package sqlite provides indie-mode metadata storage backed by an embedded SQLite
// database (modernc.org/sqlite, pure Go — no CGO). Mirrors the postgres package
// store-for-store so cmd/server can swap one for the other based on Mode.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register the "sqlite" driver

	"github.com/agentpulse/agentpulse/backend/internal/migrations"
)

// Open opens (or creates) the SQLite file at path, applies all embedded
// migrations, and returns a *sql.DB configured for indie-mode workloads.
//
// SQLite is opened in WAL journal mode with a 5s busy timeout, both required
// for safe concurrent use under typical indie loads (≤1M spans/day).
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open %s: %w", path, err)
	}

	// SQLite is single-writer; cap connections to keep WAL contention predictable.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}

	if err := migrations.Apply(db, migrations.SQLite()); err != nil {
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	return db, nil
}
