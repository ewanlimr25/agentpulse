package pricing

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

const testYAML = `
models:
  claude-sonnet-4-6:
    provider: anthropic
    input_per_million: 3.00
    output_per_million: 15.00
  gpt-4o:
    provider: openai
    input_per_million: 2.50
    output_per_million: 10.00
fallback:
  input_per_million: 1.00
  output_per_million: 5.00
`

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "model_pricing.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		path := writeFixture(t, testYAML)
		tbl, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if len(tbl.Models) != 2 {
			t.Errorf("expected 2 models, got %d", len(tbl.Models))
		}
		m, ok := tbl.Models["claude-sonnet-4-6"]
		if !ok {
			t.Fatal("missing model claude-sonnet-4-6")
		}
		if m.Provider != "anthropic" {
			t.Errorf("provider = %q, want %q", m.Provider, "anthropic")
		}
		if m.InputPerMillion != 3.0 {
			t.Errorf("input_per_million = %f, want 3.0", m.InputPerMillion)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := Load("/nonexistent/path.yaml")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := writeFixture(t, "not: [valid: yaml: {{")
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error for invalid yaml")
		}
	})
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestCost(t *testing.T) {
	path := writeFixture(t, testYAML)
	tbl, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		name         string
		modelID      string
		inputTokens  int
		outputTokens int
		want         float64
	}{
		{
			name:         "known model",
			modelID:      "gpt-4o",
			inputTokens:  1_000_000,
			outputTokens: 500_000,
			want:         2.50 + 5.00, // 2.50 input + 10.00*0.5 output
		},
		{
			name:         "unknown model uses fallback",
			modelID:      "unknown-model",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			want:         1.00 + 5.00,
		},
		{
			name:         "zero tokens",
			modelID:      "gpt-4o",
			inputTokens:  0,
			outputTokens: 0,
			want:         0.0,
		},
		{
			name:         "small token count",
			modelID:      "claude-sonnet-4-6",
			inputTokens:  1000,
			outputTokens: 500,
			want:         3.0*1000/1_000_000 + 15.0*500/1_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tbl.Cost(tt.modelID, tt.inputTokens, tt.outputTokens)
			if !almostEqual(got, tt.want) {
				t.Errorf("Cost(%q, %d, %d) = %f, want %f",
					tt.modelID, tt.inputTokens, tt.outputTokens, got, tt.want)
			}
		})
	}
}

func TestModelList(t *testing.T) {
	path := writeFixture(t, testYAML)
	tbl, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	list := tbl.ModelList()
	if len(list) != 2 {
		t.Fatalf("expected 2 models, got %d", len(list))
	}

	// Verify sorted order: claude-sonnet-4-6 < gpt-4o
	if list[0].ModelID != "claude-sonnet-4-6" {
		t.Errorf("list[0].ModelID = %q, want %q", list[0].ModelID, "claude-sonnet-4-6")
	}
	if list[1].ModelID != "gpt-4o" {
		t.Errorf("list[1].ModelID = %q, want %q", list[1].ModelID, "gpt-4o")
	}

	// Verify fields populated
	if list[0].Provider != "anthropic" {
		t.Errorf("list[0].Provider = %q, want %q", list[0].Provider, "anthropic")
	}
	if list[1].InputPerMillion != 2.50 {
		t.Errorf("list[1].InputPerMillion = %f, want 2.50", list[1].InputPerMillion)
	}
}
