//go:build duckdb

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/store/duckdb"
	"github.com/agentpulse/agentpulse/backend/internal/store/sqlite"
)

// MigrationOptions controls a single Migrate invocation.
type MigrationOptions struct {
	IndieDataDir  string
	PostgresURL   string
	ClickHouseDSN string
	BatchSize     int
	DryRun        bool
	Resume        bool
}

// MigrationReport summarises what Migrate did.
type MigrationReport struct {
	SQLiteRowsCopied int64
	DuckDBRowsCopied int64
	RowsSkipped      int64
}

// Checkpoint records the last-migrated row per table so the tool is resumable.
type Checkpoint struct {
	Version  string                  `json:"version"`
	Updated  time.Time               `json:"updated"`
	Tables   map[string]TableProgress `json:"tables"`
}

// TableProgress tracks per-table migration state.
type TableProgress struct {
	Done      bool      `json:"done"`
	LastID    string    `json:"last_id"` // for tables with TEXT/UUID PKs
	LastTime  time.Time `json:"last_time,omitempty"` // for span tables (range by start_time)
	RowCount  int64     `json:"row_count"`
}

// sqliteTables is the metadata-table migration order. Order matters for FKs:
// projects must come before everything that references projects.
var sqliteTables = []string{
	"projects",
	"project_ingest_tokens",
	"topology_nodes",
	"topology_edges",
	"budget_rules",
	"budget_alerts",
	"alert_rules",
	"alert_events",
	"eval_jobs",
	"project_eval_configs",
	"project_pii_configs",
	"span_feedback",
	"prompt_playground_sessions",
	"prompt_playground_variants",
	"prompt_playground_executions",
	"run_tags",
	"run_annotations",
	"project_retention_config",
	"purge_jobs",
	"push_subscriptions",
	"email_digest_configs",
	"run_loops",
	"loop_detector_watermarks",
}

// duckdbTables is the columnar-table migration order. spans depends on nothing
// but should be migrated last because it's typically the largest by row count.
var duckdbTables = []string{
	"span_evals",
	"audit_events",
	"spans",
}

// Migrate orchestrates the full indie → team data move.
func Migrate(ctx context.Context, opts MigrationOptions) (*MigrationReport, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// 1. Open source DBs (always required).
	sqliteDB, err := sqlite.Open(filepath.Join(opts.IndieDataDir, "agentpulse.db"))
	if err != nil {
		return nil, fmt.Errorf("open sqlite source: %w", err)
	}
	defer sqliteDB.Close()

	duckDB, err := duckdb.Open(filepath.Join(opts.IndieDataDir, "spans.duckdb"))
	if err != nil {
		return nil, fmt.Errorf("open duckdb source: %w", err)
	}
	defer duckDB.Close()

	// 2. Load (or initialise) checkpoint.
	cpPath := filepath.Join(opts.IndieDataDir, "migration_checkpoint.json")
	checkpoint, err := loadCheckpoint(cpPath, opts.Resume)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}

	// 3. Open destination DBs unless dry-run.
	var pgPool *pgxpool.Pool
	if !opts.DryRun {
		pgPool, err = pgxpool.New(ctx, opts.PostgresURL)
		if err != nil {
			return nil, fmt.Errorf("open postgres dest: %w", err)
		}
		defer pgPool.Close()
		if err := pgPool.Ping(ctx); err != nil {
			return nil, fmt.Errorf("ping postgres dest: %w", err)
		}
	}

	report := &MigrationReport{}

	// 4. Migrate SQLite → Postgres tables.
	for _, table := range sqliteTables {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		prog := checkpoint.Tables[table]
		if prog.Done {
			report.RowsSkipped += prog.RowCount
			logger.Info("skip (done)", "table", table, "rows", prog.RowCount)
			continue
		}
		n, err := copyTable(ctx, sqliteDB, pgPool, table, opts, &prog)
		if err != nil {
			saveCheckpoint(cpPath, checkpoint, table, prog)
			return report, fmt.Errorf("copy %s: %w", table, err)
		}
		prog.Done = true
		prog.RowCount = n
		report.SQLiteRowsCopied += n
		checkpoint.Tables[table] = prog
		if err := saveCheckpoint(cpPath, checkpoint, table, prog); err != nil {
			logger.Warn("save checkpoint", "error", err)
		}
		logger.Info("copied", "table", table, "rows", n)
	}

	// 5. Migrate DuckDB → ClickHouse tables.
	// In dry-run we just count rows. In real mode we use DuckDB's COPY ... TO
	// PARQUET → clickhouse-client --input-format Parquet for highest throughput.
	for _, table := range duckdbTables {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		prog := checkpoint.Tables[table]
		if prog.Done {
			report.RowsSkipped += prog.RowCount
			logger.Info("skip (done)", "table", table, "rows", prog.RowCount)
			continue
		}
		n, err := exportDuckDBTable(ctx, duckDB, table, opts, &prog)
		if err != nil {
			saveCheckpoint(cpPath, checkpoint, table, prog)
			return report, fmt.Errorf("export %s: %w", table, err)
		}
		prog.Done = true
		prog.RowCount = n
		report.DuckDBRowsCopied += n
		checkpoint.Tables[table] = prog
		if err := saveCheckpoint(cpPath, checkpoint, table, prog); err != nil {
			logger.Warn("save checkpoint", "error", err)
		}
		logger.Info("exported", "table", table, "rows", n, "format", "parquet")
	}

	return report, nil
}

// copyTable streams every row from a SQLite table to Postgres in batch-sized
// chunks. In dry-run it only counts. The table layout is assumed to be
// identical (same column names + types modulo bool/int conversions handled by
// the database driver).
func copyTable(ctx context.Context, src *sql.DB, dst *pgxpool.Pool, table string, opts MigrationOptions, prog *TableProgress) (int64, error) {
	// Discover columns at runtime so we don't have to hand-maintain a column
	// list per table — we copy whatever the source schema has.
	cols, err := sqliteColumns(ctx, src, table)
	if err != nil {
		return 0, fmt.Errorf("introspect cols: %w", err)
	}
	if len(cols) == 0 {
		return 0, nil
	}

	colList := strings.Join(cols, ", ")
	rows, err := src.QueryContext(ctx, "SELECT "+colList+" FROM "+quoteIdent(table))
	if err != nil {
		return 0, fmt.Errorf("select: %w", err)
	}
	defer rows.Close()

	var (
		count    int64
		batch    [][]any
		batchCap = opts.BatchSize
	)

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		batch = append(batch, dest)
		count++

		if len(batch) >= batchCap {
			if !opts.DryRun {
				if err := pgInsertBatch(ctx, dst, table, cols, batch); err != nil {
					return count, err
				}
			}
			batch = batch[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	if len(batch) > 0 && !opts.DryRun {
		if err := pgInsertBatch(ctx, dst, table, cols, batch); err != nil {
			return count, err
		}
	}
	prog.RowCount = count
	return count, nil
}

// pgInsertBatch performs a single bulk INSERT … ON CONFLICT DO NOTHING into
// Postgres for one batch of rows.
func pgInsertBatch(ctx context.Context, pool *pgxpool.Pool, table string, cols []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	colList := strings.Join(cols, ", ")
	placeholders := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*len(cols))
	idx := 1
	for _, row := range rows {
		ph := make([]string, len(cols))
		for j := range row {
			ph[j] = fmt.Sprintf("$%d", idx)
			idx++
		}
		placeholders = append(placeholders, "("+strings.Join(ph, ",")+")")
		args = append(args, row...)
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s ON CONFLICT DO NOTHING",
		quoteIdent(table), colList, strings.Join(placeholders, ","))
	_, err := pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("pg batch insert %s: %w", table, err)
	}
	return nil
}

// exportDuckDBTable writes a Parquet snapshot of a DuckDB table for offline
// import into ClickHouse via `clickhouse-client --query='INSERT INTO X FROM
// INFILE \'<path>.parquet\' FORMAT Parquet'`. We do NOT directly write to
// ClickHouse here because ClickHouse's HTTP/native interface from Go would
// duplicate row-by-row code we don't need — Parquet is faster, lossless, and
// the standard team-mode loading path.
func exportDuckDBTable(ctx context.Context, src *sql.DB, table string, opts MigrationOptions, prog *TableProgress) (int64, error) {
	var count int64
	if err := src.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(table)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	prog.RowCount = count

	if opts.DryRun {
		return count, nil
	}
	out := filepath.Join(opts.IndieDataDir, "migration_export", table+".parquet")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return count, fmt.Errorf("mkdir export: %w", err)
	}
	// DuckDB's COPY supports parquet directly. Single statement, no Go-side
	// row materialisation.
	stmt := fmt.Sprintf("COPY (SELECT * FROM %s) TO %s (FORMAT 'parquet')",
		quoteIdent(table), sqlString(out))
	if _, err := src.ExecContext(ctx, stmt); err != nil {
		return count, fmt.Errorf("duckdb export parquet %s: %w", table, err)
	}
	return count, nil
}

// sqliteColumns returns the column names for a table in declaration order.
func sqliteColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var (
			cid          int
			name, ctype  string
			notnull, pk  int
			dfltValue    sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func quoteIdent(s string) string {
	// SQLite + Postgres + DuckDB all accept double-quoted identifiers.
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func sqlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// loadCheckpoint reads the on-disk checkpoint, optionally honouring --resume.
// Without --resume an existing checkpoint is discarded so the migration restarts.
func loadCheckpoint(path string, resume bool) (*Checkpoint, error) {
	cp := &Checkpoint{
		Version: migrateVersion,
		Updated: time.Now().UTC(),
		Tables:  map[string]TableProgress{},
	}
	if !resume {
		return cp, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cp, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(body, cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}
	if cp.Tables == nil {
		cp.Tables = map[string]TableProgress{}
	}
	return cp, nil
}

func saveCheckpoint(path string, cp *Checkpoint, table string, prog TableProgress) error {
	cp.Updated = time.Now().UTC()
	cp.Tables[table] = prog
	body, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}
