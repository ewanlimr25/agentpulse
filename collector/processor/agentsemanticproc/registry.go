package agentsemanticproc

import (
	"os"

	"gopkg.in/yaml.v3"
)

// attributeRegistry holds the loaded agent_attributes.yaml configuration.
type attributeRegistry struct {
	SpanKindDetection []spanKindRule        `yaml:"span_kind_detection"`
	FieldExtraction   map[string][]string   `yaml:"field_extraction"`
	Cost              costRegistryConfig    `yaml:"cost"`
}

type spanKindRule struct {
	Attribute   string            `yaml:"attribute"`
	ValueMap    map[string]string `yaml:"value_map"`
	PrefixMap   map[string]string `yaml:"prefix_map"`
	Passthrough bool              `yaml:"passthrough"`
}

type costRegistryConfig struct {
	ExplicitAttribute string `yaml:"explicit_attribute"`
	PricingConfig     string `yaml:"pricing_config"`
}

// pricingRegistry holds the loaded model_pricing.yaml configuration.
type pricingRegistry struct {
	Models   map[string]modelPrice `yaml:"models"`
	Fallback modelPrice            `yaml:"fallback"`
}

type modelPrice struct {
	InputPerMillion  float64 `yaml:"input_per_million"`
	OutputPerMillion float64 `yaml:"output_per_million"`
}

func loadAttributeRegistry(path string) (*attributeRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reg attributeRegistry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

func loadPricingRegistry(path string) (*pricingRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reg pricingRegistry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

// costUSD computes the estimated cost for a span given token counts and model ID.
func (p *pricingRegistry) costUSD(modelID string, inputTokens, outputTokens uint32) float64 {
	price, ok := p.Models[modelID]
	if !ok {
		price = p.Fallback
	}
	return (float64(inputTokens)/1_000_000)*price.InputPerMillion +
		(float64(outputTokens)/1_000_000)*price.OutputPerMillion
}
