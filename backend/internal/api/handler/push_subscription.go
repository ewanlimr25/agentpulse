package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// PushSubscriptionHandler manages browser Web Push subscriptions for a project.
type PushSubscriptionHandler struct {
	subs           store.PushSubscriptionStore
	vapidPublicKey string
}

// NewPushSubscriptionHandler returns a new PushSubscriptionHandler.
func NewPushSubscriptionHandler(subs store.PushSubscriptionStore, vapidPublicKey string) *PushSubscriptionHandler {
	return &PushSubscriptionHandler{subs: subs, vapidPublicKey: vapidPublicKey}
}

// Routes registers the push subscription routes on r.
func (h *PushSubscriptionHandler) Routes(r chi.Router) {
	r.Get("/vapid-public-key", h.getVAPIDPublicKey)
	r.Post("/subscribe", h.subscribe)
	r.Delete("/subscribe", h.unsubscribe)
}

func (h *PushSubscriptionHandler) getVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]string{"key": h.vapidPublicKey})
}

type subscribeRequest struct {
	Endpoint       string `json:"endpoint"`
	P256DHKey      string `json:"p256dh_key"`
	AuthKey        string `json:"auth_key"`
	VAPIDPublicKey string `json:"vapid_public_key"`
	UserAgent      string `json:"user_agent"`
}

func (h *PushSubscriptionHandler) subscribe(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" {
		httputil.Error(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if req.P256DHKey == "" {
		httputil.Error(w, http.StatusBadRequest, "p256dh_key is required")
		return
	}
	if req.AuthKey == "" {
		httputil.Error(w, http.StatusBadRequest, "auth_key is required")
		return
	}

	sub := &domain.PushSubscription{
		ProjectID:      projectID,
		Endpoint:       req.Endpoint,
		P256DHKey:      req.P256DHKey,
		AuthKey:        req.AuthKey,
		VAPIDPublicKey: req.VAPIDPublicKey,
		UserAgent:      req.UserAgent,
	}

	if err := h.subs.Upsert(r.Context(), sub); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save subscription")
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (h *PushSubscriptionHandler) unsubscribe(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req unsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" {
		httputil.Error(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	if err := h.subs.Delete(r.Context(), projectID, req.Endpoint); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to remove subscription")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
