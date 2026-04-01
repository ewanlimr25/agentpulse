package piimaskerproc

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.uber.org/zap"
)

const typeStr = "piimaskerproc"

// NewFactory creates the piimaskerproc processor factory.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		func() component.Config { return defaultConfig() },
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	c, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("piimaskerproc: invalid config type %T", cfg)
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}

	store, err := newPIISettingsStore(ctx, set.Logger, c.PostgresDSN, c.MaxPoolConns)
	if err != nil {
		// Log but do not fail hard — processor will run in fail-closed mode.
		set.Logger.Warn("piimaskerproc: could not connect to Postgres at startup, will retry",
			zap.Error(err),
		)
		// Create a nil-store processor; Start() will handle it safely.
		// Actually, we need a valid store, so we must return error here and
		// let the operator know. The fail-closed logic in Start handles transient
		// failures, not startup failures where no store exists at all.
		return nil, fmt.Errorf("piimaskerproc: connect to postgres: %w", err)
	}

	proc := newPIIMaskerProcessor(set.Logger, c, store)

	return processorhelper.NewTraces(
		ctx, set, cfg, next,
		proc.ProcessTraces,
		processorhelper.WithStart(proc.Start),
		processorhelper.WithShutdown(proc.Shutdown),
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}
