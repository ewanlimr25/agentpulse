// Package migrations runs versioned SQL migrations against an embedded backend
// (SQLite for metadata, DuckDB for spans) used by indie mode.
//
// Migrations are tracked per-backend in a `_schema_migrations` table so that
// SQLite and DuckDB schemas evolve independently. Each migration file is
// applied exactly once, in lexicographic order.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed sqlite/*.sql
var sqliteFS embed.FS

//go:embed duckdb/*.sql
var duckdbFS embed.FS

// SQLite returns the embedded SQLite migrations FS.
func SQLite() fs.FS { return mustSub(sqliteFS, "sqlite") }

// DuckDB returns the embedded DuckDB migrations FS.
func DuckDB() fs.FS { return mustSub(duckdbFS, "duckdb") }

func mustSub(efs embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(efs, dir)
	if err != nil {
		panic(fmt.Errorf("migrations: sub %q: %w", dir, err))
	}
	return sub
}

// Apply runs all migrations in fsys against db that have not yet been recorded
// in the `_schema_migrations` table. Files must be named `<version>_<name>.sql`
// and are applied in lexicographic order. Each file is run as a single batch.
func Apply(db *sql.DB, fsys fs.FS) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("migrations: create table: %w", err)
	}

	applied, err := loadApplied(db)
	if err != nil {
		return fmt.Errorf("migrations: load applied: %w", err)
	}

	files, err := listMigrations(fsys)
	if err != nil {
		return fmt.Errorf("migrations: list: %w", err)
	}

	for _, name := range files {
		if applied[name] {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("migrations: read %s: %w", name, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			return fmt.Errorf("migrations: exec %s: %w", name, err)
		}
		if _, err := db.Exec(`INSERT INTO _schema_migrations(version) VALUES (?)`, name); err != nil {
			return fmt.Errorf("migrations: record %s: %w", name, err)
		}
	}
	return nil
}

func loadApplied(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT version FROM _schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func listMigrations(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}
