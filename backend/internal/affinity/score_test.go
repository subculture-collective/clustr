package affinity_test

import (
	"math"
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/affinity"
)

func TestScoreIsSymmetricFiniteAndBounded(t *testing.T) {
	left := affinity.Evidence{
		ActiveUsers: affinity.Layer{Observed: 14, LeftMarginal: 80, RightMarginal: 40, Universe: 1000, Available: true},
		Authors:     affinity.Layer{Observed: 5, LeftMarginal: 20, RightMarginal: 10, Universe: 300, Available: true},
	}
	right := left
	right.ActiveUsers.LeftMarginal, right.ActiveUsers.RightMarginal = right.ActiveUsers.RightMarginal, right.ActiveUsers.LeftMarginal
	right.Authors.LeftMarginal, right.Authors.RightMarginal = right.Authors.RightMarginal, right.Authors.LeftMarginal

	a := affinity.Score(left, affinity.DefaultWeights())
	b := affinity.Score(right, affinity.DefaultWeights())
	if math.IsNaN(a.Affinity) || math.IsInf(a.Affinity, 0) || a.Affinity < 0 || a.Affinity > 1 {
		t.Fatalf("affinity must be finite and normalized, got %v", a.Affinity)
	}
	if math.Abs(a.Affinity-b.Affinity) > 1e-12 {
		t.Fatalf("score must be symmetric: %v != %v", a.Affinity, b.Affinity)
	}
}

func TestMissingLayerRenormalizesEffectiveWeights(t *testing.T) {
	evidence := affinity.Evidence{
		ActiveUsers: affinity.Layer{Observed: 8, LeftMarginal: 20, RightMarginal: 16, Universe: 200, Available: true},
		Authors:     affinity.Layer{Observed: 3, LeftMarginal: 8, RightMarginal: 6, Universe: 100, Available: true},
	}
	result := affinity.Score(evidence, affinity.DefaultWeights())
	if got := result.EffectiveWeights[affinity.ActiveUsers]; math.Abs(got-0.75) > 1e-12 {
		t.Fatalf("active-user weight = %v, want .75", got)
	}
	if got := result.EffectiveWeights[affinity.Authors]; math.Abs(got-0.25) > 1e-12 {
		t.Fatalf("author weight = %v, want .25", got)
	}
	if result.EffectiveWeights[affinity.CrossReferences] != 0 {
		t.Fatal("unavailable cross-reference layer must contribute zero")
	}
}

func TestLowSupportIsShrunkAndEvidenceGrowthIsMonotonic(t *testing.T) {
	weights := affinity.Weights{affinity.ActiveUsers: 1}
	one := affinity.Score(affinity.Evidence{ActiveUsers: affinity.Layer{Observed: 1, LeftMarginal: 20, RightMarginal: 20, Universe: 1000, Available: true}}, weights)
	two := affinity.Score(affinity.Evidence{ActiveUsers: affinity.Layer{Observed: 2, LeftMarginal: 20, RightMarginal: 20, Universe: 1000, Available: true}}, weights)
	five := affinity.Score(affinity.Evidence{ActiveUsers: affinity.Layer{Observed: 5, LeftMarginal: 20, RightMarginal: 20, Universe: 1000, Available: true}}, weights)
	if !(one.Affinity < two.Affinity && two.Affinity < five.Affinity) {
		t.Fatalf("affinity should grow monotonically with supporting evidence: %v, %v, %v", one.Affinity, two.Affinity, five.Affinity)
	}
	if one.Components[affinity.ActiveUsers].Shrinkage >= two.Components[affinity.ActiveUsers].Shrinkage {
		t.Fatal("one observation should be shrunk more than two")
	}
}

func TestUbiquitousHubOverlapScoresBelowDistinctiveOverlap(t *testing.T) {
	weights := affinity.Weights{affinity.ActiveUsers: 1}
	hub := affinity.Score(affinity.Evidence{ActiveUsers: affinity.Layer{Observed: 10, LeftMarginal: 900, RightMarginal: 20, Universe: 1000, Available: true}}, weights)
	distinctive := affinity.Score(affinity.Evidence{ActiveUsers: affinity.Layer{Observed: 10, LeftMarginal: 30, RightMarginal: 20, Universe: 1000, Available: true}}, weights)
	if hub.Affinity >= distinctive.Affinity {
		t.Fatalf("hub correction failed: hub=%v distinctive=%v", hub.Affinity, distinctive.Affinity)
	}
}

func TestInvalidAndEmptyEvidenceProduceZero(t *testing.T) {
	cases := []affinity.Evidence{
		{},
		{ActiveUsers: affinity.Layer{Observed: -1, LeftMarginal: 0, RightMarginal: 0, Universe: 0, Available: true}},
		{ActiveUsers: affinity.Layer{Observed: 50, LeftMarginal: 2, RightMarginal: 2, Universe: 1, Available: true}},
	}
	for _, evidence := range cases {
		result := affinity.Score(evidence, affinity.DefaultWeights())
		if result.Affinity != 0 || math.IsNaN(result.Affinity) {
			t.Fatalf("invalid evidence should safely score zero, got %+v", result)
		}
	}
}
