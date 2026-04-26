package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
	"github.com/agentpulse/agentpulse/backend/internal/store/conformance"
	"github.com/agentpulse/agentpulse/backend/internal/store/sqlite"
)

// TestProjectStore_Conformance runs the shared store contract against SQLite.
// The same test bodies will run against the Postgres ProjectStore via a
// matching test in store/postgres (gated by an integration tag) — that's how
// indie- and team-mode parity is enforced.
func TestProjectStore_Conformance(t *testing.T) {
	conformance.RunProjectStore(t, func(t *testing.T) (store.ProjectStore, func()) {
		t.Helper()
		dbPath := filepath.Join(t.TempDir(), "agentpulse.db")
		db, err := sqlite.Open(dbPath)
		if err != nil {
			t.Fatalf("sqlite.Open: %v", err)
		}
		return sqlite.NewProjectStore(db), func() { _ = db.Close() }
	})
}

// TestIngestTokenStore_Conformance runs the IngestTokenStore contract.
// Each test gets a fresh database with one seeded project so the FK is
// satisfied.
func TestIngestTokenStore_Conformance(t *testing.T) {
	conformance.RunIngestTokenStore(t, func(t *testing.T) (store.IngestTokenStore, string, func()) {
		t.Helper()
		dbPath := filepath.Join(t.TempDir(), "agentpulse.db")
		db, err := sqlite.Open(dbPath)
		if err != nil {
			t.Fatalf("sqlite.Open: %v", err)
		}
		// Seed a project so the ingest_tokens FK passes.
		projects := sqlite.NewProjectStore(db)
		p := &domain.Project{Name: "harness", APIKeyHash: "harness-api", AdminKeyHash: "harness-admin"}
		if err := projects.Create(t.Context(), p); err != nil {
			_ = db.Close()
			t.Fatalf("seed project: %v", err)
		}
		return sqlite.NewIngestTokenStore(db), p.ID, func() { _ = db.Close() }
	})
}
