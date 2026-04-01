package piimaskerproc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// piiSettingsStore reads project PII settings from Postgres and listens for
// change notifications so the processor can refresh its in-memory cache
// without waiting for the next refresh tick.
type piiSettingsStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// projectPIISettings holds the compiled custom rules for a single project.
type projectPIISettings struct {
	customRules []piiPattern
}

// newPIISettingsStore creates a pgxpool limited to maxConns connections.
// It returns an error if the initial ping fails so the processor can decide
// whether to enter fail-closed mode rather than panicking.
func newPIISettingsStore(ctx context.Context, logger *zap.Logger, dsn string, maxConns int) (*piiSettingsStore, error) {
	// Inject pool_max_conns into the DSN.
	connStr := fmt.Sprintf("%s&pool_max_conns=%d", dsn, maxConns)
	// If the DSN already uses '?' for the first query param, the connector
	// handles '&' appends correctly in pgxpool. But if the DSN has no query
	// params at all we need '?' instead.
	// Use pgxpool.ParseConfig to merge the setting cleanly.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pii settings store: parse dsn: %w", err)
	}
	cfg.MaxConns = int32(maxConns)
	_ = connStr // connStr was only illustrative; we use cfg directly

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pii settings store: pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pii settings store: ping: %w", err)
	}
	return &piiSettingsStore{pool: pool, logger: logger}, nil
}

func (s *piiSettingsStore) close() {
	s.pool.Close()
}

// loadEnabledProjects queries all projects with pii_redaction_enabled = true
// and compiles their custom rules. If the query fails it returns nil + error
// and the caller continues to use the stale in-memory cache.
func (s *piiSettingsStore) loadEnabledProjects(ctx context.Context) (map[string]*projectPIISettings, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT project_id::text, pii_custom_rules
		FROM project_pii_configs
		WHERE pii_redaction_enabled = true
	`)
	if err != nil {
		return nil, fmt.Errorf("pii settings store: query: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*projectPIISettings)
	for rows.Next() {
		var projectID string
		var rulesJSON []byte
		if err := rows.Scan(&projectID, &rulesJSON); err != nil {
			return nil, fmt.Errorf("pii settings store: scan row: %w", err)
		}

		var raw []piiCustomRule
		if len(rulesJSON) > 0 {
			if err := json.Unmarshal(rulesJSON, &raw); err != nil {
				s.logger.Warn("piimaskerproc: failed to unmarshal custom rules for project",
					zap.String("project_id", projectID),
					zap.Error(err),
				)
				// Continue with empty custom rules for this project.
			}
		}

		result[projectID] = &projectPIISettings{
			customRules: parseCustomRules(s.logger, raw),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pii settings store: rows: %w", err)
	}
	return result, nil
}

// listenForChanges acquires a dedicated connection and issues LISTEN
// pii_settings_changed. Each NOTIFY call from Postgres triggers onChange().
// The function blocks until ctx is cancelled or a connection error occurs,
// returning the error so the caller can reconnect.
func (s *piiSettingsStore) listenForChanges(ctx context.Context, onChange func()) error {
	// Acquire a dedicated connection — LISTEN/NOTIFY requires a persistent conn.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("pii listen: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN pii_settings_changed"); err != nil {
		return fmt.Errorf("pii listen: LISTEN: %w", err)
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled — normal shutdown.
				return ctx.Err()
			}
			return fmt.Errorf("pii listen: wait for notification: %w", err)
		}
		_ = notification
		onChange()
	}
}

// acquireRawConn returns a raw *pgx.Conn for use with WaitForNotification.
// It is not currently called directly but is kept for clarity.
var _ = func(conn *pgxpool.Conn) *pgx.Conn {
	return conn.Conn()
}
