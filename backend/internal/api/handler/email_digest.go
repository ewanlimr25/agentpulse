package handler

import (
	"encoding/json"
	"net/http"
	"net/mail"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// EmailDigestHandler manages per-project email digest configuration.
type EmailDigestHandler struct {
	digests store.EmailDigestStore
}

// NewEmailDigestHandler returns a new EmailDigestHandler.
func NewEmailDigestHandler(digests store.EmailDigestStore) *EmailDigestHandler {
	return &EmailDigestHandler{digests: digests}
}

// Routes registers the email digest routes on r.
func (h *EmailDigestHandler) Routes(r chi.Router) {
	r.Get("/", h.get)
	r.Put("/", h.upsert)
}

func (h *EmailDigestHandler) get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	cfg, err := h.digests.Get(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to get email digest config")
		return
	}
	if cfg == nil {
		// Return a default empty config when none exists yet.
		cfg = &domain.EmailDigestConfig{
			ProjectID: projectID,
			Enabled:   false,
			Schedule:  "daily",
		}
	}
	httputil.JSON(w, http.StatusOK, cfg)
}

type emailDigestRequest struct {
	Enabled        bool   `json:"enabled"`
	RecipientEmail string `json:"recipient_email"`
	Schedule       string `json:"schedule"`
}

func (h *EmailDigestHandler) upsert(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req emailDigestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Enabled && req.RecipientEmail != "" {
		if _, err := mail.ParseAddress(req.RecipientEmail); err != nil {
			httputil.Error(w, http.StatusBadRequest, "recipient_email is not a valid email address")
			return
		}
	}
	if req.Enabled && req.RecipientEmail == "" {
		httputil.Error(w, http.StatusBadRequest, "recipient_email is required when enabled is true")
		return
	}

	if req.Schedule == "" {
		req.Schedule = "daily"
	}
	if req.Schedule != "daily" && req.Schedule != "hourly" {
		httputil.Error(w, http.StatusBadRequest, "schedule must be 'daily' or 'hourly'")
		return
	}

	cfg := &domain.EmailDigestConfig{
		ProjectID:      projectID,
		Enabled:        req.Enabled,
		RecipientEmail: req.RecipientEmail,
		Schedule:       req.Schedule,
	}

	if err := h.digests.Upsert(r.Context(), cfg); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save email digest config")
		return
	}

	// Re-fetch to return the full persisted record.
	saved, err := h.digests.Get(r.Context(), projectID)
	if err != nil || saved == nil {
		httputil.JSON(w, http.StatusOK, cfg)
		return
	}
	httputil.JSON(w, http.StatusOK, saved)
}
