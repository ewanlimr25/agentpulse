package clickhouseexporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const typeStr = "clickhouseexporter"

// NewFactory creates the clickhouseexporter factory.
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

	ins, err := newClickhouseInserter(c)
	if err != nil {
		return nil, fmt.Errorf("creating clickhouse inserter: %w", err)
	}

	exp := newTracesExporter(c, set.Logger, ins)

	return exporterhelper.NewTraces(
		ctx, set, cfg,
		exp.ConsumeTraces,
		exporterhelper.WithStart(exp.Start),
		exporterhelper.WithShutdown(exp.Shutdown),
	)
}
