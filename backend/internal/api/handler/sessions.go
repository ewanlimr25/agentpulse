package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type SessionHandler struct {
	sessions store.SessionStore
	runs     store.RunStore
}

func NewSessionHandler(sessions store.SessionStore, runs store.RunStore) *SessionHandler {
	return &SessionHandler{sessions: sessions, runs: runs}
}

func (h *SessionHandler) Routes(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/{sessionID}", h.Get)
	r.Get("/{sessionID}/runs", h.ListRuns)
}

// List returns paginated sessions for a project, newest first.
// Route: GET /api/v1/projects/{projectID}/sessions
func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 50)
	offset := intQueryParam(r, "offset", 0)

	sessions, err := h.sessions.List(r.Context(), projectID, limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	total, err := h.sessions.Count(r.Context(), projectID)
	if err != nil {
		total = 0 // non-fatal
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// Get returns a single session aggregate.
// Route: GET /api/v1/projects/{projectID}/sessions/{sessionID}
func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")

	sess, err := h.sessions.Get(r.Context(), projectID, sessionID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "session not found")
		return
	}
	httputil.JSON(w, http.StatusOK, sess)
}

// ListRuns returns all runs in a session, oldest first.
// Route: GET /api/v1/projects/{projectID}/sessions/{sessionID}/runs
func (h *SessionHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	sessionID := chi.URLParam(r, "sessionID")

	runs, err := h.runs.ListBySession(r.Context(), projectID, sessionID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list session runs")
		return
	}
	httputil.JSON(w, http.StatusOK, runs)
}
