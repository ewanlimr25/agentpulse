package handler

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const (
	maxTagLength       = 64
	maxTagsPerRun      = 50
	maxAnnotationLen   = 5000
)

var tagPattern = regexp.MustCompile(`^[a-zA-Z0-9_:\-\.]+$`)

// RunTagsHandler handles tag and annotation operations on runs.
type RunTagsHandler struct {
	tags        store.RunTagStore
	annotations store.RunAnnotationStore
}

func NewRunTagsHandler(tags store.RunTagStore, annotations store.RunAnnotationStore) *RunTagsHandler {
	return &RunTagsHandler{tags: tags, annotations: annotations}
}

// AddTag handles POST /runs/{runID}/tags.
// Body: {"tag": "some-tag"}
func (h *RunTagsHandler) AddTag(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	project, ok := middleware.ProjectFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	var req struct {
		Tag string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Tag == "" {
		httputil.Error(w, http.StatusBadRequest, "tag is required")
		return
	}
	if len(req.Tag) > maxTagLength {
		httputil.Error(w, http.StatusBadRequest, "tag must not exceed 64 characters")
		return
	}
	if !tagPattern.MatchString(req.Tag) {
		httputil.Error(w, http.StatusBadRequest, "tag may only contain letters, digits, underscores, colons, hyphens, and dots")
		return
	}

	// Enforce max-tags-per-run limit.
	existing, err := h.tags.List(r.Context(), project.ID, runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	if len(existing) >= maxTagsPerRun {
		httputil.Error(w, http.StatusUnprocessableEntity, "run already has the maximum of 50 tags")
		return
	}

	if err := h.tags.Add(r.Context(), project.ID, runID, req.Tag); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to add tag")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// RemoveTag handles DELETE /runs/{runID}/tags/{tag}.
func (h *RunTagsHandler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	tag := chi.URLParam(r, "tag")

	project, ok := middleware.ProjectFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	if err := h.tags.Delete(r.Context(), project.ID, runID, tag); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to remove tag")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListTags handles GET /runs/{runID}/tags.
// Returns {"tags": ["..."]}
func (h *RunTagsHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	project, ok := middleware.ProjectFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	tags, err := h.tags.List(r.Context(), project.ID, runID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	if tags == nil {
		tags = []string{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// UpsertAnnotation handles PUT /runs/{runID}/annotation.
// Body: {"note": "..."}
func (h *RunTagsHandler) UpsertAnnotation(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	project, ok := middleware.ProjectFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Note) > maxAnnotationLen {
		httputil.Error(w, http.StatusBadRequest, "note must not exceed 5000 characters")
		return
	}

	a := &domain.RunAnnotation{
		ID:        uuid.New().String(),
		ProjectID: project.ID,
		RunID:     runID,
		Note:      req.Note,
	}
	if err := h.annotations.Upsert(r.Context(), a); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save annotation")
		return
	}
	httputil.JSON(w, http.StatusOK, a)
}

// DeleteAnnotation handles DELETE /runs/{runID}/annotation.
func (h *RunTagsHandler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	project, ok := middleware.ProjectFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	if err := h.annotations.Delete(r.Context(), project.ID, runID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to delete annotation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListProjectTags handles GET /projects/{projectID}/tags.
// Returns all distinct tags used within the project (for filter dropdown population).
func (h *RunTagsHandler) ListProjectTags(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	tags, err := h.tags.ListAllTags(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list project tags")
		return
	}
	if tags == nil {
		tags = []string{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"tags": tags})
}
