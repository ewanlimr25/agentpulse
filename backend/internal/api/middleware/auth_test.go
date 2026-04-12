package middleware_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// mockProjectStore satisfies store.ProjectStore for testing.
type mockProjectStore struct {
	projects map[string]*domain.Project // hash -> project
}

func (m *mockProjectStore) List(_ context.Context) ([]*domain.Project, error) { return nil, nil }
func (m *mockProjectStore) Get(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStore) Create(_ context.Context, _ *domain.Project) error { return nil }
func (m *mockProjectStore) GetByAPIKeyHash(_ context.Context, hash string) (*domain.Project, error) {
	p, ok := m.projects[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}
func (m *mockProjectStore) GetByAdminKeyHash(_ context.Context, _ string) (*domain.Project, error) {
	return nil, fmt.Errorf("not found")
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func buildStore(projectID, token string) *mockProjectStore {
	return &mockProjectStore{
		projects: map[string]*domain.Project{
			hashToken(token): {ID: projectID, Name: "Test Project"},
		},
	}
}

// makeHandler wraps a simple 200 OK handler behind BearerAuth on a Chi route
// that includes {projectID} so the project-match check is exercised.
func makeHandler(store *mockProjectStore) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/projects/{projectID}", func(r chi.Router) {
		r.Use(middleware.BearerAuth(store))
		r.Get("/runs", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return r
}

func TestBearerAuth_NoHeader(t *testing.T) {
	store := buildStore("proj-1", "secret")
	h := makeHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-1/runs", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestBearerAuth_MalformedHeader(t *testing.T) {
	store := buildStore("proj-1", "secret")
	h := makeHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-1/runs", nil)
	req.Header.Set("Authorization", "Token secret") // wrong scheme
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestBearerAuth_InvalidToken(t *testing.T) {
	store := buildStore("proj-1", "correct-token")
	h := makeHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-1/runs", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestBearerAuth_ValidToken_CorrectProject(t *testing.T) {
	store := buildStore("proj-1", "secret")
	h := makeHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-1/runs", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestBearerAuth_ValidToken_WrongProject(t *testing.T) {
	// Token belongs to proj-1, but the URL says proj-2
	store := buildStore("proj-1", "secret")
	h := makeHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-2/runs", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

// mockRunStore satisfies store.RunStore for testing RunAuth.
type mockRunStore struct {
	// projectsByRunID maps runID → projectID; missing key means "not found".
	projectsByRunID map[string]string
}

func (m *mockRunStore) List(_ context.Context, _ string, _, _ int) ([]*domain.Run, error) {
	return nil, nil
}
func (m *mockRunStore) Count(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockRunStore) Get(_ context.Context, _ string) (*domain.Run, error) {
	return nil, nil
}
func (m *mockRunStore) GetMulti(_ context.Context, _ []string) ([]*domain.Run, error) {
	return nil, nil
}
func (m *mockRunStore) ListBySession(_ context.Context, _, _ string) ([]*domain.Run, error) {
	return nil, nil
}
func (m *mockRunStore) GetProjectID(_ context.Context, runID string) (string, error) {
	pid, ok := m.projectsByRunID[runID]
	if !ok {
		return "", fmt.Errorf("run not found: %s", runID)
	}
	return pid, nil
}
func (m *mockRunStore) ListActiveRunIDs(_ context.Context, _ string, _ int) (map[string]bool, error) {
	return nil, nil
}

// makeRunAuthHandler wraps a 200 OK handler behind RunAuth on a Chi route
// that exposes {runID}.
func makeRunAuthHandler(ps *mockProjectStore, rs *mockRunStore) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/runs/{runID}", func(r chi.Router) {
		r.Use(middleware.RunAuth(ps, rs))
		r.Get("/spans", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return r
}

// --- RunAuth tests ---

func TestRunAuth_NoHeader(t *testing.T) {
	ps := buildStore("proj-1", "secret")
	rs := &mockRunStore{projectsByRunID: map[string]string{"run-1": "proj-1"}}
	h := makeRunAuthHandler(ps, rs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/spans", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRunAuth_InvalidAPIKey(t *testing.T) {
	ps := buildStore("proj-1", "correct-token")
	rs := &mockRunStore{projectsByRunID: map[string]string{"run-1": "proj-1"}}
	h := makeRunAuthHandler(ps, rs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/spans", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRunAuth_RunNotFound(t *testing.T) {
	ps := buildStore("proj-1", "secret")
	// The run store has no entry for "run-unknown".
	rs := &mockRunStore{projectsByRunID: map[string]string{}}
	h := makeRunAuthHandler(ps, rs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-unknown/spans", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRunAuth_RunBelongsToDifferentProject(t *testing.T) {
	// Token is valid for proj-1, but run-2 belongs to proj-2.
	ps := buildStore("proj-1", "secret")
	rs := &mockRunStore{projectsByRunID: map[string]string{"run-2": "proj-2"}}
	h := makeRunAuthHandler(ps, rs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-2/spans", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRunAuth_ValidToken_HandlerCalled(t *testing.T) {
	ps := buildStore("proj-1", "secret")
	rs := &mockRunStore{projectsByRunID: map[string]string{"run-1": "proj-1"}}
	h := makeRunAuthHandler(ps, rs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/spans", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRunAuth_ProjectInjectedInContext(t *testing.T) {
	ps := buildStore("proj-1", "secret")
	rs := &mockRunStore{projectsByRunID: map[string]string{"run-1": "proj-1"}}

	r := chi.NewRouter()
	r.Route("/api/v1/runs/{runID}", func(r chi.Router) {
		r.Use(middleware.RunAuth(ps, rs))
		r.Get("/spans", func(w http.ResponseWriter, r *http.Request) {
			p, ok := middleware.ProjectFromContext(r.Context())
			if !ok || p.ID != "proj-1" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/spans", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected project in context, got %d", rr.Code)
	}
}

func TestBearerAuth_ProjectInjectedInContext(t *testing.T) {
	store := buildStore("proj-1", "secret")

	r := chi.NewRouter()
	r.Route("/api/v1/projects/{projectID}", func(r chi.Router) {
		r.Use(middleware.BearerAuth(store))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			p, ok := middleware.ProjectFromContext(r.Context())
			if !ok || p.ID != "proj-1" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-1/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected project in context, got %d", rr.Code)
	}
}
