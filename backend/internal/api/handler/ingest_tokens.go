package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/authutil"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// IngestTokenHandler manages ingest token CRUD for a project.
type IngestTokenHandler struct {
	tokens store.IngestTokenStore
}

func NewIngestTokenHandler(tokens store.IngestTokenStore) *IngestTokenHandler {
	return &IngestTokenHandler{tokens: tokens}
}

type createIngestTokenRequest struct {
	Label string `json:"label"`
}

type createIngestTokenResponse struct {
	Token     string `json:"token"`
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
}

// Create generates a new ingest token for the project and returns the raw token once.
func (h *IngestTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req createIngestTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rawToken := uuid.New().String()
	tokenHash := authutil.HashToken(rawToken)

	t, err := h.tokens.Create(r.Context(), projectID, tokenHash, req.Label)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to create ingest token")
		return
	}

	// Return the raw token exactly once — it is not recoverable after this response.
	httputil.JSON(w, http.StatusCreated, createIngestTokenResponse{
		Token:     rawToken,
		ID:        t.ID,
		Label:     t.Label,
		CreatedAt: t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// List returns all ingest tokens for the project (no raw tokens or hashes).
func (h *IngestTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	tokens, err := h.tokens.ListByProject(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list ingest tokens")
		return
	}
	// Ensure we return an empty array rather than null when there are no tokens.
	if tokens == nil {
		httputil.JSON(w, http.StatusOK, []any{})
		return
	}
	httputil.JSON(w, http.StatusOK, tokens)
}

// Delete removes an ingest token scoped to the project.
func (h *IngestTokenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	tokenID := chi.URLParam(r, "tokenID")

	if err := h.tokens.Delete(r.Context(), tokenID, projectID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete ingest token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
