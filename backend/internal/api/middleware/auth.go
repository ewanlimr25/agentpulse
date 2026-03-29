package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// RunAuth returns a middleware that authenticates run-scoped routes.
//
// Unlike BearerAuth (which checks {projectID} in the URL), run-scoped routes
// carry only a {runID}. This middleware resolves the run's project_id from
// ClickHouse, then validates the Bearer token belongs to that project.
//
// Returns 403 (not 404) for non-existent runs to prevent an existence oracle
// attack via timing differences.
func RunAuth(projects store.ProjectStore, runs store.RunStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearer(r)
			if !ok {
				httputil.Error(w, http.StatusUnauthorized, "missing or malformed Authorization header")
				return
			}

			hash := hashToken(token)
			project, err := projects.GetByAPIKeyHash(r.Context(), hash)
			if err != nil {
				httputil.Error(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			runID := chi.URLParam(r, "runID")
			runProjectID, err := runs.GetProjectID(r.Context(), runID)
			if err != nil {
				// Return 403 rather than 404 to avoid leaking run existence.
				httputil.Error(w, http.StatusForbidden, "access denied")
				return
			}

			if project.ID != runProjectID {
				httputil.Error(w, http.StatusForbidden, "API key does not belong to this run's project")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithProject(r.Context(), project)))
		})
	}
}

// BearerAuth returns a middleware that enforces Bearer token authentication.
//
// On every request it:
//  1. Extracts the token from "Authorization: Bearer <token>"
//  2. SHA-256 hashes it and looks up the project in the store
//  3. If the route URL contains {projectID}, verifies the token belongs to
//     that project (prevents using project-A's key to access project-B's data)
//  4. Stores the resolved project in the request context via WithProject
//
// Returns 401 for missing/invalid tokens, 403 for project ID mismatch.
func BearerAuth(projects store.ProjectStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearer(r)
			if !ok {
				httputil.Error(w, http.StatusUnauthorized, "missing or malformed Authorization header")
				return
			}

			hash := hashToken(token)
			project, err := projects.GetByAPIKeyHash(r.Context(), hash)
			if err != nil {
				httputil.Error(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			// If the route has a {projectID} parameter, verify the token
			// belongs to that specific project. This prevents cross-project access.
			if urlProjectID := chi.URLParam(r, "projectID"); urlProjectID != "" {
				if project.ID != urlProjectID {
					httputil.Error(w, http.StatusForbidden, "API key does not belong to this project")
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(WithProject(r.Context(), project)))
		})
	}
}

// extractBearer pulls the raw token from the Authorization header.
func extractBearer(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", false
	}
	token := strings.TrimSpace(auth[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// hashToken returns the lowercase hex-encoded SHA-256 of the token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
