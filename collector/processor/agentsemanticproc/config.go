package agentsemanticproc

// Config holds configuration for the agentsemanticproc processor.
type Config struct {
	// AttributesConfigPath is the path to agent_attributes.yaml.
	AttributesConfigPath string `mapstructure:"attributes_config"`
	// PricingConfigPath is the path to model_pricing.yaml.
	PricingConfigPath string `mapstructure:"pricing_config"`
}
