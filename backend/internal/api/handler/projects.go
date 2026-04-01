package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type ProjectHandler struct {
	projects store.ProjectStore
}

func NewProjectHandler(projects store.ProjectStore) *ProjectHandler {
	return &ProjectHandler{projects: projects}
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	projects, err := h.projects.List(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	httputil.JSON(w, http.StatusOK, projects)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	p, err := h.projects.Get(r.Context(), id)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "project not found")
		return
	}
	httputil.JSON(w, http.StatusOK, p)
}

type createProjectRequest struct {
	Name string `json:"name"`
}

type createProjectResponse struct {
	Project  *domain.Project `json:"project"`
	APIKey   string          `json:"api_key"`
	AdminKey string          `json:"admin_key"`
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	// Generate a random API key and store its hash.
	rawKey := uuid.New().String()
	apiKeyHash := sha256.Sum256([]byte(rawKey))

	// Generate a separate admin key for settings mutations.
	rawAdminKey := uuid.New().String()
	adminKeyHash := sha256.Sum256([]byte(rawAdminKey))

	p := &domain.Project{
		ID:           uuid.New().String(),
		Name:         req.Name,
		APIKeyHash:   hex.EncodeToString(apiKeyHash[:]),
		AdminKeyHash: hex.EncodeToString(adminKeyHash[:]),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.projects.Create(r.Context(), p); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	// Return both raw keys once — they are never recoverable after this.
	httputil.JSON(w, http.StatusCreated, createProjectResponse{
		Project:  p,
		APIKey:   rawKey,
		AdminKey: rawAdminKey,
	})
}
