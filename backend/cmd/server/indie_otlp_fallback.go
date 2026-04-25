// When the binary is built without the `duckdb` tag, indie OTLP wiring is
// stubbed — runIndie will already have failed at IndieStores so this function
// is unreachable, but the build needs a definition.
//
//go:build !duckdb

package main

import (
	"context"
	"net/http"

	"github.com/agentpulse/agentpulse/backend/internal/bootstrap"
	"github.com/agentpulse/agentpulse/backend/internal/config"
)

func startIndieOTLP(ctx context.Context, cfg *config.Config, bundle *bootstrap.StoreBundle) (http.Handler, error) {
	return nil, bootstrap.ErrIndieDuckDBMissing
}
