// Package pricing loads model pricing data and computes token costs.
package pricing

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Table holds model pricing data loaded from config/model_pricing.yaml.
type Table struct {
	Models   map[string]Model
	Fallback Model
}

// Model holds per-model pricing and provider metadata.
type Model struct {
	Provider         string
	InputPerMillion  float64
	OutputPerMillion float64
}

// ModelInfo is returned by the /api/v1/models endpoint.
type ModelInfo struct {
	ModelID          string  `json:"model_id"`
	Provider         string  `json:"provider"`
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
}

// yamlFile mirrors the YAML structure for unmarshalling.
type yamlFile struct {
	Models   map[string]yamlModel `yaml:"models"`
	Fallback yamlModel            `yaml:"fallback"`
}

type yamlModel struct {
	Provider         string  `yaml:"provider"`
	InputPerMillion  float64 `yaml:"input_per_million"`
	OutputPerMillion float64 `yaml:"output_per_million"`
}

// Load reads model_pricing.yaml from the given path.
func Load(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pricing: read file %s: %w", path, err)
	}

	var raw yamlFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("pricing: parse yaml: %w", err)
	}

	models := make(map[string]Model, len(raw.Models))
	for id, m := range raw.Models {
		models[id] = Model{
			Provider:         m.Provider,
			InputPerMillion:  m.InputPerMillion,
			OutputPerMillion: m.OutputPerMillion,
		}
	}

	return &Table{
		Models: models,
		Fallback: Model{
			InputPerMillion:  raw.Fallback.InputPerMillion,
			OutputPerMillion: raw.Fallback.OutputPerMillion,
		},
	}, nil
}

// Cost computes the cost in USD for the given model and token counts.
// Uses fallback pricing if the model is not in the table.
func (t *Table) Cost(modelID string, inputTokens, outputTokens int) float64 {
	m, ok := t.Models[modelID]
	if !ok {
		m = t.Fallback
	}
	input := float64(inputTokens) * m.InputPerMillion / 1_000_000
	output := float64(outputTokens) * m.OutputPerMillion / 1_000_000
	return input + output
}

// ModelList returns all registered models as ModelInfo slices, sorted by model ID.
func (t *Table) ModelList() []ModelInfo {
	list := make([]ModelInfo, 0, len(t.Models))
	for id, m := range t.Models {
		list = append(list, ModelInfo{
			ModelID:          id,
			Provider:         m.Provider,
			InputPerMillion:  m.InputPerMillion,
			OutputPerMillion: m.OutputPerMillion,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ModelID < list[j].ModelID
	})
	return list
}
