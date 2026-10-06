package macrocluster_test

import (
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/macrocluster"
)

func TestFallbackGroupCoversAllCommunities(t *testing.T) {
	communities := make([]macrocluster.Community, 282)
	for index := range communities {
		communities[index] = macrocluster.Community{Index: index}
	}
	result := macrocluster.FallbackGroup(communities)
	if len(result.Groups) != macrocluster.MinGroups {
		t.Fatalf("fallback produced %d groups, want %d", len(result.Groups), macrocluster.MinGroups)
	}
	seen := make([]bool, 282)
	for _, group := range result.Groups {
		for _, index := range group {
			if seen[index] {
				t.Fatalf("index %d assigned twice", index)
			}
			seen[index] = true
		}
	}
	for index := range seen {
		if !seen[index] {
			t.Fatalf("index %d not assigned", index)
		}
	}
}

func TestFallbackGroupIsDeterministic(t *testing.T) {
	communities := make([]macrocluster.Community, 100)
	for index := range communities {
		communities[index] = macrocluster.Community{Index: index}
	}
	first := macrocluster.FallbackGroup(communities)
	second := macrocluster.FallbackGroup(communities)
	if len(first.Groups) != len(second.Groups) {
		t.Fatal("group count changed")
	}
	for group := range first.Groups {
		if len(first.Groups[group]) != len(second.Groups[group]) {
			t.Fatalf("group %d size changed: %d vs %d", group, len(first.Groups[group]), len(second.Groups[group]))
		}
		for index := range first.Groups[group] {
			if first.Groups[group][index] != second.Groups[group][index] {
				t.Fatalf("group %d index %d changed", group, index)
			}
		}
	}
}

func TestFingerprintIsStable(t *testing.T) {
	communities := []macrocluster.Community{
		{Index: 0, Representatives: []string{"gaming", "pcgaming"}, Topics: []string{"games", "steam"}},
		{Index: 1, Representatives: []string{"politics", "news"}, Topics: []string{"election", "policy"}},
	}
	first := macrocluster.Fingerprint(communities)
	second := macrocluster.Fingerprint(communities)
	if first != second {
		t.Fatalf("fingerprint changed: %s vs %s", first, second)
	}
	// Order of representatives should not matter.
	communities[0].Representatives = []string{"pcgaming", "gaming"}
	third := macrocluster.Fingerprint(communities)
	if first != third {
		t.Fatalf("fingerprint should be order-independent: %s vs %s", first, third)
	}
}

func TestValidateRejectsInvalidGroups(t *testing.T) {
	// Too few groups.
	if err := macrocluster.Validate([][]int{{0, 1}, {2, 3}}, 4); err == nil {
		t.Fatal("expected error for too few groups")
	}
	// Missing index.
	if err := macrocluster.Validate([][]int{{0}, {1}, {2}, {3}, {4}, {5}, {6}, {7}}, 9); err == nil {
		t.Fatal("expected error for missing index")
	}
	// Duplicate index.
	if err := macrocluster.Validate([][]int{{0, 0}, {1}, {2}, {3}, {4}, {5}, {6}, {7}}, 8); err == nil {
		t.Fatal("expected error for duplicate index")
	}
	// Valid.
	if err := macrocluster.Validate([][]int{{0, 1}, {2, 3}, {4}, {5}, {6}, {7}, {8}, {9}}, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}