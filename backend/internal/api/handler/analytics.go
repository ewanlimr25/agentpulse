package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/pricing"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type AnalyticsHandler struct {
	analytics    store.AnalyticsStore
	pricingTable *pricing.Table
}

func NewAnalyticsHandler(analytics store.AnalyticsStore, pricingTable *pricing.Table) *AnalyticsHandler {
	return &AnalyticsHandler{analytics: analytics, pricingTable: pricingTable}
}

func (h *AnalyticsHandler) Routes(r chi.Router) {
	r.Get("/tools", h.toolStats)
	r.Get("/agents", h.agentCostStats)
	r.Get("/models", h.modelStats)
}

func parseWindow(r *http.Request) int {
	switch r.URL.Query().Get("window") {
	case "7d":
		return 7 * 24 * 3600
	default:
		return 24 * 3600 // 24h default
	}
}

func (h *AnalyticsHandler) toolStats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	window := parseWindow(r)

	tools, err := h.analytics.ToolStats(r.Context(), projectID, window)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query tool stats")
		return
	}
	if tools == nil {
		tools = []*domain.ToolStats{} // return [] not null
	}

	windowLabel := "24h"
	if window == 7*24*3600 {
		windowLabel = "7d"
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"tools":  tools,
		"window": windowLabel,
	})
}

func (h *AnalyticsHandler) agentCostStats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	window := parseWindow(r)

	agents, err := h.analytics.AgentCostStats(r.Context(), projectID, window)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query agent cost stats")
		return
	}
	if agents == nil {
		agents = []*domain.AgentCostStats{} // return [] not null
	}

	windowLabel := "24h"
	if window == 7*24*3600 {
		windowLabel = "7d"
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"window": windowLabel,
	})
}

func (h *AnalyticsHandler) modelStats(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	window := parseWindow(r)

	models, err := h.analytics.ModelStats(r.Context(), projectID, window)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query model stats")
		return
	}
	if models == nil {
		models = []*domain.ModelStats{}
	}

	// Enrich provider from pricing table.
	if h.pricingTable != nil {
		for _, m := range models {
			if info, ok := h.pricingTable.Models[m.ModelID]; ok {
				m.Provider = info.Provider
			}
		}
	}

	windowLabel := "24h"
	if window == 7*24*3600 {
		windowLabel = "7d"
	}

	// Include pricing rates so frontend can compute "what-if" projections client-side.
	type modelPricing struct {
		InputPerMillion  float64 `json:"input_per_million"`
		OutputPerMillion float64 `json:"output_per_million"`
	}
	pricingMap := make(map[string]modelPricing)
	if h.pricingTable != nil {
		for id, m := range h.pricingTable.Models {
			pricingMap[id] = modelPricing{
				InputPerMillion:  m.InputPerMillion,
				OutputPerMillion: m.OutputPerMillion,
			}
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"models":  models,
		"window":  windowLabel,
		"pricing": pricingMap,
	})
}
