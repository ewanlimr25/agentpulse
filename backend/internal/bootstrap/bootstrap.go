// Package bootstrap wires together a complete StoreBundle for the chosen run
// mode. Team mode keeps Postgres + ClickHouse + S3; indie mode uses SQLite +
// DuckDB + local filesystem. Centralizing the wiring keeps cmd/server linear
// regardless of which path is selected.
package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// FirstRunResult describes the credentials minted on the first invocation of
// indie mode. Caller is responsible for printing the raw tokens and persisting
// nothing further — only hashes live in the database.
//
// In indie mode the raw token is reused as API key, admin key, and ingest
// token (single-tenant by design), so AdminAPIKey == IngestToken.
type FirstRunResult struct {
	ProjectID   string
	AdminAPIKey string // raw, printed once
	IngestToken string // raw, printed once (== AdminAPIKey in indie)
	DataDir     string
}

// EnsureIndieDataDir resolves the indie-mode data directory, creating it if
// missing. Defaults to ~/.agentpulse when the configured path is empty.
func EnsureIndieDataDir(configured string) (string, error) {
	dir := configured
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
		dir = filepath.Join(home, ".agentpulse")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// MaybeBootstrapFirstRun creates a default project with admin API key + an
// initial ingest token if no project exists yet. Returns nil, nil when the
// instance has already been bootstrapped.
func MaybeBootstrapFirstRun(
	ctx context.Context,
	projects store.ProjectStore,
	tokens store.IngestTokenStore,
	dataDir string,
	logger *slog.Logger,
) (*FirstRunResult, error) {
	existing, err := projects.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	if len(existing) > 0 {
		return nil, nil
	}

	// Indie ergonomics: one bearer token does all three jobs (API auth, admin
	// auth for settings mutations, OTLP ingest). Team mode keeps these distinct
	// because multi-user environments need separate trust boundaries.
	rawToken, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	p := &domain.Project{
		ID:           uuid.NewString(),
		Name:         "default",
		APIKeyHash:   sha256Hex(rawToken),
		AdminKeyHash: sha256Hex(rawToken),
	}
	if err := projects.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create default project: %w", err)
	}
	if _, err := tokens.Create(ctx, p.ID, sha256Hex(rawToken), "default"); err != nil {
		return nil, fmt.Errorf("create ingest token: %w", err)
	}
	rawAdmin := rawToken
	rawIngest := rawToken

	// Persist the raw admin key one-time as a hint for the user; ingest token
	// goes only to stdout (per recommendations.md design principle 3).
	apiKeyPath := filepath.Join(dataDir, "admin_api_key")
	if err := os.WriteFile(apiKeyPath, []byte(rawAdmin+"\n"), 0o600); err != nil {
		logger.Warn("could not persist admin key file", "path", apiKeyPath, "err", err)
	}

	return &FirstRunResult{
		ProjectID:   p.ID,
		AdminAPIKey: rawAdmin,
		IngestToken: rawIngest,
		DataDir:     dataDir,
	}, nil
}

// PrintFirstRun emits the once-only credentials to stderr in a banner format.
// Stdout is reserved for JSON logs.
func PrintFirstRun(r *FirstRunResult) {
	if r == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "──────────────────────────────────────────────────────────────────")
	fmt.Fprintln(os.Stderr, "  AgentPulse indie mode — first-run credentials")
	fmt.Fprintln(os.Stderr, "──────────────────────────────────────────────────────────────────")
	fmt.Fprintf(os.Stderr,  "  Project ID    : %s\n", r.ProjectID)
	fmt.Fprintf(os.Stderr,  "  Admin API key : %s\n", r.AdminAPIKey)
	fmt.Fprintf(os.Stderr,  "  Ingest token  : %s\n", r.IngestToken)
	fmt.Fprintf(os.Stderr,  "  Data dir      : %s\n", r.DataDir)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  Save these now — they will not be shown again.")
	fmt.Fprintln(os.Stderr, "  The ingest token is what your SDK should send as `Authorization: Bearer ...`")
	fmt.Fprintln(os.Stderr, "──────────────────────────────────────────────────────────────────")
	fmt.Fprintln(os.Stderr, "")
}

func generateToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ErrIndieDuckDBMissing is returned by IndieStores when --mode=indie is
// requested but the binary was built without the duckdb tag.
var ErrIndieDuckDBMissing = errors.New(
	"indie mode requires DuckDB support — rebuild with `go build -tags=duckdb` " +
		"or set AGENTPULSE_MODE=team",
)
