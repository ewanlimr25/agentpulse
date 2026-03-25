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

// builtinEvalNames lists the built-in eval type names (no prompt_template required).
var builtinEvalNames = map[string]bool{
	"relevance":        true,
	"hallucination":    true,
	"faithfulness":     true,
	"toxicity":         true,
	"tool_correctness": true,
}

type EvalConfigHandler struct {
	configs store.EvalConfigStore
}

func NewEvalConfigHandler(configs store.EvalConfigStore) *EvalConfigHandler {
	return &EvalConfigHandler{configs: configs}
}

func (h *EvalConfigHandler) Routes(r chi.Router) {
	r.Get("/config", h.list)
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

	cfg := &domain.EvalConfig{
		ID:             uuid.New().String(),
		ProjectID:      projectID,
		EvalName:       req.EvalName,
		Enabled:        req.Enabled,
		SpanKind:       req.SpanKind,
		PromptTemplate: req.PromptTemplate,
		PromptVersion:  1, // store auto-increments on template change via ON CONFLICT
		ScopeFilter:    req.ScopeFilter,
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
