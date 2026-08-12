package graph

import (
	"math"
	"testing"
)

func TestOctree3DCoincidentBucketIsFiniteAndBounded(t *testing.T) {
	const count = 1000
	x, y, z := make([]float64, count), make([]float64, count), make([]float64, count)
	tree := buildOctree3D(x, y, z)
	if tree == nil || !tree.isLeaf() || len(tree.points) != count || tree.mass != count {
		t.Fatalf("unexpected coincident bucket: %#v", tree)
	}
	fx, fy, fz := make([]float64, count), make([]float64, count), make([]float64, count)
	calculateOctree3DForces(x, y, z, fx, fy, fz, 0.8, 1)
	for i := range fx {
		if math.IsNaN(fx[i]) || math.IsInf(fx[i], 0) || math.IsNaN(fy[i]) || math.IsInf(fy[i], 0) || math.IsNaN(fz[i]) || math.IsInf(fz[i], 0) {
			t.Fatalf("non-finite force at %d", i)
		}
	}
}

func TestOctree3DProducesZRepulsion(t *testing.T) {
	x, y, z := []float64{0, 0}, []float64{0, 0}, []float64{-10, 10}
	fx, fy, fz := make([]float64, 2), make([]float64, 2), make([]float64, 2)
	calculateOctree3DForces(x, y, z, fx, fy, fz, 0.8, 100)
	if fz[0] >= 0 || fz[1] <= 0 || math.Abs(fz[0]+fz[1]) > 1e-9 {
		t.Fatalf("unexpected z repulsion: %v", fz)
	}
}
