package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/eval"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// builtinEvalNames lists the built-in eval type names (no prompt_template required).
var builtinEvalNames = map[string]bool{
	"relevance":        true,
	"hallucination":    true,
	"faithfulness":     true,
	"toxicity":         true,
	"tool_correctness": true,
}

// EvalConfigHandler handles CRUD for per-project eval configurations.
// providerKeys is used to validate that judge models are usable at request time.
type EvalConfigHandler struct {
	configs      store.EvalConfigStore
	providerKeys eval.ProviderKeys
}

func NewEvalConfigHandler(configs store.EvalConfigStore) *EvalConfigHandler {
	return &EvalConfigHandler{configs: configs}
}

// NewEvalConfigHandlerWithKeys creates the handler with provider keys for model validation.
func NewEvalConfigHandlerWithKeys(configs store.EvalConfigStore, keys eval.ProviderKeys) *EvalConfigHandler {
	return &EvalConfigHandler{configs: configs, providerKeys: keys}
}

func (h *EvalConfigHandler) Routes(r chi.Router) {
	r.Get("/config", h.list)
	// dry-run must be registered before the parameterised /:evalName route.
	r.Post("/config/dry-run", h.DryRun)
	r.Post("/config", h.upsert)
	r.Delete("/config/{evalName}", h.delete)
}

func (h *EvalConfigHandler) list(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	cfgs, err := h.configs.List(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list eval configs")
		return
	}
	httputil.JSON(w, http.StatusOK, cfgs)
}

type evalConfigRequest struct {
	EvalName       string              `json:"eval_name"`
	Enabled        bool                `json:"enabled"`
	SpanKind       string              `json:"span_kind"`
	PromptTemplate *string             `json:"prompt_template,omitempty"`
	ScopeFilter    map[string][]string `json:"scope_filter,omitempty"`
	JudgeModels    []string            `json:"judge_models,omitempty"`
}

func (req *evalConfigRequest) validate() string {
	if req.EvalName == "" {
		return "eval_name is required"
	}
	switch req.SpanKind {
	case "llm.call", "tool.call":
	default:
		return "span_kind must be 'llm.call' or 'tool.call'"
	}
	// Custom eval: must have a prompt template
	if !builtinEvalNames[req.EvalName] {
		if req.PromptTemplate == nil || *req.PromptTemplate == "" {
			return "prompt_template is required for custom eval types"
		}
		if len(*req.PromptTemplate) > 4000 {
			return "prompt_template must be 4000 characters or fewer"
		}
		if !strings.Contains(*req.PromptTemplate, "{{input}}") &&
			!strings.Contains(*req.PromptTemplate, "{{output}}") {
			return "prompt_template must contain {{input}} or {{output}}"
		}
	}
	return ""
}

// validateJudgeModels checks that every model in the list is in SupportedModels
// and that the required provider key is configured. Returns a human-readable
// error string, or "" if all models are valid.
func (h *EvalConfigHandler) validateJudgeModels(models []string) string {
	for _, model := range models {
		provider, ok := eval.SupportedModels[model]
		if !ok {
			supported := make([]string, 0, len(eval.SupportedModels))
			for k := range eval.SupportedModels {
				supported = append(supported, k)
			}
			return fmt.Sprintf("unsupported judge model %q; supported models: %s", model, strings.Join(supported, ", "))
		}
		switch provider {
		case "anthropic":
			if h.providerKeys.Anthropic == "" {
				return fmt.Sprintf("judge model %q requires ANTHROPIC_API_KEY which is not configured", model)
			}
		case "openai":
			if h.providerKeys.OpenAI == "" {
				return fmt.Sprintf("judge model %q requires OPENAI_API_KEY which is not configured", model)
			}
		case "google":
			if h.providerKeys.Google == "" {
				return fmt.Sprintf("judge model %q requires GOOGLE_AI_API_KEY which is not configured", model)
			}
		}
	}
	return ""
}

func (h *EvalConfigHandler) upsert(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req evalConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.validate(); msg != "" {
		httputil.Error(w, http.StatusBadRequest, msg)
		return
	}

	// Validate judge_models if provided.
	if len(req.JudgeModels) > 0 {
		if msg := h.validateJudgeModels(req.JudgeModels); msg != "" {
			httputil.Error(w, http.StatusUnprocessableEntity, msg)
			return
		}
	}

	cfg := &domain.EvalConfig{
		ID:             uuid.New().String(),
		ProjectID:      projectID,
		EvalName:       req.EvalName,
		Enabled:        req.Enabled,
		SpanKind:       req.SpanKind,
		PromptTemplate: req.PromptTemplate,
		PromptVersion:  1, // store auto-increments on template change via ON CONFLICT
		ScopeFilter:    req.ScopeFilter,
		JudgeModels:    req.JudgeModels,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.configs.Upsert(r.Context(), cfg); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save eval config")
		return
	}
	httputil.JSON(w, http.StatusOK, cfg)
}

func (h *EvalConfigHandler) delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	evalName := chi.URLParam(r, "evalName")

	if builtinEvalNames[evalName] {
		httputil.Error(w, http.StatusBadRequest, "built-in eval types cannot be deleted; disable them instead")
		return
	}

	if err := h.configs.Delete(r.Context(), projectID, evalName); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete eval config")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"deleted": evalName})
}

// ── Dry-run ───────────────────────────────────────────────────────────────────

type dryRunRequest struct {
	PromptTemplate string   `json:"prompt_template"`
	JudgeModels    []string `json:"judge_models"`
	TestInput      string   `json:"test_input"`
	TestOutput     string   `json:"test_output"`
}

type dryRunScore struct {
	ModelID   string  `json:"model_id"`
	Score     float32 `json:"score"`
	Rationale string  `json:"rationale"`
}

type dryRunResponse struct {
	Scores []dryRunScore `json:"scores"`
}

// DryRun handles POST /api/v1/projects/:projectID/evals/config/dry-run.
// It renders the prompt template with the provided test inputs and calls each
// selected judge model, returning the score and rationale per model.
// Only standard project Bearer auth is required.
func (h *EvalConfigHandler) DryRun(w http.ResponseWriter, r *http.Request) {
	var req dryRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate prompt_template.
	if req.PromptTemplate == "" {
		httputil.Error(w, http.StatusBadRequest, "prompt_template is required")
		return
	}
	if !strings.Contains(req.PromptTemplate, "{{input}}") && !strings.Contains(req.PromptTemplate, "{{output}}") {
		httputil.Error(w, http.StatusBadRequest, "prompt_template must contain {{input}} or {{output}}")
		return
	}

	// Validate judge_models.
	if len(req.JudgeModels) == 0 {
		httputil.Error(w, http.StatusBadRequest, "judge_models must not be empty")
		return
	}
	if msg := h.validateJudgeModels(req.JudgeModels); msg != "" {
		httputil.Error(w, http.StatusUnprocessableEntity, msg)
		return
	}

	// Render the template.
	rendered := strings.ReplaceAll(req.PromptTemplate, "{{input}}", req.TestInput)
	rendered = strings.ReplaceAll(rendered, "{{output}}", req.TestOutput)

	// Cap with a 30-second timeout.
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	scores := make([]dryRunScore, 0, len(req.JudgeModels))
	for _, modelID := range req.JudgeModels {
		jr, err := eval.CallJudgeModel(ctx, h.providerKeys, modelID, rendered)
		if err != nil {
			httputil.Error(w, http.StatusBadGateway, fmt.Sprintf("judge call failed for model %q: %s", modelID, err.Error()))
			return
		}
		scores = append(scores, dryRunScore{
			ModelID:   modelID,
			Score:     jr.Score,
			Rationale: jr.Reasoning,
		})
	}

	httputil.JSON(w, http.StatusOK, dryRunResponse{Scores: scores})
}
