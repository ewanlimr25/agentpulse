package agentsemanticproc

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

const typeStr = "agentsemanticproc"

// NewFactory creates the agentsemanticproc processor factory.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		defaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func defaultConfig() component.Config {
	return &Config{
		AttributesConfigPath: "config/agent_attributes.yaml",
		PricingConfigPath:    "config/model_pricing.yaml",
	}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	c, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type %T", cfg)
	}

	attrReg, err := loadAttributeRegistry(c.AttributesConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading attribute registry: %w", err)
	}

	pricingReg, err := loadPricingRegistry(c.PricingConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading pricing registry: %w", err)
	}

	proc := newSemanticProcessor(set.Logger, attrReg, pricingReg)

	return processorhelper.NewTraces(
		ctx, set, cfg, next,
		proc.ProcessTraces,
		processorhelper.WithStart(proc.Start),
		processorhelper.WithShutdown(proc.Shutdown),
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}
