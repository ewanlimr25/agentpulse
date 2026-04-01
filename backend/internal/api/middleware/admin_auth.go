package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/authutil"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// AdminKeyAuth validates the X-Admin-Key header (SHA-256 hash lookup).
// Used for settings mutations — separate from the SDK Bearer token.
//
// Returns 401 if the header is missing, 403 if the key is invalid or
// does not belong to the project identified by {projectID} in the URL.
func AdminKeyAuth(projects store.ProjectStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := r.Header.Get("X-Admin-Key")
			if rawKey == "" {
				httputil.Error(w, http.StatusUnauthorized, "missing X-Admin-Key header")
				return
			}

			hash := authutil.HashToken(rawKey)
			project, err := projects.GetByAdminKeyHash(r.Context(), hash)
			if err != nil {
				httputil.Error(w, http.StatusForbidden, "invalid admin key")
				return
			}

			// Verify the admin key belongs to the project in the URL.
			if urlProjectID := chi.URLParam(r, "projectID"); urlProjectID != "" {
				if project.ID != urlProjectID {
					httputil.Error(w, http.StatusForbidden, "admin key does not belong to this project")
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(WithProject(r.Context(), project)))
		})
	}
}
