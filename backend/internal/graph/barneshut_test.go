package graph

import (
	"fmt"
	"math"
	"testing"
)

func TestNewBarnesHutNode(t *testing.T) {
	node := newBarnesHutNode(0, 0, 100, 100)
	if node == nil {
		t.Fatal("expected node to be created")
	}
	if !node.isLeaf {
		t.Error("new node should be a leaf")
	}
	if node.body != -1 {
		t.Error("new node should have no body")
	}
	if node.mass != 0 {
		t.Error("new node should have zero mass")
	}
}

func TestBarnesHutNodeInsertSingle(t *testing.T) {
	node := newBarnesHutNode(0, 0, 100, 100)
	node.insert(0, 50, 50, 1.0)

	if !node.isLeaf {
		t.Error("node with single particle should remain a leaf")
	}
	if node.body != 0 {
		t.Errorf("expected body=0, got %d", node.body)
	}
	if node.mass != 1.0 {
		t.Errorf("expected mass=1.0, got %f", node.mass)
	}
	if node.centerX != 50 || node.centerY != 50 {
		t.Errorf("expected center at (50,50), got (%f,%f)", node.centerX, node.centerY)
	}
}

func TestBarnesHutNodeInsertMultiple(t *testing.T) {
	node := newBarnesHutNode(0, 0, 100, 100)

	// Insert first particle in NW quadrant
	node.insert(0, 25, 25, 1.0)
	if !node.isLeaf {
		t.Error("node with single particle should remain a leaf")
	}

	// Insert second particle in SE quadrant - should split into quadrants
	node.insert(1, 75, 75, 1.0)
	if node.isLeaf {
		t.Error("node with two particles should not be a leaf")
	}
	if node.nw == nil || node.se == nil {
		t.Error("expected quadrants to be created")
	}

	// Check center of mass (should be at midpoint)
	expectedX := (25 + 75) / 2.0
	expectedY := (25 + 75) / 2.0
	if math.Abs(node.centerX-expectedX) > 1e-6 || math.Abs(node.centerY-expectedY) > 1e-6 {
		t.Errorf("expected center at (%f,%f), got (%f,%f)", expectedX, expectedY, node.centerX, node.centerY)
	}
	if node.mass != 2.0 {
		t.Errorf("expected total mass=2.0, got %f", node.mass)
	}
}

func TestBarnesHutNodeInsertQuadrants(t *testing.T) {
	node := newBarnesHutNode(0, 0, 100, 100)

	// Insert particles in each quadrant
	node.insert(0, 25, 25, 1.0) // NW
	node.insert(1, 75, 25, 1.0) // NE
	node.insert(2, 25, 75, 1.0) // SW
	node.insert(3, 75, 75, 1.0) // SE

	if node.isLeaf {
		t.Error("node with multiple particles should not be a leaf")
	}

	// Verify each quadrant has a particle
	if node.nw == nil || !node.nw.isLeaf || node.nw.body != 0 {
		t.Error("NW quadrant should contain particle 0")
	}
	if node.ne == nil || !node.ne.isLeaf || node.ne.body != 1 {
		t.Error("NE quadrant should contain particle 1")
	}
	if node.sw == nil || !node.sw.isLeaf || node.sw.body != 2 {
		t.Error("SW quadrant should contain particle 2")
	}
	if node.se == nil || !node.se.isLeaf || node.se.body != 3 {
		t.Error("SE quadrant should contain particle 3")
	}

	// Center of mass should be at (50, 50) since all particles have equal mass
	if math.Abs(node.centerX-50) > 1e-6 || math.Abs(node.centerY-50) > 1e-6 {
		t.Errorf("expected center at (50,50), got (%f,%f)", node.centerX, node.centerY)
	}
	if node.mass != 4.0 {
		t.Errorf("expected total mass=4.0, got %f", node.mass)
	}
}

func TestCalculateForceLeafSelf(t *testing.T) {
	node := newBarnesHutNode(0, 0, 100, 100)
	node.insert(0, 50, 50, 1.0)

	// Force on self should be zero
	fx, fy := node.calculateForce(0, 50, 50, 0.8, 1.0)
	if fx != 0 || fy != 0 {
		t.Errorf("force on self should be zero, got (%f,%f)", fx, fy)
	}
}

func TestCalculateForceTwoParticles(t *testing.T) {
	node := newBarnesHutNode(0, 0, 100, 100)
	node.insert(0, 40, 50, 1.0)
	node.insert(1, 60, 50, 1.0)

	// Force should be repulsive (pushing particles apart)
	fx, fy := node.calculateForce(0, 40, 50, 0.8, 1.0)

	// Particle 0 at (40,50) should be pushed left (negative fx)
	if fx >= 0 {
		t.Errorf("expected negative force (pushing left), got fx=%f", fx)
	}
	// No vertical component since particles are horizontally aligned
	if math.Abs(fy) > 1e-6 {
		t.Errorf("expected no vertical force, got fy=%f", fy)
	}
}

func TestBuildBarnesHutTree(t *testing.T) {
	X := []float64{10, 90, 10, 90}
	Y := []float64{10, 10, 90, 90}

	tree := buildBarnesHutTree(X, Y)
	if tree == nil {
		t.Fatal("expected tree to be built")
	}

	// Tree should be a square containing all particles
	if tree.width != tree.height {
		t.Errorf("expected square tree, got width=%f height=%f", tree.width, tree.height)
	}

	// Tree should contain all 4 particles (mass = 4.0)
	if tree.mass != 4.0 {
		t.Errorf("expected total mass=4.0, got %f", tree.mass)
	}

	// Center of mass should be at (50, 50)
	expectedX, expectedY := 50.0, 50.0
	if math.Abs(tree.centerX-expectedX) > 1e-6 || math.Abs(tree.centerY-expectedY) > 1e-6 {
		t.Errorf("expected center at (%f,%f), got (%f,%f)", expectedX, expectedY, tree.centerX, tree.centerY)
	}
}

func TestBuildBarnesHutTreeEmpty(t *testing.T) {
	X := []float64{}
	Y := []float64{}

	tree := buildBarnesHutTree(X, Y)
	if tree != nil {
		t.Error("expected nil tree for empty input")
	}
}

func TestBarnesHutCoincidentPointsUseBoundedBucket(t *testing.T) {
	const count = 1000
	x := make([]float64, count)
	y := make([]float64, count)
	tree := buildBarnesHutTree(x, y)
	if tree == nil || tree.mass != count {
		t.Fatalf("coincident tree mass=%v", tree)
	}
	if !tree.isLeaf || len(tree.bodies) != count {
		t.Fatalf("coincident particles were not retained in one bounded bucket: leaf=%v bodies=%d", tree.isLeaf, len(tree.bodies))
	}
	dx, dy := make([]float64, count), make([]float64, count)
	calculateBarnesHutForces(x, y, dx, dy, 0.8, 1)
	for i := range dx {
		if math.IsNaN(dx[i]) || math.IsInf(dx[i], 0) || math.IsNaN(dy[i]) || math.IsInf(dy[i], 0) {
			t.Fatalf("non-finite coincident force at %d", i)
		}
	}
}

func TestCalculateBarnesHutForces(t *testing.T) {
	// Two particles on horizontal line
	X := []float64{40, 60}
	Y := []float64{50, 50}

	dispX := make([]float64, 2)
	dispY := make([]float64, 2)
	calculateBarnesHutForces(X, Y, dispX, dispY, 0.8, 100.0)

	if len(dispX) != 2 || len(dispY) != 2 {
		t.Fatalf("expected 2 force values, got %d, %d", len(dispX), len(dispY))
	}

	// Particle 0 should be pushed left (negative)
	if dispX[0] >= 0 {
		t.Errorf("expected particle 0 to be pushed left, got fx=%f", dispX[0])
	}
	// Particle 1 should be pushed right (positive)
	if dispX[1] <= 0 {
		t.Errorf("expected particle 1 to be pushed right, got fx=%f", dispX[1])
	}
	// No vertical forces
	if math.Abs(dispY[0]) > 1e-6 || math.Abs(dispY[1]) > 1e-6 {
		t.Errorf("expected no vertical forces, got fy0=%f, fy1=%f", dispY[0], dispY[1])
	}

	// Forces should be equal and opposite (approximately)
	if math.Abs(dispX[0]+dispX[1]) > 1e-6 {
		t.Errorf("forces should be equal and opposite, got %f and %f", dispX[0], dispX[1])
	}
}

func TestCalculateBarnesHutForcesLarge(t *testing.T) {
	// Test with larger number of particles
	N := 100
	X := make([]float64, N)
	Y := make([]float64, N)

	// Arrange in a grid
	gridSize := 10
	for i := 0; i < N; i++ {
		X[i] = float64(i%gridSize) * 10
		Y[i] = float64(i/gridSize) * 10
	}

	dispX := make([]float64, N)
	dispY := make([]float64, N)
	calculateBarnesHutForces(X, Y, dispX, dispY, 0.8, 100.0)

	if len(dispX) != N || len(dispY) != N {
		t.Fatalf("expected %d force values, got %d, %d", N, len(dispX), len(dispY))
	}

	// Check that forces are non-zero for most particles (not on edges)
	nonZeroCount := 0
	for i := 0; i < N; i++ {
		if math.Abs(dispX[i]) > 1e-6 || math.Abs(dispY[i]) > 1e-6 {
			nonZeroCount++
		}
	}
	// Expect most particles to have non-zero forces
	if nonZeroCount < N/2 {
		t.Errorf("expected at least %d particles with non-zero force, got %d", N/2, nonZeroCount)
	}
}

func TestBarnesHutThetaParameter(t *testing.T) {
	// Same setup, different theta values
	X := []float64{0, 100, 0, 100}
	Y := []float64{0, 0, 100, 100}

	// With theta=0.0 (exact, no approximation)
	dispX0 := make([]float64, 4)
	dispY0 := make([]float64, 4)
	calculateBarnesHutForces(X, Y, dispX0, dispY0, 0.0, 100.0)

	// With theta=0.8 (standard approximation)
	dispX8 := make([]float64, 4)
	dispY8 := make([]float64, 4)
	calculateBarnesHutForces(X, Y, dispX8, dispY8, 0.8, 100.0)

	// Forces should be similar but not identical
	// Check that both produce reasonable repulsive forces
	for i := 0; i < 4; i++ {
		if math.Abs(dispX0[i]) < 1e-10 && math.Abs(dispX8[i]) < 1e-10 {
			// Both zero is fine
			continue
		}
		// Check that forces have same sign (direction)
		if (dispX0[i] > 0) != (dispX8[i] > 0) {
			t.Errorf("particle %d: force direction differs between theta=0.0 and theta=0.8", i)
		}
	}
}

// Benchmark Barnes-Hut vs brute force
func BenchmarkBarnesHutForces(b *testing.B) {
	sizes := []int{100, 1000, 5000}

	for _, N := range sizes {
		X := make([]float64, N)
		Y := make([]float64, N)
		for i := 0; i < N; i++ {
			X[i] = float64(i%100) * 10
			Y[i] = float64(i/100) * 10
		}

		b.Run(fmt.Sprintf("BarnesHut_%d", N), func(b *testing.B) {
			dispX := make([]float64, N)
			dispY := make([]float64, N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				calculateBarnesHutForces(X, Y, dispX, dispY, 0.8, 100.0)
			}
		})
	}
}
