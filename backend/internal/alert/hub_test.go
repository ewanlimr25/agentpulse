package alert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// mockProjectStore — implements store.ProjectStore for hub tests
// ---------------------------------------------------------------------------

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

func buildHubStore(projectID, token string) *mockProjectStore {
	return &mockProjectStore{
		projects: map[string]*domain.Project{
			hashToken(token): {ID: projectID, Name: "Test Project"},
		},
	}
}

// newTestHub creates a Hub backed by the given project store.
func newTestHub(ps *mockProjectStore) *Hub {
	return NewHub(ps)
}

// ---------------------------------------------------------------------------
// Helper: make an HTTP request to ServeWS via httptest
// ---------------------------------------------------------------------------

// serveWSHTTP issues a plain (non-upgraded) HTTP request to ServeWS so we can
// inspect the HTTP-level response code for all pre-upgrade error paths.
func serveWSHTTP(hub *Hub, url string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	hub.ServeWS(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Test 1: Missing ?project_id → 400
// ---------------------------------------------------------------------------

func TestServeWS_MissingProjectID(t *testing.T) {
	hub := newTestHub(buildHubStore("proj-1", "secret"))
	rr := serveWSHTTP(hub, "/ws/alerts", nil)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when project_id missing, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 2: Missing Authorization header → 401
// ---------------------------------------------------------------------------

func TestServeWS_MissingAuthHeader(t *testing.T) {
	hub := newTestHub(buildHubStore("proj-1", "secret"))
	rr := serveWSHTTP(hub, "/ws/alerts?project_id=proj-1", nil)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when Authorization header missing, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 3: Malformed Authorization header (wrong scheme) → 401
// ---------------------------------------------------------------------------

func TestServeWS_MalformedAuthHeader(t *testing.T) {
	hub := newTestHub(buildHubStore("proj-1", "secret"))
	rr := serveWSHTTP(hub, "/ws/alerts?project_id=proj-1", map[string]string{
		"Authorization": "Token wrongscheme",
	})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for malformed auth header, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 4: Invalid token (not in store) → 401
// ---------------------------------------------------------------------------

func TestServeWS_InvalidToken(t *testing.T) {
	hub := newTestHub(buildHubStore("proj-1", "correct-token"))
	rr := serveWSHTTP(hub, "/ws/alerts?project_id=proj-1", map[string]string{
		"Authorization": "Bearer wrong-token",
	})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 5: Token belongs to a different project → 403
// ---------------------------------------------------------------------------

func TestServeWS_TokenForWrongProject(t *testing.T) {
	// Token is registered for proj-1, but the client claims proj-2.
	hub := newTestHub(buildHubStore("proj-1", "secret"))
	rr := serveWSHTTP(hub, "/ws/alerts?project_id=proj-2", map[string]string{
		"Authorization": "Bearer secret",
	})

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when token belongs to a different project, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 6: Valid token + correct project → WebSocket upgrade (101)
// ---------------------------------------------------------------------------

func TestServeWS_ValidAuth_UpgradesWebSocket(t *testing.T) {
	ps := buildHubStore("proj-1", "secret")
	hub := newTestHub(ps)

	// Mount the hub on a real httptest.Server so the gorilla upgrader can
	// negotiate the WebSocket handshake over a real TCP connection.
	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	// Convert http:// URL to ws://.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?project_id=proj-1"

	dialer := websocket.Dialer{}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer secret")

	conn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		// If the dial fails, report the HTTP status code from the handshake.
		if resp != nil {
			t.Fatalf("WebSocket dial failed: %v (HTTP status %d)", err, resp.StatusCode)
		}
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Test 7: Empty project_id string (explicit empty param) → 400
// ---------------------------------------------------------------------------

func TestServeWS_EmptyProjectIDParam(t *testing.T) {
	hub := newTestHub(buildHubStore("proj-1", "secret"))
	// ?project_id= with no value — URL.Query().Get returns "".
	rr := serveWSHTTP(hub, "/ws/alerts?project_id=", map[string]string{
		"Authorization": "Bearer secret",
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty project_id param, got %d — body: %s", rr.Code, rr.Body.String())
	}
}
