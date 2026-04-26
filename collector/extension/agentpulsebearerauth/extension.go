// Package agentpulsebearerauth implements an OTel collector auth extension
// that validates `Authorization: Bearer <token>` headers on inbound OTLP
// requests against AgentPulse's `project_ingest_tokens` Postgres table.
//
// Compared to the legacy authenforceproc processor (which reads
// `agentpulse.ingest_token` from span attributes), this extension enforces
// auth at the receiver boundary and keeps the secret out of span data.
package agentpulsebearerauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Context-key type so callers can fish the resolved project ID out without
// colliding with other extensions. Stored as the value of authContextKey{}.
type authContextKey struct{}

// AuthInfo is what the extension stashes in the request context after a
// successful auth check.
type AuthInfo struct {
	ProjectID string
	TokenID   string
}

// FromContext extracts AuthInfo set by Authenticate, if any.
func FromContext(ctx context.Context) (*AuthInfo, bool) {
	v, ok := ctx.Value(authContextKey{}).(*AuthInfo)
	return v, ok
}

// bearerAuth is the extension implementation. It satisfies both
// extension.Extension (component.Component) and extensionauth.Server.
type bearerAuth struct {
	cfg    *Config
	logger *zap.Logger
	pool   *pgxpool.Pool

	cacheMu sync.RWMutex
	cache   map[string]cacheEntry
}

type cacheEntry struct {
	info      AuthInfo
	expiresAt time.Time
}

func newExtension(cfg *Config, logger *zap.Logger) *bearerAuth {
	return &bearerAuth{
		cfg:    cfg,
		logger: logger,
		cache:  map[string]cacheEntry{},
	}
}

// Start opens the Postgres pool. Done lazily to avoid blocking collector start
// when DSN is unreachable; first request will retry.
func (b *bearerAuth) Start(ctx context.Context, _ component.Host) error {
	pool, err := pgxpool.New(ctx, b.cfg.DSN)
	if err != nil {
		if b.cfg.Required {
			return fmt.Errorf("agentpulsebearerauth: pgxpool.New: %w", err)
		}
		b.logger.Warn("agentpulsebearerauth: postgres unreachable; running warn-only", zap.Error(err))
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		if b.cfg.Required {
			pool.Close()
			return fmt.Errorf("agentpulsebearerauth: ping: %w", err)
		}
		b.logger.Warn("agentpulsebearerauth: postgres ping failed; running warn-only", zap.Error(err))
	}
	b.pool = pool
	return nil
}

func (b *bearerAuth) Shutdown(_ context.Context) error {
	if b.pool != nil {
		b.pool.Close()
	}
	return nil
}

// Authenticate satisfies extensionauth.Server.
//
// `sources` contains the full inbound metadata map. The OTLP/HTTP receiver
// passes HTTP headers (lowercased keys); the OTLP/gRPC receiver passes gRPC
// metadata. In both cases the convention is `authorization`.
func (b *bearerAuth) Authenticate(ctx context.Context, sources map[string][]string) (context.Context, error) {
	token := extractBearer(sources)
	if token == "" {
		return b.deny(ctx, errors.New("missing or malformed Authorization header"))
	}

	hash := hashToken(token)

	if cached, ok := b.lookupCache(hash); ok {
		return contextWithAuth(ctx, cached), nil
	}

	if b.pool == nil {
		// Postgres still unavailable.
		return b.deny(ctx, errors.New("auth backend unavailable"))
	}

	row := b.pool.QueryRow(ctx, `
		SELECT id, project_id
		FROM project_ingest_tokens
		WHERE token_hash = $1
	`, hash)

	info := AuthInfo{}
	if err := row.Scan(&info.TokenID, &info.ProjectID); err != nil {
		return b.deny(ctx, fmt.Errorf("invalid token: %w", err))
	}

	b.storeCache(hash, info)
	return contextWithAuth(ctx, info), nil
}

func (b *bearerAuth) deny(ctx context.Context, err error) (context.Context, error) {
	if !b.cfg.Required {
		// Warn-mode: log the rejection but pass through. Lets operators audit
		// would-have-rejected traffic before flipping `required: true`.
		b.logger.Warn("agentpulsebearerauth: warn-mode reject (allowing)", zap.Error(err))
		return ctx, nil
	}
	return ctx, err
}

func (b *bearerAuth) lookupCache(hash string) (AuthInfo, bool) {
	if b.cfg.CacheTTL <= 0 {
		return AuthInfo{}, false
	}
	b.cacheMu.RLock()
	entry, ok := b.cache[hash]
	b.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return AuthInfo{}, false
	}
	return entry.info, true
}

func (b *bearerAuth) storeCache(hash string, info AuthInfo) {
	if b.cfg.CacheTTL <= 0 {
		return
	}
	b.cacheMu.Lock()
	b.cache[hash] = cacheEntry{info: info, expiresAt: time.Now().Add(b.cfg.CacheTTL)}
	b.cacheMu.Unlock()
}

// extractBearer pulls the raw token out of the standard `authorization` source.
// Header keys arriving from collector receivers are lowercased.
func extractBearer(sources map[string][]string) string {
	candidates := append([]string{}, sources["authorization"]...)
	candidates = append(candidates, sources["Authorization"]...)
	for _, v := range candidates {
		v = strings.TrimSpace(v)
		if !strings.HasPrefix(v, "Bearer ") && !strings.HasPrefix(v, "bearer ") {
			continue
		}
		tok := strings.TrimSpace(v[len("Bearer "):])
		if tok != "" {
			return tok
		}
	}
	return ""
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func contextWithAuth(ctx context.Context, info AuthInfo) context.Context {
	cp := info
	return context.WithValue(ctx, authContextKey{}, &cp)
}

// Compile-time assertions that the extension implements the required interfaces.
var (
	_ component.Component  = (*bearerAuth)(nil)
	_ extensionauth.Server = (*bearerAuth)(nil)
)
