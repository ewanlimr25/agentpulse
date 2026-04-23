package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type AlertRuleHandler struct {
	rules store.AlertRuleStore
}

func NewAlertRuleHandler(rules store.AlertRuleStore) *AlertRuleHandler {
	return &AlertRuleHandler{rules: rules}
}

func (h *AlertRuleHandler) Routes(r chi.Router) {
	r.Get("/rules", h.listRules)
	r.Post("/rules", h.createRule)
	r.Put("/rules/{ruleID}", h.updateRule)
	r.Delete("/rules/{ruleID}", h.deleteRule)
	r.Get("/events", h.listEvents)
}

// ListRecent is mounted at GET /api/v1/projects/{projectID}/alerts/events/recent.
func (h *AlertRuleHandler) ListRecent(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	events, err := h.rules.ListRecentEvents(r.Context(), projectID, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list recent alert events")
		return
	}
	httputil.JSON(w, http.StatusOK, events)
}

func (h *AlertRuleHandler) listRules(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	rules, err := h.rules.ListRules(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list alert rules")
		return
	}
	httputil.JSON(w, http.StatusOK, rules)
}

type alertRuleRequest struct {
	Name              string            `json:"name"`
	SignalType        domain.SignalType `json:"signal_type"`
	Threshold         float64           `json:"threshold"`
	CompareOp         domain.CompareOp  `json:"compare_op"`
	WindowSeconds     int               `json:"window_seconds"`
	ScopeFilter       *string           `json:"scope_filter,omitempty"`
	WebhookURL        *string           `json:"webhook_url,omitempty"`
	SlackWebhookURL   *string           `json:"slack_webhook_url,omitempty"`
	DiscordWebhookURL *string           `json:"discord_webhook_url,omitempty"`
	Enabled           bool              `json:"enabled"`
}

func (req *alertRuleRequest) validate() string {
	if req.Name == "" {
		return "name is required"
	}
	switch req.SignalType {
	case domain.SignalTypeErrorRate, domain.SignalTypeLatencyP95,
		domain.SignalTypeQualityScore, domain.SignalTypeToolFailure,
		domain.SignalTypeAgentLoop:
	default:
		return "signal_type must be one of: error_rate, latency_p95, quality_score, tool_failure, agent_loop"
	}
	if req.Threshold < 0 {
		return "threshold must be >= 0"
	}
	switch req.CompareOp {
	case domain.CompareOpGt, domain.CompareOpLt:
	default:
		return "compare_op must be 'gt' or 'lt'"
	}
	if req.WindowSeconds <= 0 {
		return "window_seconds must be > 0"
	}
	if req.SignalType == domain.SignalTypeToolFailure && (req.ScopeFilter == nil || *req.ScopeFilter == "") {
		return "scope_filter (tool name) is required for tool_failure signal type"
	}
	if req.SlackWebhookURL != nil && *req.SlackWebhookURL != "" {
		if !strings.HasPrefix(*req.SlackWebhookURL, "https://hooks.slack.com/services/") {
			return "slack_webhook_url must start with https://hooks.slack.com/services/"
		}
	}
	return ""
}

func (h *AlertRuleHandler) createRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req alertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		httputil.Error(w, http.StatusBadRequest, msg)
		return
	}
	if req.WebhookURL != nil && *req.WebhookURL != "" {
		if msg := validateWebhookURL(*req.WebhookURL); msg != "" {
			httputil.Error(w, http.StatusBadRequest, msg)
			return
		}
	}

	rule := &domain.AlertRule{
		ID:                uuid.New().String(),
		ProjectID:         projectID,
		Name:              req.Name,
		SignalType:        req.SignalType,
		Threshold:         req.Threshold,
		CompareOp:         req.CompareOp,
		WindowSeconds:     req.WindowSeconds,
		ScopeFilter:       req.ScopeFilter,
		WebhookURL:        req.WebhookURL,
		SlackWebhookURL:   req.SlackWebhookURL,
		DiscordWebhookURL: req.DiscordWebhookURL,
		Enabled:           req.Enabled,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := h.rules.CreateRule(r.Context(), rule); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to create alert rule")
		return
	}
	httputil.JSON(w, http.StatusCreated, rule)
}

func (h *AlertRuleHandler) updateRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	ruleID := chi.URLParam(r, "ruleID")

	existing, err := h.rules.GetRule(r.Context(), ruleID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "rule not found")
		return
	}
	if existing.ProjectID != projectID {
		httputil.Error(w, http.StatusForbidden, "rule does not belong to this project")
		return
	}

	var req alertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		httputil.Error(w, http.StatusBadRequest, msg)
		return
	}
	if req.WebhookURL != nil && *req.WebhookURL != "" {
		if msg := validateWebhookURL(*req.WebhookURL); msg != "" {
			httputil.Error(w, http.StatusBadRequest, msg)
			return
		}
	}

	existing.Name = req.Name
	existing.SignalType = req.SignalType
	existing.Threshold = req.Threshold
	existing.CompareOp = req.CompareOp
	existing.WindowSeconds = req.WindowSeconds
	existing.ScopeFilter = req.ScopeFilter
	existing.WebhookURL = req.WebhookURL
	existing.SlackWebhookURL = req.SlackWebhookURL
	existing.DiscordWebhookURL = req.DiscordWebhookURL
	existing.Enabled = req.Enabled
	existing.UpdatedAt = time.Now()

	if err := h.rules.UpdateRule(r.Context(), existing); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to update alert rule")
		return
	}
	httputil.JSON(w, http.StatusOK, existing)
}

func (h *AlertRuleHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	ruleID := chi.URLParam(r, "ruleID")

	existing, err := h.rules.GetRule(r.Context(), ruleID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "rule not found")
		return
	}
	if existing.ProjectID != projectID {
		httputil.Error(w, http.StatusForbidden, "rule does not belong to this project")
		return
	}

	if err := h.rules.DeleteRule(r.Context(), ruleID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete alert rule")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"deleted": ruleID})
}

func (h *AlertRuleHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 100)
	events, err := h.rules.ListEvents(r.Context(), projectID, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list alert events")
		return
	}
	httputil.JSON(w, http.StatusOK, events)
}
