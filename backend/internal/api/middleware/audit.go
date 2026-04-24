package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/audit"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Audit records one audit event per request into ClickHouse asynchronously.
func Audit(w *audit.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			tokenHash := ""
			if token != "" {
				sum := sha256.Sum256([]byte(token))
				tokenHash = hex.EncodeToString(sum[:])
			}

			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.SplitN(fwd, ",", 2)[0]
			}

			outcome := "success"
			if rec.status >= 400 {
				outcome = "error"
			}

			w.Record(audit.Event{
				Timestamp:  time.Now().UTC(),
				TokenHash:  tokenHash,
				IP:         strings.TrimSpace(ip),
				Method:     r.Method,
				Endpoint:   r.URL.Path,
				StatusCode: rec.status,
				Outcome:    outcome,
			})
		})
	}
}
