package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

type UserHandler struct {
	users store.UserStore
}

func NewUserHandler(users store.UserStore) *UserHandler {
	return &UserHandler{users: users}
}

func (h *UserHandler) Routes(r chi.Router) {
	r.Get("/", h.List)
}

// List returns paginated users for a project, ordered by total cost descending.
// Route: GET /api/v1/projects/{projectID}/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	limit := intQueryParam(r, "limit", 50)
	offset := intQueryParam(r, "offset", 0)

	users, err := h.users.List(r.Context(), projectID, limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	total, err := h.users.Count(r.Context(), projectID)
	if err != nil {
		total = 0 // non-fatal
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"users":  users,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
