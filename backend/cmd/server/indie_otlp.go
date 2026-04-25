// Indie-mode OTLP receiver wiring. The receiver depends on duckdb.SpanStore as
// the writer interface, so this file is gated on the same build tag.
//
//go:build duckdb

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/agentpulse/agentpulse/backend/internal/bootstrap"
	"github.com/agentpulse/agentpulse/backend/internal/config"
	"github.com/agentpulse/agentpulse/backend/internal/otlp"
	"github.com/agentpulse/agentpulse/backend/internal/store/duckdb"
)

// startIndieOTLP returns the http.Handler for the embedded OTLP/HTTP receiver.
// Returns an error if the bundle's SpanStore isn't a DuckDB writer (which
// shouldn't happen in indie mode but is checked defensively).
func startIndieOTLP(ctx context.Context, cfg *config.Config, bundle *bootstrap.StoreBundle) (http.Handler, error) {
	writer, ok := bundle.Spans.(*duckdb.SpanStore)
	if !ok {
		return nil, errors.New("indie: span store is not a DuckDB writer — bundle wiring is wrong")
	}
	rec := otlp.NewReceiver(bundle.IngestTokens, writer, slog.Default())
	return rec.Handler(), nil
}
