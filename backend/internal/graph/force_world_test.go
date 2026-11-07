package graph

import (
	"fmt"
	"math"
	"testing"
)

// twoClusters builds two dense groups of size n joined by one weak edge.
func twoClusters(n int) ([]WorldNode, []WorldEdge) {
	nodes := make([]WorldNode, 0, 2*n)
	for i := 0; i < 2*n; i++ {
		nodes = append(nodes, WorldNode{ID: fmt.Sprintf("subreddit_%d", i)})
	}
	var edges []WorldEdge
	for group := 0; group < 2; group++ {
		base := group * n
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if (i*7+j*3)%4 == 0 {
					edges = append(edges, WorldEdge{A: base + i, B: base + j, Weight: 20})
				}
			}
		}
	}
	edges = append(edges, WorldEdge{A: 0, B: n, Weight: 1})
	return nodes, edges
}

func meanDistance(positions [][3]float64, a, b []int) float64 {
	var total float64
	var count int
	for _, i := range a {
		for _, j := range b {
			if i == j {
				continue
			}
			dx := positions[i][0] - positions[j][0]
			dy := positions[i][1] - positions[j][1]
			dz := positions[i][2] - positions[j][2]
			total += math.Sqrt(dx*dx + dy*dy + dz*dz)
			count++
		}
	}
	return total / float64(count)
}

func indexRange(from, to int) []int {
	out := make([]int, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}

func TestLayoutWorldSeparatesLinkedClusters(t *testing.T) {
	nodes, edges := twoClusters(40)
	positions, stats := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: 200})
	if stats.Nodes != 80 || stats.Edges != len(edges) {
		t.Fatalf("stats = %+v", stats)
	}
	first, second := indexRange(0, 40), indexRange(40, 80)
	within := (meanDistance(positions, first, first) + meanDistance(positions, second, second)) / 2
	between := meanDistance(positions, first, second)
	if between < within*1.5 {
		t.Fatalf("clusters did not separate: within %.2f, between %.2f", within, between)
	}
	for i, position := range positions {
		if !isFinite3D(position[0], position[1], position[2]) {
			t.Fatalf("position %d is not finite: %v", i, position)
		}
	}
}

func TestLayoutWorldForceAtlasLinLogSeparatesClustersWithCosineWeights(t *testing.T) {
	nodes, edges := twoClusters(40)
	for _, edge := range edges {
		nodes[edge.A].Mass++
		nodes[edge.B].Mass++
	}
	for i := range nodes {
		nodes[i].Mass++
	}
	prepared := PrepareWorldEdges(len(nodes), edges, 0)
	if len(prepared) != len(edges) {
		t.Fatalf("prepared %d edges, want %d", len(prepared), len(edges))
	}
	for _, edge := range prepared {
		if !(edge.Weight > 0 && edge.Weight <= 1) {
			t.Fatalf("cosine weight %g is outside (0,1]", edge.Weight)
		}
	}
	positions, _ := LayoutWorld(nodes, prepared, WorldLayoutOptions{Iterations: 200, Model: WorldModelForceAtlas, LinLog: true})
	first, second := indexRange(0, 40), indexRange(40, 80)
	within := (meanDistance(positions, first, first) + meanDistance(positions, second, second)) / 2
	between := meanDistance(positions, first, second)
	if between < within*1.5 {
		t.Fatalf("clusters did not separate: within %.2f, between %.2f", within, between)
	}
}

func TestPrepareWorldEdgesKeepsStrongestPerBody(t *testing.T) {
	edges := []WorldEdge{{A: 0, B: 1, Weight: 10}, {A: 0, B: 2, Weight: 1}, {A: 0, B: 3, Weight: 5}, {A: 2, B: 3, Weight: 1}}
	kept := PrepareWorldEdges(4, edges, 1)
	// Body 0 keeps 0-1; body 1 keeps 0-1; body 2 keeps 2-3 (its stronger
	// cosine); body 3 keeps its strongest. Every surviving edge is one of
	// those choices and the weakest hub edge 0-2 is gone.
	for _, edge := range kept {
		if edge.A == 0 && edge.B == 2 {
			t.Fatalf("weak edge 0-2 survived pruning: %+v", kept)
		}
	}
	if len(kept) == 0 || len(kept) >= len(edges) {
		t.Fatalf("pruning kept %d of %d edges", len(kept), len(edges))
	}
}

func TestLayoutWorldIsDeterministic(t *testing.T) {
	nodes, edges := twoClusters(15)
	first, _ := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: 50})
	second, _ := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: 50})
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("position %d differs between runs: %v vs %v", i, first[i], second[i])
		}
	}
}

func TestLayoutWorldRescalesToTargetRadius(t *testing.T) {
	nodes, edges := twoClusters(30)
	positions, stats := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: 100, TargetRadius: 60000})
	if math.Abs(stats.Radius95-60000) > 1 {
		t.Fatalf("radius95 = %.2f, want 60000", stats.Radius95)
	}
	var far int
	for _, position := range positions {
		if math.Sqrt(position[0]*position[0]+position[1]*position[1]+position[2]*position[2]) > 60000 {
			far++
		}
	}
	if far > len(positions)/20+1 {
		t.Fatalf("%d of %d bodies lie beyond the target radius", far, len(positions))
	}
}

func TestLayoutWorldKeepsPinnedBodiesAndSettlesNewOnes(t *testing.T) {
	nodes, edges := twoClusters(20)
	settled, _ := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: 150, TargetRadius: 1000})
	for i := range nodes {
		nodes[i].X, nodes[i].Y, nodes[i].Z = settled[i][0], settled[i][1], settled[i][2]
		nodes[i].Seeded = true
		nodes[i].Pinned = true
	}
	// A newcomer linked to the second cluster starts next to the first one.
	newcomer := len(nodes)
	nodes = append(nodes, WorldNode{ID: "subreddit_new", X: settled[0][0] + 5, Y: settled[0][1], Z: settled[0][2], Seeded: true})
	for j := 20; j < 40; j++ {
		edges = append(edges, WorldEdge{A: newcomer, B: j, Weight: 30})
	}
	positions, stats := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: 150, TargetRadius: 1000})
	if stats.Pinned != 40 || stats.Scale != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	for i := 0; i < newcomer; i++ {
		if positions[i] != settled[i] {
			t.Fatalf("pinned body %d moved from %v to %v", i, settled[i], positions[i])
		}
	}
	toFirst := meanDistance(positions, []int{newcomer}, indexRange(0, 20))
	toSecond := meanDistance(positions, []int{newcomer}, indexRange(20, 40))
	if toSecond >= toFirst {
		t.Fatalf("newcomer stayed near the wrong cluster: first %.2f, second %.2f", toFirst, toSecond)
	}
}

func TestShellPositionStaysOnShell(t *testing.T) {
	for _, id := range []string{"subreddit_1", "subreddit_2", "subreddit_999999"} {
		x, y, z := ShellPosition(id, 80000, 100000)
		radius := math.Sqrt(x*x + y*y + z*z)
		if radius < 80000-1e-6 || radius > 100000+1e-6 {
			t.Fatalf("%s radius %.2f is outside the shell", id, radius)
		}
		x2, y2, z2 := ShellPosition(id, 80000, 100000)
		if x != x2 || y != y2 || z != z2 {
			t.Fatalf("%s is not deterministic", id)
		}
	}
}
