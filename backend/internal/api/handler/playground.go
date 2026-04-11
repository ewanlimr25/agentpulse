package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/llmclient"
	"github.com/agentpulse/agentpulse/backend/internal/pricing"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const maxPlaygroundPayloadBytes = 256 * 1024 // 256 KiB

// PlaygroundHandler serves the prompt playground endpoints.
type PlaygroundHandler struct {
	store   store.PlaygroundStore
	llm     llmclient.Client
	pricing *pricing.Table
}

// NewPlaygroundHandler constructs a PlaygroundHandler.
func NewPlaygroundHandler(store store.PlaygroundStore, llm llmclient.Client, pricing *pricing.Table) *PlaygroundHandler {
	return &PlaygroundHandler{store: store, llm: llm, pricing: pricing}
}

// Routes mounts playground sub-routes on the given router.
func (h *PlaygroundHandler) Routes(r chi.Router) {
	r.Post("/sessions", h.CreateSession)
	r.Get("/sessions", h.ListSessions)
	r.Get("/sessions/{sessionID}", h.GetSession)
	r.Delete("/sessions/{sessionID}", h.DeleteSession)
	r.Put("/sessions/{sessionID}/variants/{variantID}", h.UpdateVariant)
	r.Post("/sessions/{sessionID}/variants/{variantID}/run", h.RunVariant)
}

// --- request types ---

type createSessionReq struct {
	Name         string             `json:"name"`
	SourceSpanID *string            `json:"source_span_id"`
	SourceRunID  *string            `json:"source_run_id"`
	Variants     []createVariantReq `json:"variants"`
}

type createVariantReq struct {
	Label       string                     `json:"label"`
	ModelID     string                     `json:"model_id"`
	System      string                     `json:"system"`
	Messages    []domain.PlaygroundMessage `json:"messages"`
	Temperature *float32                   `json:"temperature"`
	MaxTokens   *int                       `json:"max_tokens"`
}

type updateVariantReq struct {
	Label       *string                    `json:"label"`
	ModelID     *string                    `json:"model_id"`
	System      *string                    `json:"system"`
	Messages    []domain.PlaygroundMessage `json:"messages"`
	Temperature *float32                   `json:"temperature"`
	MaxTokens   *int                       `json:"max_tokens"`
}

// CreateSession handles POST /playground/sessions.
func (h *PlaygroundHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req createSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Variants) == 0 {
		httputil.Error(w, http.StatusBadRequest, "at least 1 variant is required")
		return
	}
	for i, v := range req.Variants {
		if v.Label == "" {
			httputil.Error(w, http.StatusBadRequest, fmt.Sprintf("variant[%d]: label is required", i))
			return
		}
		if v.ModelID == "" {
			httputil.Error(w, http.StatusBadRequest, fmt.Sprintf("variant[%d]: model_id is required", i))
			return
		}
		if len(v.Messages) == 0 {
			httputil.Error(w, http.StatusBadRequest, fmt.Sprintf("variant[%d]: at least 1 message is required", i))
			return
		}
	}

	now := time.Now().UTC()
	session := &domain.PlaygroundSession{
		ID:           uuid.NewString(),
		ProjectID:    projectID,
		Name:         req.Name,
		SourceSpanID: req.SourceSpanID,
		SourceRunID:  req.SourceRunID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	variants := make([]*domain.PlaygroundVariant, 0, len(req.Variants))
	for _, v := range req.Variants {
		variants = append(variants, &domain.PlaygroundVariant{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			Label:       v.Label,
			ModelID:     v.ModelID,
			System:      v.System,
			Messages:    v.Messages,
			Temperature: v.Temperature,
			MaxTokens:   v.MaxTokens,
			UpdatedAt:   now,
		})
	}
	session.Variants = variants

	if err := h.store.CreateSession(r.Context(), session); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	httputil.JSON(w, http.StatusCreated, session)
}

// ListSessions handles GET /playground/sessions.
func (h *PlaygroundHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)

	sessions, err := h.store.ListSessionsByProject(r.Context(), projectID, limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	total, err := h.store.CountSessionsByProject(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to count sessions")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// GetSession handles GET /playground/sessions/{sessionID}.
func (h *PlaygroundHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to get session")
		return
	}
	if session == nil {
		httputil.Error(w, http.StatusNotFound, "session not found")
		return
	}

	// Load recent executions for each variant.
	for _, v := range session.Variants {
		execs, execErr := h.store.ListExecutionsByVariant(r.Context(), v.ID, 5)
		if execErr != nil {
			httputil.Error(w, http.StatusInternalServerError, "failed to load executions")
			return
		}
		v.Executions = execs
	}

	httputil.JSON(w, http.StatusOK, session)
}

// DeleteSession handles DELETE /playground/sessions/{sessionID}.
func (h *PlaygroundHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	if err := h.store.DeleteSession(r.Context(), sessionID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete session")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"deleted": sessionID})
}

// UpdateVariant handles PUT /playground/sessions/{sessionID}/variants/{variantID}.
func (h *PlaygroundHandler) UpdateVariant(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	variantID := chi.URLParam(r, "variantID")

	var req updateVariantReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Load the existing session to find the current variant state.
	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	if session == nil {
		httputil.Error(w, http.StatusNotFound, "session not found")
		return
	}

	var existing *domain.PlaygroundVariant
	for _, v := range session.Variants {
		if v.ID == variantID {
			existing = v
			break
		}
	}

	// Build the updated variant, merging partial fields with existing if present.
	variant := &domain.PlaygroundVariant{
		ID:        variantID,
		SessionID: sessionID,
		UpdatedAt: time.Now().UTC(),
	}

	if existing != nil {
		variant.Label = existing.Label
		variant.ModelID = existing.ModelID
		variant.System = existing.System
		variant.Messages = existing.Messages
		variant.Temperature = existing.Temperature
		variant.MaxTokens = existing.MaxTokens
	}

	if req.Label != nil {
		variant.Label = *req.Label
	}
	if req.ModelID != nil {
		variant.ModelID = *req.ModelID
	}
	if req.System != nil {
		variant.System = *req.System
	}
	if req.Messages != nil {
		variant.Messages = req.Messages
	}
	if req.Temperature != nil {
		variant.Temperature = req.Temperature
	}
	if req.MaxTokens != nil {
		variant.MaxTokens = req.MaxTokens
	}

	if err := h.store.UpsertVariant(r.Context(), variant); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to update variant")
		return
	}

	httputil.JSON(w, http.StatusOK, variant)
}

// RunVariant handles POST /playground/sessions/{sessionID}/variants/{variantID}/run.
func (h *PlaygroundHandler) RunVariant(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	variantID := chi.URLParam(r, "variantID")

	// 1. Load the variant from the session.
	session, err := h.store.GetSession(r.Context(), sessionID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	if session == nil {
		httputil.Error(w, http.StatusNotFound, "session not found")
		return
	}

	var variant *domain.PlaygroundVariant
	for _, v := range session.Variants {
		if v.ID == variantID {
			variant = v
			break
		}
	}
	if variant == nil {
		httputil.Error(w, http.StatusNotFound, "variant not found")
		return
	}

	// 2. Validate payload size.
	payload, err := json.Marshal(variant.Messages)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to marshal messages")
		return
	}
	if len(payload) > maxPlaygroundPayloadBytes {
		httputil.Error(w, http.StatusRequestEntityTooLarge, "payload exceeds 256 KiB limit")
		return
	}

	// 3. Validate temperature.
	if variant.Temperature != nil {
		t := *variant.Temperature
		if t < 0 || t > 2 {
			httputil.Error(w, http.StatusBadRequest, "temperature must be between 0 and 2")
			return
		}
	}

	// 4. Cap max_tokens at 4096.
	maxTokens := 0
	if variant.MaxTokens != nil {
		maxTokens = *variant.MaxTokens
		if maxTokens > 4096 {
			maxTokens = 4096
		}
	}

	// 5. Build the completion request.
	msgs := make([]llmclient.Message, 0, len(variant.Messages))
	for _, m := range variant.Messages {
		msgs = append(msgs, llmclient.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	completionReq := llmclient.CompletionRequest{
		Model:       variant.ModelID,
		System:      variant.System,
		Messages:    msgs,
		Temperature: variant.Temperature,
		MaxTokens:   maxTokens,
	}

	// 6. Call the LLM provider.
	resp, llmErr := h.llm.Complete(r.Context(), completionReq)

	// 7. Handle LLM errors.
	if llmErr != nil {
		errMsg := llmErr.Error()
		switch {
		case strings.Contains(errMsg, "API key is empty"):
			httputil.Error(w, http.StatusServiceUnavailable, "provider not configured")
			return
		case r.Context().Err() == context.DeadlineExceeded:
			httputil.Error(w, http.StatusGatewayTimeout, "LLM request timed out")
			return
		default:
			httputil.Error(w, http.StatusBadGateway, "LLM provider error")
			return
		}
	}

	// 8. Compute cost.
	cost := h.pricing.Cost(variant.ModelID, resp.InputTokens, resp.OutputTokens)

	// 9. Build execution record.
	exec := &domain.PlaygroundExecution{
		ID:           uuid.NewString(),
		VariantID:    variantID,
		Output:       &resp.Text,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		CostUSD:      cost,
		LatencyMS:    int(resp.LatencyMS),
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.store.RecordExecution(r.Context(), exec); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to record execution")
		return
	}

	// 10. Return the execution.
	httputil.JSON(w, http.StatusOK, exec)
}

// queryInt reads an integer query param with a default value.
func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
