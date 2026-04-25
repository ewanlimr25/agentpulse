// Without the `duckdb` build tag the IndieStores constructor returns a clear
// error so the user knows to rebuild with the tag.
//
//go:build !duckdb

package bootstrap

import "context"

// IndieStores returns ErrIndieDuckDBMissing — DuckDB support was not compiled in.
func IndieStores(ctx context.Context, dataDir string) (*StoreBundle, error) {
	return nil, ErrIndieDuckDBMissing
}
