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
