package authenforceproc

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ingestTokenRecord is a local mirror of the project_ingest_tokens row.
// We avoid importing the backend module to keep the collector self-contained.
type ingestTokenRecord struct {
	projectID string
}

// authStore handles Postgres reads for the authenforceproc processor.
type authStore struct {
	pool *pgxpool.Pool
}

func newAuthStore(ctx context.Context, dsn string) (*authStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("authenforceproc: pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("authenforceproc: ping: %w", err)
	}
	return &authStore{pool: pool}, nil
}

func (s *authStore) close() {
	s.pool.Close()
}

// getByHash looks up a token by its SHA-256 hex hash.
// Returns (nil, nil) when no matching row is found (not an error — means invalid token).
func (s *authStore) getByHash(ctx context.Context, tokenHash string) (*ingestTokenRecord, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT project_id
		FROM project_ingest_tokens
		WHERE token_hash = $1
	`, tokenHash)
	var r ingestTokenRecord
	if err := row.Scan(&r.projectID); err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("authenforceproc: query token hash: %w", err)
	}
	return &r, nil
}
