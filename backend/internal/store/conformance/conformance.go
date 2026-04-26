// Package conformance defines store-interface tests that must pass against
// every backend (postgres, sqlite, clickhouse, duckdb). Each store package
// imports this and runs the suite via its driver in its own _test.go.
//
// The pattern is: each Run<Interface>(t, factory) function takes a fresh
// store factory that returns a clean instance. The same test bodies run
// against indie- and team-mode stores, guaranteeing behaviour parity.
//
// Conformance tests are kept small on purpose — exhaustive coverage belongs
// in the per-driver tests. Conformance is the *contract*: the surface every
// implementation must agree on.
package conformance

import (
	"context"
	"testing"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// ProjectStoreFactory builds a fresh ProjectStore plus a teardown.
type ProjectStoreFactory func(t *testing.T) (s store.ProjectStore, cleanup func())

// RunProjectStore exercises the full ProjectStore contract.
// Implementations must:
//   - Round-trip Create / Get / List with stable timestamps
//   - GetByAPIKeyHash and GetByAdminKeyHash find what Create inserted
//   - GetLoopConfig returns DefaultLoopConfig when no row exists
//   - PutLoopConfig persists across reads
func RunProjectStore(t *testing.T, factory ProjectStoreFactory) {
	t.Helper()
	ctx := context.Background()

	t.Run("Create_Get_RoundTrip", func(t *testing.T) {
		s, cleanup := factory(t)
		defer cleanup()

		p := &domain.Project{
			Name:         "test-project",
			APIKeyHash:   "hash-api-aaa",
			AdminKeyHash: "hash-admin-aaa",
		}
		if err := s.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if p.ID == "" {
			t.Fatalf("Create: ID not populated")
		}

		got, err := s.Get(ctx, p.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != p.Name || got.APIKeyHash != p.APIKeyHash || got.AdminKeyHash != p.AdminKeyHash {
			t.Fatalf("Get returned mismatched record: %+v", got)
		}
	})

	t.Run("GetByAPIKeyHash", func(t *testing.T) {
		s, cleanup := factory(t)
		defer cleanup()

		p := &domain.Project{Name: "by-hash", APIKeyHash: "hash-api-bbb", AdminKeyHash: "hash-admin-bbb"}
		if err := s.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := s.GetByAPIKeyHash(ctx, "hash-api-bbb")
		if err != nil {
			t.Fatalf("GetByAPIKeyHash: %v", err)
		}
		if got.ID != p.ID {
			t.Fatalf("GetByAPIKeyHash returned wrong project: %s vs %s", got.ID, p.ID)
		}
	})

	t.Run("GetByAdminKeyHash", func(t *testing.T) {
		s, cleanup := factory(t)
		defer cleanup()

		p := &domain.Project{Name: "by-admin", APIKeyHash: "hash-api-ccc", AdminKeyHash: "hash-admin-ccc"}
		if err := s.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := s.GetByAdminKeyHash(ctx, "hash-admin-ccc")
		if err != nil {
			t.Fatalf("GetByAdminKeyHash: %v", err)
		}
		if got.ID != p.ID {
			t.Fatalf("GetByAdminKeyHash returned wrong project")
		}
	})

	t.Run("List_OrdersByCreatedDesc", func(t *testing.T) {
		s, cleanup := factory(t)
		defer cleanup()

		// Insert in a known order; assert List returns newest-first.
		first := &domain.Project{Name: "first", APIKeyHash: "hash-api-1", AdminKeyHash: "hash-admin-1"}
		if err := s.Create(ctx, first); err != nil {
			t.Fatalf("create first: %v", err)
		}
		// SQLite's CURRENT_TIMESTAMP has 1-second resolution but our schema uses
		// strftime millisecond precision. Sleep 5ms to guarantee ordering.
		time.Sleep(5 * time.Millisecond)
		second := &domain.Project{Name: "second", APIKeyHash: "hash-api-2", AdminKeyHash: "hash-admin-2"}
		if err := s.Create(ctx, second); err != nil {
			t.Fatalf("create second: %v", err)
		}

		all, err := s.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(all) < 2 {
			t.Fatalf("List: expected ≥2 rows, got %d", len(all))
		}
		// Most recent first.
		if all[0].Name != "second" {
			t.Fatalf("List: expected newest first ('second'), got %q", all[0].Name)
		}
	})

	t.Run("LoopConfig_DefaultWhenAbsent", func(t *testing.T) {
		s, cleanup := factory(t)
		defer cleanup()

		p := &domain.Project{Name: "loops", APIKeyHash: "hash-api-loop", AdminKeyHash: "hash-admin-loop"}
		if err := s.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}

		cfg, err := s.GetLoopConfig(ctx, p.ID)
		if err != nil {
			t.Fatalf("GetLoopConfig: %v", err)
		}
		if cfg == nil {
			t.Fatalf("GetLoopConfig returned nil — must return defaults")
		}
		def := domain.DefaultLoopConfig
		if *cfg != def {
			t.Fatalf("GetLoopConfig: expected DefaultLoopConfig, got %+v", *cfg)
		}
	})

	t.Run("LoopConfig_PutPersists", func(t *testing.T) {
		s, cleanup := factory(t)
		defer cleanup()

		p := &domain.Project{Name: "loops2", APIKeyHash: "hash-api-loop2", AdminKeyHash: "hash-admin-loop2"}
		if err := s.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}

		custom := domain.DefaultLoopConfig
		custom.Tier1MinCount = 99 // arbitrary marker
		if err := s.PutLoopConfig(ctx, p.ID, custom); err != nil {
			t.Fatalf("PutLoopConfig: %v", err)
		}

		got, err := s.GetLoopConfig(ctx, p.ID)
		if err != nil {
			t.Fatalf("GetLoopConfig after Put: %v", err)
		}
		if got.Tier1MinCount != 99 {
			t.Fatalf("GetLoopConfig: PutLoopConfig did not persist (got Tier1MinCount=%d)", got.Tier1MinCount)
		}
	})
}

// IngestTokenStoreFactory builds a fresh IngestTokenStore plus a project ID
// (created by the harness so the FK is satisfied) and a teardown.
type IngestTokenStoreFactory func(t *testing.T) (s store.IngestTokenStore, projectID string, cleanup func())

// RunIngestTokenStore exercises the IngestTokenStore contract.
func RunIngestTokenStore(t *testing.T, factory IngestTokenStoreFactory) {
	t.Helper()
	ctx := context.Background()

	t.Run("Create_GetByHash_RoundTrip", func(t *testing.T) {
		s, projectID, cleanup := factory(t)
		defer cleanup()

		tok, err := s.Create(ctx, projectID, "tok-hash-aaa", "ci")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if tok.ID == "" {
			t.Fatalf("Create: ID not populated")
		}

		got, err := s.GetByHash(ctx, "tok-hash-aaa")
		if err != nil {
			t.Fatalf("GetByHash: %v", err)
		}
		if got.ProjectID != projectID {
			t.Fatalf("GetByHash: project mismatch: %s vs %s", got.ProjectID, projectID)
		}
	})

	t.Run("ListByProject", func(t *testing.T) {
		s, projectID, cleanup := factory(t)
		defer cleanup()

		_, err := s.Create(ctx, projectID, "tok-hash-list-1", "label-1")
		if err != nil {
			t.Fatalf("Create 1: %v", err)
		}
		_, err = s.Create(ctx, projectID, "tok-hash-list-2", "label-2")
		if err != nil {
			t.Fatalf("Create 2: %v", err)
		}

		toks, err := s.ListByProject(ctx, projectID)
		if err != nil {
			t.Fatalf("ListByProject: %v", err)
		}
		if len(toks) != 2 {
			t.Fatalf("ListByProject: expected 2, got %d", len(toks))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		s, projectID, cleanup := factory(t)
		defer cleanup()

		tok, err := s.Create(ctx, projectID, "tok-hash-del", "del")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := s.Delete(ctx, tok.ID, projectID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		toks, err := s.ListByProject(ctx, projectID)
		if err != nil {
			t.Fatalf("ListByProject after Delete: %v", err)
		}
		for _, t2 := range toks {
			if t2.ID == tok.ID {
				t.Fatalf("Delete: token still present")
			}
		}
	})
}
