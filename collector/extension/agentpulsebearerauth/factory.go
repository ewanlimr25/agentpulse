package agentpulsebearerauth

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

const typeStr = "agentpulsebearerauth"

// NewFactory returns the factory used to register this extension with the
// collector. Reference it from cmd/collector/components.go.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		createExtension,
		component.StabilityLevelAlpha,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Required: false,
		CacheTTL: 60 * time.Second,
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newExtension(cfg.(*Config), set.Logger), nil
}
