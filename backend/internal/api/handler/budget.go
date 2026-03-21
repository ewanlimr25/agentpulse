package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type BudgetHandler struct {
	budget store.BudgetStore
}

func NewBudgetHandler(budget store.BudgetStore) *BudgetHandler {
	return &BudgetHandler{budget: budget}
}

func (h *BudgetHandler) Routes(r chi.Router) {
	// Rules — mounted under /api/v1/projects/{projectID}/budget
	r.Get("/rules", h.listRules)
	r.Post("/rules", h.createRule)
	r.Put("/rules/{ruleID}", h.updateRule)
	r.Delete("/rules/{ruleID}", h.deleteRule)
	r.Get("/alerts", h.listAlerts)
}

// ListRecent is mounted at GET /api/v1/budget/alerts/recent (no project scope).
func (h *BudgetHandler) ListRecent(w http.ResponseWriter, r *http.Request) {
	limit := intQueryParam(r, "limit", 20)
	alerts, err := h.budget.ListRecentAlerts(r.Context(), limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list recent alerts")
		return
	}
	httputil.JSON(w, http.StatusOK, alerts)
}

func (h *BudgetHandler) listRules(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	rules, err := h.budget.ListRules(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	httputil.JSON(w, http.StatusOK, rules)
}

type ruleRequest struct {
	Name         string               `json:"name"`
	ThresholdUSD float64              `json:"threshold_usd"`
	Action       domain.BudgetAction  `json:"action"`
	Scope        domain.BudgetScope   `json:"scope"`
	WindowSecs   *int                 `json:"window_seconds,omitempty"`
	WebhookURL   *string              `json:"webhook_url,omitempty"`
	Enabled      bool                 `json:"enabled"`
}

func (h *BudgetHandler) createRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.ThresholdUSD <= 0 {
		httputil.Error(w, http.StatusBadRequest, "name and threshold_usd > 0 required")
		return
	}

	rule := &domain.BudgetRule{
		ID:           uuid.New().String(),
		ProjectID:    projectID,
		Name:         req.Name,
		ThresholdUSD: req.ThresholdUSD,
		Action:       req.Action,
		Scope:        req.Scope,
		WindowSeconds: req.WindowSecs,
		WebhookURL:   req.WebhookURL,
		Enabled:      req.Enabled,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.budget.CreateRule(r.Context(), rule); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to create rule")
		return
	}
	httputil.JSON(w, http.StatusCreated, rule)
}

func (h *BudgetHandler) updateRule(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "ruleID")

	existing, err := h.budget.GetRule(r.Context(), ruleID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "rule not found")
		return
	}

	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	existing.Name = req.Name
	existing.ThresholdUSD = req.ThresholdUSD
	existing.Action = req.Action
	existing.Scope = req.Scope
	existing.WindowSeconds = req.WindowSecs
	existing.WebhookURL = req.WebhookURL
	existing.Enabled = req.Enabled
	existing.UpdatedAt = time.Now()

	if err := h.budget.UpdateRule(r.Context(), existing); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to update rule")
		return
	}
	httputil.JSON(w, http.StatusOK, existing)
}

func (h *BudgetHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	ruleID := chi.URLParam(r, "ruleID")
	if err := h.budget.DeleteRule(r.Context(), ruleID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"deleted": ruleID})
}

func (h *BudgetHandler) listAlerts(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 100)
	alerts, err := h.budget.ListAlerts(r.Context(), projectID, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}
	httputil.JSON(w, http.StatusOK, alerts)
}
