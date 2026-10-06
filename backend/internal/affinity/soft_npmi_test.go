package affinity

import (
	"testing"
)

func TestSoftNPMIProducesTinyPositiveAffinity(t *testing.T) {
	// Negative-NPMI candidate: popular subreddit (50K users) + tiny (100 users), overlap 2
	e := Evidence{
		ActiveUsers: Layer{
			Observed: 2, LeftMarginal: 50000, RightMarginal: 100, Universe: 100000, Available: true,
		},
	}
	result := Score(e, DefaultWeights())
	if result.Affinity <= 0 {
		t.Fatal("soft NPMI should produce positive affinity for negative-NPMI candidates")
	}
	if result.Components[ActiveUsers].PositiveNPMI != 0 {
		t.Fatal("PositiveNPMI should still be 0 for negative NPMI (diagnostic field)")
	}

	// Positive-NPMI candidate: two small subreddits, overlap 10
	e2 := Evidence{
		ActiveUsers: Layer{
			Observed: 10, LeftMarginal: 30, RightMarginal: 20, Universe: 1000, Available: true,
		},
	}
	result2 := Score(e2, DefaultWeights())
	if result2.Affinity <= result.Affinity {
		t.Fatal("positive NPMI should score higher than negative NPMI")
	}
}
