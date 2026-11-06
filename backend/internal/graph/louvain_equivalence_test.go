package graph

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// referenceSinglePassLouvain is the implementation before community degree
// totals were maintained incrementally. It rescans every node for each
// candidate move and is kept only to prove the faster pass is equivalent.
func referenceSinglePassLouvain(nodeIDs []string, adjacency map[string]map[string]int, degrees map[string]int, totalWeight int) map[string]int {
	nodeToCommunity := make(map[string]int)
	for i, id := range nodeIDs {
		nodeToCommunity[id] = i
	}
	sum := func(community int) int {
		total := 0
		for node, comm := range nodeToCommunity {
			if comm == community {
				total += degrees[node]
			}
		}
		return total
	}
	gain := func(nodeID string, from, to int) float64 {
		if totalWeight == 0 {
			return 0
		}
		weightTo, weightFrom := 0, 0
		for neighbor, weight := range adjacency[nodeID] {
			switch nodeToCommunity[neighbor] {
			case to:
				weightTo += weight
			case from:
				weightFrom += weight
			}
		}
		m2 := float64(2 * totalWeight)
		return float64(weightTo-weightFrom)/m2 -
			float64(degrees[nodeID])*(float64(sum(to))-float64(sum(from)))/(m2*m2)
	}
	improved := true
	for iteration := 0; improved && iteration < 50; iteration++ {
		improved = false
		ordered := append([]string(nil), nodeIDs...)
		sort.Strings(ordered)
		for _, nodeID := range ordered {
			current := nodeToCommunity[nodeID]
			candidates := map[int]bool{}
			for neighbor := range adjacency[nodeID] {
				candidates[nodeToCommunity[neighbor]] = true
			}
			ids := make([]int, 0, len(candidates))
			for id := range candidates {
				ids = append(ids, id)
			}
			sort.Ints(ids)
			best, bestGain := current, 0.0
			for _, target := range ids {
				if target == current {
					continue
				}
				if g := gain(nodeID, current, target); g > bestGain {
					best, bestGain = target, g
				}
			}
			if best != current {
				nodeToCommunity[nodeID] = best
				improved = true
			}
		}
	}
	return nodeToCommunity
}

// randomCommunityGraph builds a deterministic graph with planted clusters,
// self-loops and repeated edges so weights exceed one.
func randomCommunityGraph(seed int64, nodes, edges int) ([]string, map[string]map[string]int, map[string]int, int) {
	rng := rand.New(rand.NewSource(seed))
	ids := make([]string, nodes)
	adjacency := make(map[string]map[string]int, nodes)
	degrees := make(map[string]int, nodes)
	for i := range ids {
		ids[i] = fmt.Sprintf("n_%d", i)
		adjacency[ids[i]] = map[string]int{}
	}
	clusters := 1 + nodes/25
	total := 0
	for e := 0; e < edges; e++ {
		a := rng.Intn(nodes)
		b := rng.Intn(nodes)
		if rng.Float64() < 0.8 {
			b = (a/clusters)*clusters + rng.Intn(clusters)
			if b >= nodes {
				b = a
			}
		}
		src, tgt := ids[a], ids[b]
		adjacency[src][tgt]++
		adjacency[tgt][src]++
		degrees[src]++
		degrees[tgt]++
		total++
	}
	// Shuffle the input order: production node order comes from the database.
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids, adjacency, degrees, total
}

func TestSinglePassLouvainMatchesReference(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		nodes := 20 + int(seed)*35
		ids, adjacency, degrees, total := randomCommunityGraph(seed, nodes, nodes*3)
		want := referenceSinglePassLouvain(ids, adjacency, degrees, total)
		got := runSinglePassLouvain(ids, adjacency, degrees, total)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: community assignment differs from reference", seed)
		}
	}
}

func BenchmarkSinglePassLouvain10k(b *testing.B) {
	ids, adjacency, degrees, total := randomCommunityGraph(7, 10000, 17755)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runSinglePassLouvain(ids, adjacency, degrees, total)
	}
}
