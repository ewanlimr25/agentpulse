package middleware

import "net/http"

// NewCORS returns a CORS middleware configured by the provided allowlist and
// dev-mode flag.
//
//   - devMode && len(allowedOrigins) == 0 → wildcard (Access-Control-Allow-Origin: *)
//     Preserves the original permissive behaviour for local development.
//   - otherwise → origin allowlist. The request's Origin header is checked against
//     the allowedOrigins set. Matching origins are echoed back; non-matching
//     requests receive no ACAO header. Vary: Origin is always added so CDNs
//     cache responses per origin rather than serving a cached wildcard response.
func NewCORS(allowedOrigins []string, devMode bool) func(http.Handler) http.Handler {
	// Build a lookup map once at construction time for O(1) checks.
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}

	wildcard := devMode && len(allowedOrigins) == 0

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				origin := r.Header.Get("Origin")
				if _, ok := originSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				// Non-matching origins: no ACAO header — browser blocks the request.
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
