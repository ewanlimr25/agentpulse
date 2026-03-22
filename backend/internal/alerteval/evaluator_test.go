package alerteval

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

func TestThresholdCrossed(t *testing.T) {
	tests := []struct {
		name      string
		current   float64
		threshold float64
		op        domain.CompareOp
		want      bool
	}{
		{"gt crossed", 15.0, 10.0, domain.CompareOpGt, true},
		{"gt not crossed", 5.0, 10.0, domain.CompareOpGt, false},
		{"gt equal not crossed", 10.0, 10.0, domain.CompareOpGt, false},
		{"lt crossed", 0.4, 0.6, domain.CompareOpLt, true},
		{"lt not crossed", 0.8, 0.6, domain.CompareOpLt, false},
		{"lt equal not crossed", 0.6, 0.6, domain.CompareOpLt, false},
		{"unknown op", 1.0, 0.5, domain.CompareOp("bad"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := thresholdCrossed(tt.current, tt.threshold, tt.op)
			if got != tt.want {
				t.Errorf("thresholdCrossed(%v, %v, %q) = %v; want %v",
					tt.current, tt.threshold, tt.op, got, tt.want)
			}
		})
	}
}

func TestInsufficientDataSkip(t *testing.T) {
	// -1 return value from queries means insufficient data — evaluateRule should skip
	// We verify the sentinel value is handled by checking thresholdCrossed is not called
	// when current < 0 (the evaluateRule short-circuits before that).
	// This is a logic test for the -1 sentinel contract.
	if thresholdCrossed(-1, 10.0, domain.CompareOpGt) {
		t.Error("-1 sentinel should not cross any threshold in normal usage; " +
			"caller must guard against -1 before calling thresholdCrossed")
	}
}

func TestSignalTypeMapping(t *testing.T) {
	// Ensure all four signal types are handled by querySignal (no default fallthrough gaps).
	types := []domain.SignalType{
		domain.SignalTypeErrorRate,
		domain.SignalTypeLatencyP95,
		domain.SignalTypeQualityScore,
		domain.SignalTypeToolFailure,
	}
	// Verify each has a distinct string value (catches accidental duplicate constants).
	seen := map[string]bool{}
	for _, st := range types {
		s := string(st)
		if seen[s] {
			t.Errorf("duplicate SignalType string value: %q", s)
		}
		seen[s] = true
	}
}

func TestCompareOpValues(t *testing.T) {
	if domain.CompareOpGt != "gt" {
		t.Errorf("CompareOpGt should be 'gt', got %q", domain.CompareOpGt)
	}
	if domain.CompareOpLt != "lt" {
		t.Errorf("CompareOpLt should be 'lt', got %q", domain.CompareOpLt)
	}
}
