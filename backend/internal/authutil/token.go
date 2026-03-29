// Package authutil provides shared Bearer token helpers used by HTTP middleware
// and the WebSocket hub. Centralising these avoids duplicating crypto logic.
package authutil

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// ExtractBearer pulls the raw token from the Authorization header.
// Returns the token and true if present and well-formed, ("", false) otherwise.
func ExtractBearer(r *http.Request) (string, bool) {
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

// HashToken returns the lowercase hex-encoded SHA-256 of the token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
