package authenforceproc

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

const typeStr = "authenforceproc"

// NewFactory creates the authenforceproc processor factory.
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
		return nil, fmt.Errorf("authenforceproc: invalid config type %T", cfg)
	}

	proc, err := newAuthEnforceProcessor(ctx, set.Logger, c)
	if err != nil {
		return nil, err
	}

	return processorhelper.NewTraces(
		ctx, set, cfg, next,
		proc.ProcessTraces,
		processorhelper.WithStart(proc.Start),
		processorhelper.WithShutdown(proc.Shutdown),
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}
