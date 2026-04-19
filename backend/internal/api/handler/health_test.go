package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/api/handler"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// healthSpanStore is a minimal SpanStore mock used only by health handler tests.
type healthSpanStore struct {
	lastSpanAt *time.Time
	err        error
}

func (m *healthSpanStore) ListByRun(_ context.Context, _ string) ([]*domain.Span, error) {
	return nil, nil
}
func (m *healthSpanStore) GetByID(_ context.Context, _, _ string) (*domain.Span, error) {
	return nil, nil
}
func (m *healthSpanStore) LatestSpanTime(_ context.Context, _ string) (*time.Time, error) {
	return m.lastSpanAt, m.err
}
func (m *healthSpanStore) ListByRunSince(_ context.Context, _ string, _ time.Time) ([]*domain.Span, error) {
	return nil, nil
}
func (m *healthSpanStore) ListByProjectSince(_ context.Context, _ string, _ time.Time) ([]*domain.Span, error) {
	return nil, nil
}
func (m *healthSpanStore) CountSince(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, nil
}
func (m *healthSpanStore) ListLLMSpansByRun(_ context.Context, _ string) ([]*domain.Span, error) {
	return nil, nil
}

func ptr[T any](v T) *T { return &v }

func TestHealthHandler_Status(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name               string
		lastSpanAt         *time.Time
		wantReachable      bool
		wantLastSpanAtNil  bool
	}{
		{
			name:          "recent span — collector reachable",
			lastSpanAt:    ptr(now.Add(-2 * time.Minute)),
			wantReachable: true,
		},
		{
			name:          "stale span — collector not reachable",
			lastSpanAt:    ptr(now.Add(-10 * time.Minute)),
			wantReachable: false,
		},
		{
			name:              "no spans — collector not reachable",
			lastSpanAt:        nil,
			wantReachable:     false,
			wantLastSpanAtNil: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mock := &healthSpanStore{lastSpanAt: tc.lastSpanAt}
			h := handler.NewHealthHandler(mock)

			r := chi.NewRouter()
			r.Get("/projects/{projectID}/health", h.Status)

			req := httptest.NewRequest(http.MethodGet, "/projects/proj-123/health", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}

			var envelope struct {
				Data domain.ProjectHealth `json:"data"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			body := envelope.Data

			if body.CollectorReachable != tc.wantReachable {
				t.Errorf("CollectorReachable = %v, want %v", body.CollectorReachable, tc.wantReachable)
			}

			if tc.wantLastSpanAtNil && body.LastSpanAt != nil {
				t.Errorf("LastSpanAt = %v, want nil", body.LastSpanAt)
			}
			if !tc.wantLastSpanAtNil && body.LastSpanAt == nil {
				t.Errorf("LastSpanAt is nil, want non-nil")
			}
		})
	}
}
