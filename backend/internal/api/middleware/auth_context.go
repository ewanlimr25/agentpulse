package middleware

import (
	"context"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

type contextKey int

const projectKey contextKey = iota

// WithProject stores the authenticated project in the request context.
func WithProject(ctx context.Context, p *domain.Project) context.Context {
	return context.WithValue(ctx, projectKey, p)
}

// ProjectFromContext retrieves the authenticated project from the context.
// Returns (nil, false) if not present (i.e. the route is unauthenticated).
func ProjectFromContext(ctx context.Context) (*domain.Project, bool) {
	p, ok := ctx.Value(projectKey).(*domain.Project)
	return p, ok && p != nil
}
