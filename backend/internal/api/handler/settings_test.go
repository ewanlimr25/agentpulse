package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// mockPIIConfigStore — implements store.ProjectPIIConfigStore
// ---------------------------------------------------------------------------

type mockPIIConfigStore struct {
	config       *domain.ProjectPIIConfig
	getErr       error
	upsertErr    error
	lastUpserted *domain.ProjectPIIConfig
}

func (m *mockPIIConfigStore) Get(_ context.Context, projectID string) (*domain.ProjectPIIConfig, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.config != nil {
		return m.config, nil
	}
	return &domain.ProjectPIIConfig{
		ProjectID:      projectID,
		PIICustomRules: []domain.PIICustomRule{},
	}, nil
}

func (m *mockPIIConfigStore) Upsert(_ context.Context, cfg *domain.ProjectPIIConfig) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.lastUpserted = cfg
	m.config = cfg
	return nil
}

// ---------------------------------------------------------------------------
// mockProjectStoreForSettings — implements store.ProjectStore
// ---------------------------------------------------------------------------

type mockProjectStoreForSettings struct {
	byAPIKey   map[string]*domain.Project
	byAdminKey map[string]*domain.Project
}

func (m *mockProjectStoreForSettings) List(_ context.Context) ([]*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStoreForSettings) Get(_ context.Context, _ string) (*domain.Project, error) {
	return nil, nil
}
func (m *mockProjectStoreForSettings) Create(_ context.Context, _ *domain.Project) error { return nil }
func (m *mockProjectStoreForSettings) GetByAPIKeyHash(_ context.Context, hash string) (*domain.Project, error) {
	p, ok := m.byAPIKey[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}
func (m *mockProjectStoreForSettings) GetByAdminKeyHash(_ context.Context, hash string) (*domain.Project, error) {
	p, ok := m.byAdminKey[hash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildSettingsRouter wires GET (BearerAuth) and PUT (AdminKeyAuth).
// Mirrors the production routing: separate route definitions per method.
func buildSettingsRouter(ps *mockProjectStoreForSettings, pii *mockPIIConfigStore) http.Handler {
	r := chi.NewRouter()
	// nil pool: pg_notify is guarded by nil check in handler.
	h := NewSettingsHandler(pii, (*pgxpool.Pool)(nil))

	bearerAuth := middleware.BearerAuth(ps)
	adminKeyAuth := middleware.AdminKeyAuth(ps)

	r.With(bearerAuth).Get("/api/v1/projects/{projectID}/settings", h.GetSettings)
	r.With(adminKeyAuth).Put("/api/v1/projects/{projectID}/settings", h.PutSettings)

	return r
}

func settingsProjectStore(projectID, apiToken, adminToken string) *mockProjectStoreForSettings {
	p := &domain.Project{ID: projectID, Name: "Test"}
	return &mockProjectStoreForSettings{
		byAPIKey:   map[string]*domain.Project{budgetHashToken(apiToken): p},
		byAdminKey: map[string]*domain.Project{budgetHashToken(adminToken): p},
	}
}

func jsonSettingsBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Test 1: GET returns defaults when no row exists.
func TestGetSettings_Defaults(t *testing.T) {
	ps := settingsProjectStore("proj-1", "apitoken", "admintoken")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-1/settings", nil)
	req.Header.Set("Authorization", "Bearer apitoken")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// Test 2: GET returns actual config when row exists.
func TestGetSettings_ExistingConfig(t *testing.T) {
	ps := settingsProjectStore("proj-2", "apitoken2", "admintoken2")
	pii := &mockPIIConfigStore{
		config: &domain.ProjectPIIConfig{
			ProjectID:           "proj-2",
			PIIRedactionEnabled: true,
			PIICustomRules: []domain.PIICustomRule{
				{Name: "ssn", Pattern: `\d{3}-\d{2}-\d{4}`, Enabled: true},
			},
		},
	}
	h := buildSettingsRouter(ps, pii)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-2/settings", nil)
	req.Header.Set("Authorization", "Bearer apitoken2")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// Test 3: PUT creates config, returns updated.
func TestPutSettings_CreateConfig(t *testing.T) {
	ps := settingsProjectStore("proj-3", "apitoken3", "admintoken3")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	body := jsonSettingsBody(map[string]any{
		"pii_redaction_enabled": true,
		"pii_custom_rules": []map[string]any{
			{"name": "email", "pattern": `[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`, "enabled": true},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-3/settings", body)
	req.Header.Set("X-Admin-Key", "admintoken3")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if pii.lastUpserted == nil {
		t.Fatal("expected Upsert to be called")
	}
	if !pii.lastUpserted.PIIRedactionEnabled {
		t.Error("expected pii_redaction_enabled=true in upserted config")
	}
}

// Test 4: PUT with invalid regex returns 400 with clear error.
func TestPutSettings_InvalidRegex(t *testing.T) {
	ps := settingsProjectStore("proj-4", "apitoken4", "admintoken4")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	body := jsonSettingsBody(map[string]any{
		"pii_redaction_enabled": true,
		"pii_custom_rules": []map[string]any{
			{"name": "bad", "pattern": `[invalid(`, "enabled": true},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-4/settings", body)
	req.Header.Set("X-Admin-Key", "admintoken4")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid regex, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// Test 5: PUT with tautological patterns (.*) returns 400 "pattern is too broad".
func TestPutSettings_TautologicalRegex(t *testing.T) {
	ps := settingsProjectStore("proj-5", "apitoken5", "admintoken5")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	broadPatterns := []string{".*", ".+", `\w+`, `[\s\S]*`}
	for _, broad := range broadPatterns {
		body := jsonSettingsBody(map[string]any{
			"pii_redaction_enabled": true,
			"pii_custom_rules": []map[string]any{
				{"name": "broad", "pattern": broad, "enabled": true},
			},
		})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-5/settings", body)
		req.Header.Set("X-Admin-Key", "admintoken5")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("pattern %q: expected 400 (too broad), got %d — body: %s", broad, rr.Code, rr.Body.String())
		}
	}
}

// Test 6: PUT with >20 custom rules returns 400.
func TestPutSettings_TooManyRules(t *testing.T) {
	ps := settingsProjectStore("proj-6", "apitoken6", "admintoken6")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	rules := make([]map[string]any, 21)
	for i := range rules {
		rules[i] = map[string]any{
			"name":    fmt.Sprintf("rule%d", i),
			"pattern": `\d{4}`,
			"enabled": true,
		}
	}
	body := jsonSettingsBody(map[string]any{
		"pii_redaction_enabled": false,
		"pii_custom_rules":      rules,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-6/settings", body)
	req.Header.Set("X-Admin-Key", "admintoken6")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >20 rules, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// Test 7a: PUT with missing X-Admin-Key returns 401.
func TestPutSettings_MissingAdminKey(t *testing.T) {
	ps := settingsProjectStore("proj-7", "apitoken7", "admintoken7")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	body := jsonSettingsBody(map[string]any{"pii_redaction_enabled": false, "pii_custom_rules": []any{}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-7/settings", body)
	// No X-Admin-Key header
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing admin key, got %d", rr.Code)
	}
}

// Test 7b: PUT with wrong X-Admin-Key returns 403.
func TestPutSettings_WrongAdminKey(t *testing.T) {
	ps := settingsProjectStore("proj-8", "apitoken8", "admintoken8")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	body := jsonSettingsBody(map[string]any{"pii_redaction_enabled": false, "pii_custom_rules": []any{}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-8/settings", body)
	req.Header.Set("X-Admin-Key", "wrongkey")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong admin key, got %d", rr.Code)
	}
}

// Test 8: Empty rule name returns 400.
func TestPutSettings_EmptyRuleName(t *testing.T) {
	ps := settingsProjectStore("proj-9", "apitoken9", "admintoken9")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	body := jsonSettingsBody(map[string]any{
		"pii_redaction_enabled": false,
		"pii_custom_rules": []map[string]any{
			{"name": "", "pattern": `\d{4}`, "enabled": true},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/proj-9/settings", body)
	req.Header.Set("X-Admin-Key", "admintoken9")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty rule name, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// Test 9: GET response has correct envelope shape with data field.
func TestGetSettings_ResponseShape(t *testing.T) {
	ps := settingsProjectStore("proj-10", "apitoken10", "admintoken10")
	pii := &mockPIIConfigStore{}
	h := buildSettingsRouter(ps, pii)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/proj-10/settings", nil)
	req.Header.Set("Authorization", "Bearer apitoken10")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var env struct {
		Data *domain.ProjectPIIConfig `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Data == nil {
		t.Fatal("expected non-nil data in response")
	}
}
