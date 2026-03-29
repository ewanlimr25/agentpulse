package authutil_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/authutil"
)

// ---------------------------------------------------------------------------
// ExtractBearer
// ---------------------------------------------------------------------------

func TestExtractBearer_NoHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	token, ok := authutil.ExtractBearer(req)
	if ok {
		t.Fatalf("expected ok=false when no Authorization header, got token=%q", token)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestExtractBearer_EmptyHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "")
	token, ok := authutil.ExtractBearer(req)
	if ok {
		t.Fatalf("expected ok=false for empty Authorization header, got token=%q", token)
	}
}

func TestExtractBearer_WrongScheme(t *testing.T) {
	cases := []string{
		"Token abc123",
		"Basic dXNlcjpwYXNz",
		"bearer abc123", // lowercase — must match prefix "Bearer " exactly
		"BEARER abc123",
	}
	for _, auth := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", auth)
		_, ok := authutil.ExtractBearer(req)
		if ok {
			t.Errorf("expected ok=false for Authorization=%q", auth)
		}
	}
}

func TestExtractBearer_BearerPrefixButEmptyToken(t *testing.T) {
	for _, auth := range []string{"Bearer ", "Bearer   "} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", auth)
		token, ok := authutil.ExtractBearer(req)
		if ok {
			t.Errorf("expected ok=false for Authorization=%q, got token=%q", auth, token)
		}
	}
}

func TestExtractBearer_ValidToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-secret-key")
	token, ok := authutil.ExtractBearer(req)
	if !ok {
		t.Fatal("expected ok=true for valid Bearer header")
	}
	if token != "my-secret-key" {
		t.Errorf("expected token='my-secret-key', got %q", token)
	}
}

func TestExtractBearer_TokenWithLeadingSpace(t *testing.T) {
	// The implementation trims whitespace from the token.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer  padded-token")
	token, ok := authutil.ExtractBearer(req)
	if !ok {
		t.Fatal("expected ok=true when extra whitespace trimmed")
	}
	if token != "padded-token" {
		t.Errorf("expected trimmed token='padded-token', got %q", token)
	}
}

func TestExtractBearer_TokenWithSpecialChars(t *testing.T) {
	special := "tok_abc123-XYZ.def/ghi"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+special)
	token, ok := authutil.ExtractBearer(req)
	if !ok {
		t.Fatal("expected ok=true for token with special chars")
	}
	if token != special {
		t.Errorf("expected token=%q, got %q", special, token)
	}
}

// ---------------------------------------------------------------------------
// HashToken
// ---------------------------------------------------------------------------

func TestHashToken_KnownVector(t *testing.T) {
	// Independently compute SHA-256 to verify the output format and value.
	input := "test-token"
	sum := sha256.Sum256([]byte(input))
	expected := hex.EncodeToString(sum[:])

	got := authutil.HashToken(input)
	if got != expected {
		t.Errorf("HashToken(%q): expected %q, got %q", input, expected, got)
	}
}

func TestHashToken_OutputIsLowercaseHex(t *testing.T) {
	hash := authutil.HashToken("sometoken")
	for i, ch := range hash {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Errorf("HashToken output contains non-lowercase-hex char %q at position %d", ch, i)
		}
	}
}

func TestHashToken_OutputLength(t *testing.T) {
	hash := authutil.HashToken("any-value")
	// SHA-256 is 32 bytes → 64 hex chars.
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex digest, got length %d: %q", len(hash), hash)
	}
}

func TestHashToken_DifferentInputsProduceDifferentHashes(t *testing.T) {
	h1 := authutil.HashToken("token-a")
	h2 := authutil.HashToken("token-b")
	if h1 == h2 {
		t.Error("different tokens must produce different hashes")
	}
}

func TestHashToken_SameInputProducesSameHash(t *testing.T) {
	if authutil.HashToken("stable") != authutil.HashToken("stable") {
		t.Error("same input must produce identical hash")
	}
}

func TestHashToken_EmptyString(t *testing.T) {
	// Should not panic; empty string has a valid SHA-256 digest.
	hash := authutil.HashToken("")
	if len(hash) != 64 {
		t.Errorf("empty string hash should still be 64 hex chars, got %d", len(hash))
	}
}

func TestHashToken_UnicodeInput(t *testing.T) {
	// Verify no panic and consistent output for multi-byte characters.
	hash := authutil.HashToken("密码🔑")
	if len(hash) != 64 {
		t.Errorf("unicode token hash should be 64 hex chars, got %d", len(hash))
	}
}

// ---------------------------------------------------------------------------
// Integration: ExtractBearer + HashToken round-trip
// ---------------------------------------------------------------------------

func TestExtractBearerAndHashToken_RoundTrip(t *testing.T) {
	rawToken := "integration-test-key"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)

	extracted, ok := authutil.ExtractBearer(req)
	if !ok {
		t.Fatal("expected to extract bearer token")
	}

	sum := sha256.Sum256([]byte(rawToken))
	expectedHash := hex.EncodeToString(sum[:])
	gotHash := authutil.HashToken(extracted)

	if gotHash != expectedHash {
		t.Errorf("hash mismatch: expected %q, got %q", expectedHash, gotHash)
	}
}
