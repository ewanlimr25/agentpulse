package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

func budgetHashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// mockBudgetStore — implements store.BudgetStore with controllable responses
// ---------------------------------------------------------------------------

type mockBudgetStore struct {
	recentAlerts    []*domain.RecentBudgetAlert
	recentAlertsErr error
	lastProjectID   string // captured to assert project scoping
}

func (m *mockBudgetStore) ListRules(_ context.Context, _ string) ([]*domain.BudgetRule, error) {
	return nil, nil
}
func (m *mockBudgetStore) GetRule(_ context.Context, _ string) (*domain.BudgetRule, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockBudgetStore) CreateRule(_ context.Context, _ *domain.BudgetRule) error { return nil }
func (m *mockBudgetStore) UpdateRule(_ context.Context, _ *domain.BudgetRule) error { return nil }
func (m *mockBudgetStore) DeleteRule(_ context.Context, _ string) error             { return nil }
func (m *mockBudgetStore) ListAlerts(_ context.Context, _ string, _ int) ([]*domain.BudgetAlert, error) {
	return nil, nil
}
func (m *mockBudgetStore) ListRecentAlerts(_ context.Context, projectID string, _ int) ([]*domain.RecentBudgetAlert, error) {
	m.lastProjectID = projectID
	return m.recentAlerts, m.recentAlertsErr
}

// ---------------------------------------------------------------------------
// mockProjectStoreForBudget — shared project store for BearerAuth middleware
// ---------------------------------------------------------------------------

type mockProjectStoreForBudget struct {
	projects map[string]*domain.Project // hash -> project
}

func (m *mockProjectStoreForBudget) List(_ context.Context) ([]*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStoreForBudget) Get(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStoreForBudget) Create(_ context.Context, _ *domain.Project) error { return nil }
func (m *mockProjectStoreForBudget) GetByAPIKeyHash(_ context.Context, hash string) (*domain.Project, error) {
	p, ok := m.projects[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

// buildBudgetRouter builds a full Chi router that mounts ListRecent behind
// BearerAuth, mirroring the production route at:
//
//	GET /api/v1/projects/{projectID}/budget/alerts/recent
func buildBudgetRouter(ps *mockProjectStoreForBudget, bs *mockBudgetStore) http.Handler {
	r := chi.NewRouter()
	budgetH := NewBudgetHandler(bs)
	r.Route("/api/v1/projects/{projectID}", func(r chi.Router) {
		r.Use(middleware.BearerAuth(ps))
		r.Route("/budget", func(r chi.Router) {
			r.Get("/alerts/recent", budgetH.ListRecent)
		})
	})
	return r
}

func budgetProjectStore(projectID, token string) *mockProjectStoreForBudget {
	return &mockProjectStoreForBudget{
		projects: map[string]*domain.Project{
			budgetHashToken(token): {ID: projectID, Name: "Test Project"},
		},
	}
}

// ---------------------------------------------------------------------------
// TestListRecentBudgetAlerts
// ---------------------------------------------------------------------------

// Test 1: No Authorization header → 401 (BearerAuth rejects before handler).
func TestListRecentBudgetAlerts_NoAuth(t *testing.T) {
	ps := budgetProjectStore("proj-1", "secret")
	bs := &mockBudgetStore{}
	h := buildBudgetRouter(ps, bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/budget/alerts/recent", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no auth header, got %d", rr.Code)
	}
}

// Test 2: Malformed Authorization header → 401.
func TestListRecentBudgetAlerts_MalformedAuth(t *testing.T) {
	ps := budgetProjectStore("proj-1", "secret")
	bs := &mockBudgetStore{}
	h := buildBudgetRouter(ps, bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/budget/alerts/recent", nil)
	req.Header.Set("Authorization", "Token secret") // wrong scheme
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with malformed auth, got %d", rr.Code)
	}
}

// Test 3: Valid token but wrong project → 403.
func TestListRecentBudgetAlerts_WrongProject(t *testing.T) {
	// Token belongs to proj-1, URL says proj-2.
	ps := budgetProjectStore("proj-1", "secret")
	bs := &mockBudgetStore{}
	h := buildBudgetRouter(ps, bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-2/budget/alerts/recent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong project, got %d", rr.Code)
	}
}

// Test 4: Valid auth + correct project → 200, store receives correct projectID.
func TestListRecentBudgetAlerts_ValidAuth_CorrectProject(t *testing.T) {
	ps := budgetProjectStore("proj-1", "secret")
	bs := &mockBudgetStore{
		recentAlerts: []*domain.RecentBudgetAlert{},
	}
	h := buildBudgetRouter(ps, bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/budget/alerts/recent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid auth, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if bs.lastProjectID != "proj-1" {
		t.Errorf("store received projectID=%q, expected 'proj-1'", bs.lastProjectID)
	}
}

// Test 5: Handler reads projectID from the URL param and passes it to the store.
func TestListRecentBudgetAlerts_ProjectIDFromURLParam(t *testing.T) {
	ps := budgetProjectStore("proj-xyz", "mytoken")
	bs := &mockBudgetStore{
		recentAlerts: []*domain.RecentBudgetAlert{},
	}
	h := buildBudgetRouter(ps, bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-xyz/budget/alerts/recent", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if bs.lastProjectID != "proj-xyz" {
		t.Errorf("handler passed projectID=%q to store, expected 'proj-xyz'", bs.lastProjectID)
	}
}

// Test 6: Store error → 500.
func TestListRecentBudgetAlerts_StoreError(t *testing.T) {
	ps := budgetProjectStore("proj-1", "secret")
	bs := &mockBudgetStore{recentAlertsErr: errors.New("db unavailable")}
	h := buildBudgetRouter(ps, bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/budget/alerts/recent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Direct handler tests (no full router overhead) — exercise chi URL param
// ---------------------------------------------------------------------------

// Test 7: Handler called directly with chi route context passes projectID to store.
func TestListRecentBudgetAlerts_DirectHandler_ProjectIDPropagated(t *testing.T) {
	bs := &mockBudgetStore{
		recentAlerts: []*domain.RecentBudgetAlert{},
	}
	h := NewBudgetHandler(bs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/direct-proj/budget/alerts/recent", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "direct-proj")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.ListRecent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	if bs.lastProjectID != "direct-proj" {
		t.Errorf("store received projectID=%q, expected 'direct-proj'", bs.lastProjectID)
	}
}
