package topologyexporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const typeStr = "topologyexporter"

// NewFactory creates the topologyexporter factory.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		component.MustNewType(typeStr),
		func() component.Config { return defaultConfig() },
		exporter.WithTraces(createTracesExporter, component.StabilityLevelDevelopment),
	)
}

func createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Traces, error) {
	c, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type %T", cfg)
	}

	store, err := newPostgresStore(ctx, c.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("creating topology store: %w", err)
	}

	exp := newTopologyExporter(c, set.Logger, store)

	return exporterhelper.NewTraces(
		ctx, set, cfg,
		exp.ConsumeTraces,
		exporterhelper.WithStart(exp.Start),
		exporterhelper.WithShutdown(exp.Shutdown),
	)
}
