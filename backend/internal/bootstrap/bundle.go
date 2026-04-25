package bootstrap

import (
	"context"
	"database/sql"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// StoreBundle is the complete set of stores cmd/server needs. Both team-mode
// and indie-mode bootstrap fill it; the API router consumes it identically.
type StoreBundle struct {
	// Connection handles — only one of (Postgres + ClickHouse) and
	// (SQLite + DuckDB) is set, depending on Mode.
	PgPool   *pgxpool.Pool
	teamCH   chdriver.Conn
	SQLiteDB *sql.DB
	DuckDB   *sql.DB

	Projects        store.ProjectStore
	Runs            store.RunStore
	Spans           store.SpanStore
	Topology        store.TopologyStore
	Budget          store.BudgetStore
	Evals           store.EvalStore
	EvalConfigs     store.EvalConfigStore
	AlertRules      store.AlertRuleStore
	Analytics       store.AnalyticsStore
	Loops           store.LoopStore
	Sessions        store.SessionStore
	Users           store.UserStore
	Search          store.SearchStore
	PIIConfigs      store.ProjectPIIConfigStore
	SpanFeedback    store.SpanFeedbackStore
	Payloads        store.PayloadStore
	Playground      store.PlaygroundStore
	Exports         store.ExportStore
	RunTags         store.RunTagStore
	RunAnnotations  store.RunAnnotationStore
	PushSubs        store.PushSubscriptionStore
	EmailDigests    store.EmailDigestStore
	IngestTokens    store.IngestTokenStore
	Retention       store.RetentionStore
	PurgeJobs       store.PurgeJobStore
	EvalJobs        store.EvalJobStore
}

// Close releases all underlying database handles.
func (b *StoreBundle) Close() {
	if b.PgPool != nil {
		b.PgPool.Close()
	}
	if b.teamCH != nil {
		_ = b.teamCH.Close()
	}
	if b.SQLiteDB != nil {
		_ = b.SQLiteDB.Close()
	}
	if b.DuckDB != nil {
		_ = b.DuckDB.Close()
	}
}

// ClickHouseConn returns the team-mode ClickHouse driver, or nil when running
// in indie mode. Used by audit writer + eval workers, which still hold a raw
// driver.Conn handle.
func (b *StoreBundle) ClickHouseConn() chdriver.Conn { return b.teamCH }

// IndieStoresFunc constructs an indie-mode bundle from a data dir. The actual
// implementation lives behind the `duckdb` build tag; without it, IndieStores
// returns ErrIndieDuckDBMissing.
type IndieStoresFunc func(ctx context.Context, dataDir string) (*StoreBundle, error)
