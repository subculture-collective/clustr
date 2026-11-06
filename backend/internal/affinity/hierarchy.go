package affinity

import (
	"errors"
	"log"
	"math/rand/v2"
	"slices"
	"sort"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/community"
	"gonum.org/v1/gonum/graph/simple"
)

type WeightedEdge struct {
	A, B   int64
	Weight float64
}
type GraphLayer struct {
	Name  Signal
	Edges []WeightedEdge
}

type Partition struct {
	Communities [][]int64
	Modularity  float64
	Resolution  float64
}

type FinePartitionQuality struct {
	Valid                    bool
	PositiveNodes            int
	NonSingletonNodeFraction float64
	SingletonFraction        float64
	MedianClusterSize        float64
}

// PartitionMultiplex is the deterministic weighted Louvain boundary. Stable,
// sorted source IDs and an explicit PCG seed make repeated builds reproducible.
func PartitionMultiplex(layers []GraphLayer, weights []float64, resolution float64, seed uint64) (Partition, error) {
	if len(layers) == 0 || len(layers) != len(weights) || resolution <= 0 {
		return Partition{}, errors.New("valid multiplex layers, weights, and resolution are required")
	}
	nodeSet := map[int64]bool{}
	for _, layer := range layers {
		for _, edge := range layer.Edges {
			if edge.A == edge.B || edge.Weight <= 0 {
				continue
			}
			nodeSet[edge.A] = true
			nodeSet[edge.B] = true
		}
	}
	ids := make([]int64, 0, len(nodeSet))
	for id := range nodeSet {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if len(ids) == 0 {
		return Partition{}, errors.New("multiplex has no positive-evidence nodes")
	}
	graphs := make([]graph.Undirected, len(layers))
	for index, layer := range layers {
		g := simple.NewWeightedUndirectedGraph(0, 0)
		for _, id := range ids {
			g.AddNode(simple.Node(id))
		}
		edges := append([]WeightedEdge(nil), layer.Edges...)
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].A != edges[j].A {
				return edges[i].A < edges[j].A
			}
			return edges[i].B < edges[j].B
		})
		for _, edge := range edges {
			if edge.A == edge.B || edge.Weight <= 0 {
				continue
			}
			a, b := edge.A, edge.B
			if a > b {
				a, b = b, a
			}
			g.SetWeightedEdge(g.NewWeightedEdge(simple.Node(a), simple.Node(b), edge.Weight))
		}
		graphs[index] = g
	}
	multiplex, err := community.NewUndirectedLayers(graphs...)
	if err != nil {
		return Partition{}, err
	}
	source := rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)
	reduced := community.ModularizeMultiplex(multiplex, weights, []float64{resolution}, false, source)
	communities := make([][]int64, 0)
	for _, members := range reduced.Communities() {
		ids := make([]int64, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.ID())
		}
		slices.Sort(ids)
		communities = append(communities, ids)
	}
	sort.Slice(communities, func(i, j int) bool { return communities[i][0] < communities[j][0] })
	q := community.QMultiplex(multiplex, reduced.Communities(), weights, []float64{resolution})
	var modularity float64
	for _, value := range q {
		modularity += value
	}
	return Partition{Communities: communities, Modularity: modularity, Resolution: resolution}, nil
}

func AssessFinePartition(communities [][]int64, positiveNodes int) FinePartitionQuality {
	quality := FinePartitionQuality{PositiveNodes: positiveNodes}
	if positiveNodes <= 0 || len(communities) == 0 {
		return quality
	}
	sizes := make([]int, 0, len(communities))
	nonSingletonNodes, singletons := 0, 0
	for _, members := range communities {
		size := len(members)
		sizes = append(sizes, size)
		if size > 1 {
			nonSingletonNodes += size
		} else if size == 1 {
			singletons++
		}
	}
	slices.Sort(sizes)
	middle := len(sizes) / 2
	if len(sizes)%2 == 0 {
		quality.MedianClusterSize = float64(sizes[middle-1]+sizes[middle]) / 2
	} else {
		quality.MedianClusterSize = float64(sizes[middle])
	}
	quality.NonSingletonNodeFraction = float64(nonSingletonNodes) / float64(positiveNodes)
	quality.SingletonFraction = float64(singletons) / float64(len(communities))
	quality.Valid = quality.NonSingletonNodeFraction >= .95 && quality.SingletonFraction < .20 && quality.MedianClusterSize >= 5 && quality.MedianClusterSize <= 500
	return quality
}

func SelectFinePartition(layers []GraphLayer, weights []float64, resolutions []float64, seed uint64) (Partition, FinePartitionQuality, error) {
	var best Partition
	var bestQuality FinePartitionQuality
	found := false
	nodes := map[int64]bool{}
	for _, layer := range layers {
		for _, edge := range layer.Edges {
			if edge.Weight > 0 {
				nodes[edge.A] = true
				nodes[edge.B] = true
			}
		}
	}
	for _, resolution := range resolutions {
		partition, err := PartitionMultiplex(layers, weights, resolution, seed)
		if err != nil {
			return Partition{}, FinePartitionQuality{}, err
		}
		quality := AssessFinePartition(partition.Communities, len(nodes))
		log.Printf("DIAG resolution=%.2f nodes=%d communities=%d modularity=%.4f nonsingleton=%.4f singleton=%.4f median=%.1f valid=%v",
			resolution, len(nodes), len(partition.Communities), partition.Modularity,
			quality.NonSingletonNodeFraction, quality.SingletonFraction, quality.MedianClusterSize, quality.Valid)
		if quality.Valid && (!found || partition.Modularity > best.Modularity) {
			best, bestQuality, found = partition, quality, true
		}
	}
	if !found {
		return Partition{}, FinePartitionQuality{}, errors.New("no resolution profile satisfies fine partition gates")
	}
	return best, bestQuality, nil
}
