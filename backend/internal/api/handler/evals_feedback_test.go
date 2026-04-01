package handler

import (
	"testing"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// applyFeedbackOverrides unit tests
// ---------------------------------------------------------------------------

// helper: build a simple []EvalTypeBaseline from (name, score) pairs.
func makeTypes(pairs ...interface{}) []domain.EvalTypeBaseline {
	out := make([]domain.EvalTypeBaseline, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		name := pairs[i].(string)
		score := float32(pairs[i+1].(float64))
		out = append(out, domain.EvalTypeBaseline{
			EvalName:  name,
			AvgScore:  score,
			SpanCount: 5,
			RunCount:  3,
		})
	}
	return out
}

// TestApplyFeedbackOverrides_Empty — with no feedback the original types slice is
// returned unchanged (same pointer, not a defensive copy).
func TestApplyFeedbackOverrides_Empty(t *testing.T) {
	types := makeTypes("relevance", 0.7, "faithfulness", 0.6)
	result := applyFeedbackOverrides(types, map[string]string{})

	if len(result) != len(types) {
		t.Fatalf("expected %d types, got %d", len(types), len(result))
	}
	for i, r := range result {
		if r.AvgScore != types[i].AvgScore {
			t.Errorf("types[%d] score changed: expected %v, got %v", i, types[i].AvgScore, r.AvgScore)
		}
	}
}

// TestApplyFeedbackOverrides_AllBad — when every feedback record is "bad",
// the adjusted score should approach 0 (bad fraction = 1.0, so adjusted = score * 0 = 0.0).
func TestApplyFeedbackOverrides_AllBad(t *testing.T) {
	types := makeTypes("relevance", 0.9)
	feedback := map[string]string{
		"span-1": "bad",
		"span-2": "bad",
		"span-3": "bad",
	}
	result := applyFeedbackOverrides(types, feedback)

	if len(result) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result))
	}
	// With badFraction = 1.0: adjusted = score*(1-1.0) + 0*1.0 = 0.0
	// goodFraction = 0.0, so floor contribution = 0.
	if result[0].AvgScore != 0.0 {
		t.Errorf("all-bad feedback: expected score 0.0, got %v", result[0].AvgScore)
	}
}

// TestApplyFeedbackOverrides_AllGood — when every feedback record is "good",
// the floor contribution (0.8 * goodFraction) should apply if the judge score is low.
func TestApplyFeedbackOverrides_AllGood(t *testing.T) {
	// Judge score 0.5 — below 0.8 floor.
	types := makeTypes("relevance", 0.5)
	feedback := map[string]string{
		"span-1": "good",
		"span-2": "good",
	}
	result := applyFeedbackOverrides(types, feedback)

	if len(result) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result))
	}
	// goodFraction = 1.0, badFraction = 0.0
	// adjusted = 0.5 * (1 - 0) = 0.5
	// floorContribution = 0.8 * 1.0 = 0.8
	// 0.5 < 0.8 → adjusted = 0.8
	if result[0].AvgScore != 0.8 {
		t.Errorf("all-good feedback with low judge score: expected 0.8, got %v", result[0].AvgScore)
	}
}

// TestApplyFeedbackOverrides_MixedGoodBad — half good, half bad; the logic
// should blend the two effects proportionally.
func TestApplyFeedbackOverrides_MixedGoodBad(t *testing.T) {
	types := makeTypes("relevance", 0.6)
	feedback := map[string]string{
		"span-1": "good",
		"span-2": "bad",
	}
	result := applyFeedbackOverrides(types, feedback)

	if len(result) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result))
	}
	// badFraction = 0.5, goodFraction = 0.5
	// adjusted = 0.6 * (1 - 0.5) = 0.3
	// floorContribution = 0.8 * 0.5 = 0.4
	// 0.3 < 0.4 → adjusted = 0.4
	const expected float32 = 0.4
	if result[0].AvgScore != expected {
		t.Errorf("mixed feedback: expected %v, got %v", expected, result[0].AvgScore)
	}
}

// TestApplyFeedbackOverrides_GoodNoOp — when the judge score is already high
// (0.9), all-good feedback should not decrease it.
func TestApplyFeedbackOverrides_GoodNoOp(t *testing.T) {
	types := makeTypes("relevance", 0.9)
	feedback := map[string]string{
		"span-1": "good",
		"span-2": "good",
	}
	result := applyFeedbackOverrides(types, feedback)

	if len(result) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result))
	}
	// goodFraction = 1.0, badFraction = 0.0
	// adjusted = 0.9 * 1.0 = 0.9
	// floorContribution = 0.8 * 1.0 = 0.8
	// 0.9 >= 0.8 → no floor applied; result should stay at 0.9.
	if result[0].AvgScore != 0.9 {
		t.Errorf("high judge score with good feedback: expected 0.9 (no decrease), got %v", result[0].AvgScore)
	}
}

// TestApplyFeedbackOverrides_ImmutableInput — the function must not mutate the
// original types slice; callers rely on their copy being unchanged.
func TestApplyFeedbackOverrides_ImmutableInput(t *testing.T) {
	types := makeTypes("relevance", 0.7, "faithfulness", 0.8)
	origScores := make([]float32, len(types))
	for i, tp := range types {
		origScores[i] = tp.AvgScore
	}

	feedback := map[string]string{
		"span-1": "bad",
		"span-2": "bad",
	}
	result := applyFeedbackOverrides(types, feedback)

	// Mutate the result to prove it is a separate allocation.
	for i := range result {
		result[i].AvgScore = 99.0
	}

	for i, tp := range types {
		if tp.AvgScore != origScores[i] {
			t.Errorf("input types[%d] was mutated: expected %v, got %v", i, origScores[i], tp.AvgScore)
		}
	}
}

// TestApplyFeedbackOverrides_MetadataPreserved — EvalName, SpanCount, and RunCount
// on the output records should match the originals exactly.
func TestApplyFeedbackOverrides_MetadataPreserved(t *testing.T) {
	input := []domain.EvalTypeBaseline{
		{EvalName: "hallucination", AvgScore: 0.5, SpanCount: 12, RunCount: 7},
	}
	feedback := map[string]string{"span-1": "good"}
	result := applyFeedbackOverrides(input, feedback)

	if result[0].EvalName != "hallucination" {
		t.Errorf("EvalName changed: got %q", result[0].EvalName)
	}
	if result[0].SpanCount != 12 {
		t.Errorf("SpanCount changed: expected 12, got %d", result[0].SpanCount)
	}
	if result[0].RunCount != 7 {
		t.Errorf("RunCount changed: expected 7, got %d", result[0].RunCount)
	}
}

// TestApplyFeedbackOverrides_UnknownRatingsIgnored — ratings that are neither
// "good" nor "bad" should not affect the count or the result.
func TestApplyFeedbackOverrides_UnknownRatingsIgnored(t *testing.T) {
	types := makeTypes("relevance", 0.7)
	// Only unknown ratings — total should be 0, function returns unchanged types.
	feedback := map[string]string{
		"span-1": "ok",
		"span-2": "neutral",
	}
	result := applyFeedbackOverrides(types, feedback)

	if result[0].AvgScore != 0.7 {
		t.Errorf("unknown ratings should be no-op: expected 0.7, got %v", result[0].AvgScore)
	}
}

// TestApplyFeedbackOverrides_MultipleEvalTypes — all types in the slice are
// adjusted independently; a single feedback map applies the same good/bad
// fractions to all types.
func TestApplyFeedbackOverrides_MultipleEvalTypes(t *testing.T) {
	types := makeTypes("relevance", 0.8, "faithfulness", 0.6, "toxicity", 0.95)
	feedback := map[string]string{
		"span-1": "bad",
		"span-2": "bad",
	}
	result := applyFeedbackOverrides(types, feedback)

	if len(result) != 3 {
		t.Fatalf("expected 3 types, got %d", len(result))
	}
	// All-bad: every score should collapse to 0.
	for _, r := range result {
		if r.AvgScore != 0.0 {
			t.Errorf("%s: expected 0.0 with all-bad feedback, got %v", r.EvalName, r.AvgScore)
		}
	}
}

// TestApplyFeedbackOverrides_EmptyTypesSlice — gracefully handles no eval types.
func TestApplyFeedbackOverrides_EmptyTypesSlice(t *testing.T) {
	result := applyFeedbackOverrides([]domain.EvalTypeBaseline{}, map[string]string{"span-1": "good"})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}
