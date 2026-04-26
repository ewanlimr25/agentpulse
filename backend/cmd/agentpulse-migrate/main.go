// Command agentpulse-migrate moves data between indie-mode storage
// (SQLite + DuckDB) and team-mode storage (Postgres + ClickHouse).
//
// Usage:
//
//	agentpulse-migrate --to=team \
//	    --indie-data-dir=$HOME/.agentpulse \
//	    --postgres-url=postgres://... \
//	    --clickhouse-dsn=clickhouse://... \
//	    [--batch-size=10000] [--dry-run] [--resume]
//
// The tool is resumable: a checkpoint file at <data-dir>/migration_checkpoint.json
// records the last-migrated row per table. Re-running with --resume picks up
// where the previous run left off.
//
// Build:
//
//	CGO_ENABLED=1 go build -tags=duckdb ./cmd/agentpulse-migrate
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Version of the migrate tool — bumped when checkpoint format changes.
const migrateVersion = "1.0.0"

func main() {
	var (
		toMode       = flag.String("to", "", "destination mode: only \"team\" is currently supported")
		indieDataDir = flag.String("indie-data-dir", os.ExpandEnv("$HOME/.agentpulse"), "path to the indie-mode data directory (contains agentpulse.db + spans.duckdb + payloads/)")
		postgresURL  = flag.String("postgres-url", os.Getenv("DATABASE_URL"), "Postgres connection URL (team-mode metadata)")
		clickhouseDSN = flag.String("clickhouse-dsn", os.Getenv("CLICKHOUSE_DSN"), "ClickHouse DSN (team-mode columnar)")
		batchSize    = flag.Int("batch-size", 10_000, "row batch size for streaming writes")
		dryRun       = flag.Bool("dry-run", false, "read & count source rows but do NOT write to the destination")
		resume       = flag.Bool("resume", false, "resume from the previous checkpoint instead of starting over")
		showVersion  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("agentpulse-migrate", migrateVersion)
		return
	}

	if *toMode != "team" {
		die("--to=team is required (only direction currently supported)")
	}
	if !*dryRun {
		if *postgresURL == "" {
			die("--postgres-url is required (or DATABASE_URL env var) when not in --dry-run")
		}
		if *clickhouseDSN == "" {
			die("--clickhouse-dsn is required (or CLICKHOUSE_DSN env var) when not in --dry-run")
		}
	}
	if *batchSize <= 0 {
		die("--batch-size must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts := MigrationOptions{
		IndieDataDir:  *indieDataDir,
		PostgresURL:   *postgresURL,
		ClickHouseDSN: *clickhouseDSN,
		BatchSize:     *batchSize,
		DryRun:        *dryRun,
		Resume:        *resume,
	}

	start := time.Now()
	report, err := Migrate(ctx, opts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "migration interrupted; checkpoint saved — re-run with --resume to continue")
			os.Exit(130)
		}
		die("migration failed: %v", err)
	}

	fmt.Fprintf(os.Stdout, `
Migration complete (mode=%s)
   elapsed:           %s
   sqlite rows moved: %d
   duckdb rows moved: %d
   skipped (resume):  %d
   dry-run:           %t
`, *toMode, time.Since(start).Round(time.Second),
		report.SQLiteRowsCopied, report.DuckDBRowsCopied,
		report.RowsSkipped, *dryRun)
}

func die(format string, args ...any) {
	fmt.Fprintln(os.Stderr, "agentpulse-migrate:", fmt.Sprintf(format, args...))
	os.Exit(1)
}
