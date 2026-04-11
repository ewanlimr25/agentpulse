package handler

import (
	"net/http"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/llmclient"
	"github.com/agentpulse/agentpulse/backend/internal/pricing"
)

// ModelsHandler serves the /api/v1/models endpoint.
type ModelsHandler struct {
	pricing *pricing.Table
	keys    llmclient.ProviderKeys
}

// NewModelsHandler constructs a ModelsHandler.
func NewModelsHandler(pricing *pricing.Table, keys llmclient.ProviderKeys) *ModelsHandler {
	return &ModelsHandler{pricing: pricing, keys: keys}
}

type modelInfoResponse struct {
	pricing.ModelInfo
	Available bool `json:"available"`
}

// List handles GET /api/v1/models.
func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	models := h.pricing.ModelList()

	result := make([]modelInfoResponse, 0, len(models))
	for _, m := range models {
		result = append(result, modelInfoResponse{
			ModelInfo: m,
			Available: h.providerAvailable(m.Provider),
		})
	}

	httputil.JSON(w, http.StatusOK, result)
}

// providerAvailable returns true if the API key for the given provider is set.
func (h *ModelsHandler) providerAvailable(provider string) bool {
	switch provider {
	case "anthropic":
		return h.keys.Anthropic != ""
	case "openai":
		return h.keys.OpenAI != ""
	case "google":
		return h.keys.Google != ""
	default:
		return false
	}
}
