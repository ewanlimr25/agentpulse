// Fallback implementation when the binary is built without the `duckdb` tag.
// Lets the rest of the codebase reference duckdb.Open / duckdb.Available without
// pulling in the CGO dependency, and surfaces a clear error if indie mode is
// requested at runtime.
//
//go:build !duckdb

package duckdb

import (
	"database/sql"
	"errors"
)

// ErrNotCompiled is returned by Open when this binary was built without -tags=duckdb.
var ErrNotCompiled = errors.New("duckdb support not compiled in — rebuild with `-tags=duckdb` to enable indie mode")

// Open returns ErrNotCompiled. Indie-mode bootstrap checks Available() first and
// fails fast with a friendly message if the user requested --mode=indie on a
// team-only build.
func Open(path string) (*sql.DB, error) { return nil, ErrNotCompiled }

// Available reports whether DuckDB support is compiled into this binary.
func Available() bool { return false }
