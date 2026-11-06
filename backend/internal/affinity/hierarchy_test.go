package affinity_test

import (
	"reflect"
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/affinity"
)

func TestWeightedMultiplexPartitionIsDeterministic(t *testing.T) {
	layers := []affinity.GraphLayer{
		{Name: affinity.ActiveUsers, Edges: []affinity.WeightedEdge{{A: 1, B: 2, Weight: 4}, {A: 2, B: 3, Weight: 4}, {A: 4, B: 5, Weight: 4}, {A: 5, B: 6, Weight: 4}, {A: 3, B: 4, Weight: .01}}},
		{Name: affinity.Authors, Edges: []affinity.WeightedEdge{{A: 1, B: 3, Weight: 2}, {A: 4, B: 6, Weight: 2}}},
	}
	first, err := affinity.PartitionMultiplex(layers, []float64{.75, .25}, 1, 91)
	if err != nil {
		t.Fatal(err)
	}
	second, err := affinity.PartitionMultiplex(layers, []float64{.75, .25}, 1, 91)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Communities, second.Communities) {
		t.Fatalf("partition changed across identical runs: %v != %v", first.Communities, second.Communities)
	}
	if len(first.Communities) != 2 {
		t.Fatalf("got %d communities, want 2: %v", len(first.Communities), first.Communities)
	}
}

func TestFinePartitionGates(t *testing.T) {
	communities := make([][]int64, 10)
	for index := range communities {
		for member := 0; member < 5; member++ {
			communities[index] = append(communities[index], int64(index*5+member+1))
		}
	}
	quality := affinity.AssessFinePartition(communities, 50)
	if !quality.Valid {
		t.Fatalf("expected valid profile: %+v", quality)
	}
	quality = affinity.AssessFinePartition([][]int64{{1}, {2}, {3}, {4, 5}}, 5)
	if quality.Valid {
		t.Fatalf("singleton-dominated profile should fail: %+v", quality)
	}
	pairSized := make([][]int64, 10)
	for index := range pairSized {
		pairSized[index] = []int64{int64(index*2 + 1), int64(index*2 + 2)}
	}
	quality = affinity.AssessFinePartition(pairSized, 20)
	if quality.Valid {
		t.Fatalf("median cluster size below five should fail: %+v", quality)
	}
}
