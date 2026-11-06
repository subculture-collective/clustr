package affinity

import (
	"reflect"
	"testing"
)

func TestMacroPartitionIsDeterministicAndAffinityDerived(t *testing.T) {
	communities := make([][]int64, 16)
	ids := make([]string, 16)
	for index := range communities {
		communities[index] = []int64{int64(index + 1)}
		ids[index] = "fine-" + string(rune('a'+index))
	}
	pairs := make([]stagedPair, 0, 23)
	for group := 0; group < 8; group++ {
		a := int64(group*2 + 1)
		b := a + 1
		pairs = append(pairs, stagedPair{a: a, b: b, score: Result{Affinity: 1}})
		if group < 7 {
			pairs = append(pairs, stagedPair{a: b, b: b + 1, score: Result{Affinity: .0001}})
		}
	}
	first, err := selectMacroPartition(pairs, communities, ids)
	if err != nil {
		t.Fatal(err)
	}
	second, err := selectMacroPartition(pairs, communities, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.groups) < 8 || len(first.groups) > 12 {
		t.Fatalf("macro partition has %d groups, want 8-12", len(first.groups))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("macro partition changed across identical affinity evidence: %+v != %+v", first, second)
	}
	if len(first.communityToMacro) != len(communities) {
		t.Fatalf("macro membership omitted fine communities: %v", first.communityToMacro)
	}
}

func TestUnsupportedNodeIsNotGivenManufacturedMembership(t *testing.T) {
	partition, err := PartitionMultiplex([]GraphLayer{{Name: ActiveUsers, Edges: []WeightedEdge{
		{A: 1, B: 2, Weight: 1}, {A: 2, B: 3, Weight: 1},
	}}}, []float64{1}, 1, PublicationSeed)
	if err != nil {
		t.Fatal(err)
	}
	for _, community := range partition.Communities {
		for _, node := range community {
			if node == 99 {
				t.Fatal("unsupported node was assigned by manufacturing a relationship")
			}
		}
	}
}

func TestUbiquitousDefaultCannotJoinOrganicGroupsWithZeroPositiveNPMI(t *testing.T) {
	pairs := []stagedPair{
		{a: 1, b: 2, score: Result{Components: map[Signal]Component{ActiveUsers: {Raw: 1}}}},
		{a: 3, b: 4, score: Result{Components: map[Signal]Component{ActiveUsers: {Raw: 1}}}},
		{a: 5, b: 6, score: Result{Components: map[Signal]Component{ActiveUsers: {Raw: 1}}}},
		// Node 99 is ubiquitous, but each relationship is anti-correlated after
		// marginal correction and therefore has no positive-NPMI evidence.
		{a: 1, b: 99, score: Result{Components: map[Signal]Component{ActiveUsers: {Raw: 0}}}},
		{a: 3, b: 99, score: Result{Components: map[Signal]Component{ActiveUsers: {Raw: 0}}}},
		{a: 5, b: 99, score: Result{Components: map[Signal]Component{ActiveUsers: {Raw: 0}}}},
	}
	layers, weights := layersFromPairs(pairs, Weights{ActiveUsers: 1}, false)
	partition, err := PartitionMultiplex(layers, weights, 1, PublicationSeed)
	if err != nil {
		t.Fatal(err)
	}
	if len(partition.Communities) != 3 {
		t.Fatalf("default hub merged organic groups: %v", partition.Communities)
	}
	for _, community := range partition.Communities {
		for _, node := range community {
			if node == 99 {
				t.Fatal("zero-positive-NPMI default hub received manufactured membership")
			}
		}
	}
}
