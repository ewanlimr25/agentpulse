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

func alertHashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// mockAlertRuleStore — implements store.AlertRuleStore with controllable responses
// ---------------------------------------------------------------------------

type mockAlertRuleStore struct {
	recentEvents    []*domain.RecentAlertEvent
	recentEventsErr error
	lastProjectID   string // captured to assert project scoping
}

func (m *mockAlertRuleStore) ListRules(_ context.Context, _ string) ([]*domain.AlertRule, error) {
	return nil, nil
}
func (m *mockAlertRuleStore) GetRule(_ context.Context, _ string) (*domain.AlertRule, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockAlertRuleStore) CreateRule(_ context.Context, _ *domain.AlertRule) error { return nil }
func (m *mockAlertRuleStore) UpdateRule(_ context.Context, _ *domain.AlertRule) error { return nil }
func (m *mockAlertRuleStore) DeleteRule(_ context.Context, _ string) error            { return nil }
func (m *mockAlertRuleStore) ListEnabledRules(_ context.Context) ([]*domain.AlertRule, error) {
	return nil, nil
}
func (m *mockAlertRuleStore) ListEvents(_ context.Context, _ string, _ int) ([]*domain.AlertEvent, error) {
	return nil, nil
}
func (m *mockAlertRuleStore) CreateEvent(_ context.Context, _ *domain.AlertEvent) error { return nil }
func (m *mockAlertRuleStore) LastEventForRule(_ context.Context, _ string) (*domain.AlertEvent, error) {
	return nil, nil
}
func (m *mockAlertRuleStore) ListRecentEvents(_ context.Context, projectID string, _ int) ([]*domain.RecentAlertEvent, error) {
	m.lastProjectID = projectID
	return m.recentEvents, m.recentEventsErr
}

// ---------------------------------------------------------------------------
// mockProjectStoreForAlerts — shares same shape as other mock project stores
// ---------------------------------------------------------------------------

type mockProjectStoreForAlerts struct {
	projects map[string]*domain.Project // hash -> project
}

func (m *mockProjectStoreForAlerts) List(_ context.Context) ([]*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStoreForAlerts) Get(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStoreForAlerts) Create(_ context.Context, _ *domain.Project) error { return nil }
func (m *mockProjectStoreForAlerts) GetByAPIKeyHash(_ context.Context, hash string) (*domain.Project, error) {
	p, ok := m.projects[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func alertProjectStore(projectID, token string) *mockProjectStoreForAlerts {
	return &mockProjectStoreForAlerts{
		projects: map[string]*domain.Project{
			alertHashToken(token): {ID: projectID, Name: "Test Project"},
		},
	}
}

// buildAlertRouter builds a full Chi router that mounts ListRecent behind
// BearerAuth, mirroring the production route at:
//
//	GET /api/v1/projects/{projectID}/alerts/events/recent
func buildAlertRouter(ps *mockProjectStoreForAlerts, as *mockAlertRuleStore) http.Handler {
	r := chi.NewRouter()
	alertH := NewAlertRuleHandler(as)
	r.Route("/api/v1/projects/{projectID}", func(r chi.Router) {
		r.Use(middleware.BearerAuth(ps))
		r.Route("/alerts", func(r chi.Router) {
			r.Get("/events/recent", alertH.ListRecent)
		})
	})
	return r
}

// ---------------------------------------------------------------------------
// TestListRecentAlertEvents
// ---------------------------------------------------------------------------

// Test 1: No Authorization header → 401 (BearerAuth rejects before handler).
func TestListRecentAlertEvents_NoAuth(t *testing.T) {
	ps := alertProjectStore("proj-1", "secret")
	as := &mockAlertRuleStore{}
	h := buildAlertRouter(ps, as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/alerts/events/recent", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no auth header, got %d", rr.Code)
	}
}

// Test 2: Malformed Authorization header → 401.
func TestListRecentAlertEvents_MalformedAuth(t *testing.T) {
	ps := alertProjectStore("proj-1", "secret")
	as := &mockAlertRuleStore{}
	h := buildAlertRouter(ps, as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/alerts/events/recent", nil)
	req.Header.Set("Authorization", "Token secret") // wrong scheme
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with malformed auth, got %d", rr.Code)
	}
}

// Test 3: Valid token but wrong project → 403.
func TestListRecentAlertEvents_WrongProject(t *testing.T) {
	// Token belongs to proj-1, URL says proj-2.
	ps := alertProjectStore("proj-1", "secret")
	as := &mockAlertRuleStore{}
	h := buildAlertRouter(ps, as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-2/alerts/events/recent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong project, got %d", rr.Code)
	}
}

// Test 4: Valid auth + correct project → 200, store receives correct projectID.
func TestListRecentAlertEvents_ValidAuth_CorrectProject(t *testing.T) {
	ps := alertProjectStore("proj-1", "secret")
	as := &mockAlertRuleStore{
		recentEvents: []*domain.RecentAlertEvent{},
	}
	h := buildAlertRouter(ps, as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/alerts/events/recent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid auth, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if as.lastProjectID != "proj-1" {
		t.Errorf("store received projectID=%q, expected 'proj-1'", as.lastProjectID)
	}
}

// Test 5: Handler reads projectID from the URL param and passes it to the store.
func TestListRecentAlertEvents_ProjectIDFromURLParam(t *testing.T) {
	ps := alertProjectStore("proj-abc", "mytoken")
	as := &mockAlertRuleStore{
		recentEvents: []*domain.RecentAlertEvent{},
	}
	h := buildAlertRouter(ps, as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-abc/alerts/events/recent", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if as.lastProjectID != "proj-abc" {
		t.Errorf("handler passed projectID=%q to store, expected 'proj-abc'", as.lastProjectID)
	}
}

// Test 6: Store error → 500.
func TestListRecentAlertEvents_StoreError(t *testing.T) {
	ps := alertProjectStore("proj-1", "secret")
	as := &mockAlertRuleStore{recentEventsErr: errors.New("db unavailable")}
	h := buildAlertRouter(ps, as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/proj-1/alerts/events/recent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on store error, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Direct handler tests — exercise chi URL param injection
// ---------------------------------------------------------------------------

// Test 7: Handler called directly with chi route context passes projectID to store.
func TestListRecentAlertEvents_DirectHandler_ProjectIDPropagated(t *testing.T) {
	as := &mockAlertRuleStore{
		recentEvents: []*domain.RecentAlertEvent{},
	}
	h := NewAlertRuleHandler(as)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/direct-proj/alerts/events/recent", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectID", "direct-proj")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.ListRecent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	if as.lastProjectID != "direct-proj" {
		t.Errorf("store received projectID=%q, expected 'direct-proj'", as.lastProjectID)
	}
}
