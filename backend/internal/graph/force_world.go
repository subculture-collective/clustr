package graph

import (
	"math"
	"sort"
)

// WorldNode is one body in the subreddit force layout.
type WorldNode struct {
	ID string
	// X, Y, Z seed the body when Seeded is true; otherwise a deterministic
	// spherical seed derived from ID is used.
	X, Y, Z float64
	Seeded  bool
	// Pinned bodies exert forces but never move. A warm build pins every
	// subreddit that already has a position so the published world stays put
	// while new subreddits settle among their neighbours.
	Pinned bool
	// Mass scales repulsion and gravity under WorldModelForceAtlas; callers
	// use degree+1 so hubs claim space and clusters form around them. Zero
	// means one.
	Mass float64
}

// WorldModel selects the force model.
type WorldModel int

const (
	// WorldModelFruchtermanReingold attracts along edges with d^2/k and repels
	// with k^2/d. On a dense graph it settles into one uniform ball.
	WorldModelFruchtermanReingold WorldModel = iota
	// WorldModelForceAtlas follows ForceAtlas2: attraction grows linearly
	// with distance, repulsion is mass_a*mass_b/d and gravity is a constant
	// pull per unit mass, so dense neighbourhoods contract into clusters and
	// hubs keep space around them.
	WorldModelForceAtlas
)

// WorldEdge joins two bodies by index with a positive attraction weight.
// Callers shape the weight; see PrepareWorldEdges.
type WorldEdge struct {
	A, B   int
	Weight float64
}

// WorldLayoutOptions tunes the simulation. Zero values take the defaults.
type WorldLayoutOptions struct {
	// Iterations of the force simulation. Cold builds need a few hundred; a
	// warm build only settles new bodies and can use far fewer.
	Iterations int
	// Theta is the Barnes-Hut opening angle (0 exact, 0.8 standard).
	Theta float64
	// Gravity pulls every free body toward the origin in proportion to its
	// distance, so disconnected components stay inside one world.
	Gravity float64
	// TargetRadius rescales a cold result so the 95th percentile of body
	// distance from the origin equals this value. Zero keeps the natural
	// scale. Rescaling never applies when any body is pinned.
	TargetRadius float64
	// Model selects the force model; the zero value is Fruchterman-Reingold.
	Model WorldModel
	// LinLog makes ForceAtlas attraction grow with log(1+d/k) instead of d,
	// so distant pairs pull less and dense groups separate more clearly.
	LinLog bool
}

// WorldLayoutStats describes a finished layout.
type WorldLayoutStats struct {
	Nodes, Edges, Pinned, Iterations int
	Scale                            float64
	Radius95                         float64
	MeanEdgeLength                   float64
}

const (
	defaultWorldIterations = 300
	defaultWorldTheta      = 0.8
	defaultWorldGravity    = 0.02
)

// LayoutWorld runs a Fruchterman-Reingold simulation in three dimensions:
// every body repels every other with a long-range k^2/d force through a
// Barnes-Hut octree, linked bodies attract along their edge with d^2/k scaled
// by the edge weight, and a weak gravity keeps the whole world bounded. It is deterministic for
// the same inputs and runs on one goroutine.
//
// The result is one position per node in input order.
func LayoutWorld(nodes []WorldNode, edges []WorldEdge, options WorldLayoutOptions) ([][3]float64, WorldLayoutStats) {
	n := len(nodes)
	stats := WorldLayoutStats{Nodes: n, Edges: len(edges), Scale: 1}
	positions := make([][3]float64, n)
	if n == 0 {
		return positions, stats
	}
	iterations := options.Iterations
	if iterations <= 0 {
		iterations = defaultWorldIterations
	}
	theta := options.Theta
	if theta <= 0 {
		theta = defaultWorldTheta
	}
	gravity := options.Gravity
	if gravity <= 0 {
		gravity = defaultWorldGravity
	}
	stats.Iterations = iterations

	// Natural scale: bodies spread over a sphere whose radius grows with the
	// cube root of the population, so the mean spacing k stays comparable.
	radius := 100.0 * math.Cbrt(float64(n)/1000.0+1)
	k := radius / math.Cbrt(float64(n))
	x := make([]float64, n)
	y := make([]float64, n)
	z := make([]float64, n)
	pinned := 0
	for i, node := range nodes {
		if node.Seeded && isFinite3D(node.X, node.Y, node.Z) {
			x[i], y[i], z[i] = node.X, node.Y, node.Z
		} else {
			x[i], y[i], z[i] = deterministicSphericalSeed(node.ID, radius)
		}
		if node.Pinned {
			pinned++
		}
	}
	stats.Pinned = pinned
	masses := make([]float64, n)
	var massSum float64
	for i, node := range nodes {
		masses[i] = node.Mass
		if !(masses[i] > 0) {
			masses[i] = 1
		}
		massSum += masses[i]
	}
	meanMass := massSum / float64(n)
	if pinned > 0 {
		// New bodies start at the natural scale of the pinned world, not at
		// the internal radius, so their first steps are proportionate.
		pinnedRadius := percentileRadius(x, y, z, nodes, 0.95)
		if pinnedRadius > 0 {
			k = pinnedRadius / math.Cbrt(float64(n))
		}
	}

	// Weights are relative: the mean edge pulls with the plain d^2/k force.
	weights := make([]float64, len(edges))
	var weightSum float64
	for i, edge := range edges {
		w := edge.Weight
		if !(w > 0) {
			w = 1e-9
		}
		weights[i] = w
		weightSum += w
	}
	if len(edges) > 0 {
		mean := weightSum / float64(len(edges))
		for i := range weights {
			weights[i] /= mean
		}
	}

	dispX := make([]float64, n)
	dispY := make([]float64, n)
	dispZ := make([]float64, n)
	repX := make([]float64, n)
	repY := make([]float64, n)
	repZ := make([]float64, n)
	atlas := options.Model == WorldModelForceAtlas
	// Under both models a mean pair of linked bodies balances at distance k.
	repStrength := k * k
	if atlas {
		repStrength = k * k / (meanMass * meanMass)
	}
	startTemperature := k * 4
	for it := 0; it < iterations; it++ {
		if atlas {
			calculateOctree3DMassForces(x, y, z, masses, repX, repY, repZ, theta, repStrength, inverseDistanceRepulsion)
		} else {
			calculateOctree3DForcesWithModel(x, y, z, repX, repY, repZ, theta, repStrength, inverseDistanceRepulsion)
		}
		for i := 0; i < n; i++ {
			// Repulsion plus gravity toward the origin: proportional to the
			// distance for Fruchterman-Reingold, constant per unit mass for
			// ForceAtlas.
			gx, gy, gz := gravity*x[i], gravity*y[i], gravity*z[i]
			if atlas {
				r := math.Sqrt(x[i]*x[i] + y[i]*y[i] + z[i]*z[i])
				if r > 1e-9 {
					pull := gravity * k * masses[i] / meanMass / r
					gx, gy, gz = pull*x[i], pull*y[i], pull*z[i]
				} else {
					gx, gy, gz = 0, 0, 0
				}
			}
			dispX[i] = repX[i] - gx
			dispY[i] = repY[i] - gy
			dispZ[i] = repZ[i] - gz
		}
		for ei, edge := range edges {
			dx := x[edge.A] - x[edge.B]
			dy := y[edge.A] - y[edge.B]
			dz := z[edge.A] - z[edge.B]
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist < 1e-6 {
				dx, dy, dz = deterministicDirection3D(edge.A, edge.B)
				dist = 1e-6
			}
			force := dist * dist / k * weights[ei]
			if atlas {
				force = dist * weights[ei]
				if options.LinLog {
					force = k * math.Log1p(dist/k) * weights[ei]
				}
			}
			ax := dx / dist * force
			ay := dy / dist * force
			az := dz / dist * force
			dispX[edge.A] -= ax
			dispY[edge.A] -= ay
			dispZ[edge.A] -= az
			dispX[edge.B] += ax
			dispY[edge.B] += ay
			dispZ[edge.B] += az
		}
		// Linear cooling bounds every step; pinned bodies never move.
		temperature := startTemperature * (1 - float64(it)/float64(iterations))
		if temperature < k*0.01 {
			temperature = k * 0.01
		}
		for i := 0; i < n; i++ {
			if nodes[i].Pinned {
				continue
			}
			dx, dy, dz := dispX[i], dispY[i], dispZ[i]
			disp := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if disp > 0 {
				step := math.Min(disp, temperature)
				x[i] += dx / disp * step
				y[i] += dy / disp * step
				z[i] += dz / disp * step
			}
		}
	}

	if pinned == 0 && options.TargetRadius > 0 {
		r95 := percentileRadius(x, y, z, nodes, 0.95)
		if r95 > 0 {
			stats.Scale = options.TargetRadius / r95
			for i := 0; i < n; i++ {
				x[i] *= stats.Scale
				y[i] *= stats.Scale
				z[i] *= stats.Scale
			}
		}
	}
	for i := 0; i < n; i++ {
		positions[i] = [3]float64{x[i], y[i], z[i]}
	}
	stats.Radius95 = percentileRadius(x, y, z, nodes, 0.95)
	if len(edges) > 0 {
		var total float64
		for _, edge := range edges {
			dx := x[edge.A] - x[edge.B]
			dy := y[edge.A] - y[edge.B]
			dz := z[edge.A] - z[edge.B]
			total += math.Sqrt(dx*dx + dy*dy + dz*dz)
		}
		stats.MeanEdgeLength = total / float64(len(edges))
	}
	return positions, stats
}

// percentileRadius returns the given percentile of distance from the origin
// over pinned bodies when any are pinned, otherwise over every body.
func percentileRadius(x, y, z []float64, nodes []WorldNode, percentile float64) float64 {
	pinnedOnly := false
	for _, node := range nodes {
		if node.Pinned {
			pinnedOnly = true
			break
		}
	}
	radii := make([]float64, 0, len(x))
	for i := range x {
		if pinnedOnly && !nodes[i].Pinned {
			continue
		}
		radii = append(radii, math.Sqrt(x[i]*x[i]+y[i]*y[i]+z[i]*z[i]))
	}
	if len(radii) == 0 {
		return 0
	}
	sort.Float64s(radii)
	index := int(math.Ceil(percentile*float64(len(radii)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(radii) {
		index = len(radii) - 1
	}
	return radii[index]
}

// ShellPosition places a body that has no edges on a deterministic spherical
// shell between the inner and outer radii, so isolated subreddits surround the
// connected world instead of hiding inside it.
func ShellPosition(id string, innerRadius, outerRadius float64) (float64, float64, float64) {
	ux, uy, uz := deterministicSphericalSeed(id, 1)
	length := math.Sqrt(ux*ux + uy*uy + uz*uz)
	if length < 1e-9 {
		ux, uy, uz, length = 0, 0, 1, 1
	}
	// A second hash picks the depth within the shell.
	_, _, depth := deterministicSphericalSeed(id+":shell", 1)
	radius := innerRadius + (outerRadius-innerRadius)*(depth+1)/2
	return ux / length * radius, uy / length * radius, uz / length * radius
}

// PrepareWorldEdges turns raw overlap counts into attraction weights. Each
// overlap is divided by the geometric mean of both endpoints' total overlap
// (cosine similarity), so a hub that shares a few users with everyone pulls no
// harder on a niche subreddit than that subreddit's real neighbours do. When
// perNode is positive, only each body's strongest perNode edges survive (an
// edge survives if either endpoint keeps it), which sparsifies a dense graph
// into the neighbourhood structure a graph view is meant to show.
func PrepareWorldEdges(bodyCount int, edges []WorldEdge, perNode int) []WorldEdge {
	strength := make([]float64, bodyCount)
	for _, edge := range edges {
		strength[edge.A] += edge.Weight
		strength[edge.B] += edge.Weight
	}
	weighted := make([]WorldEdge, len(edges))
	for i, edge := range edges {
		denominator := math.Sqrt(strength[edge.A] * strength[edge.B])
		weight := 0.0
		if denominator > 0 {
			weight = edge.Weight / denominator
		}
		weighted[i] = WorldEdge{A: edge.A, B: edge.B, Weight: weight}
	}
	if perNode <= 0 {
		return weighted
	}
	byBody := make([][]int, bodyCount)
	for i, edge := range weighted {
		byBody[edge.A] = append(byBody[edge.A], i)
		byBody[edge.B] = append(byBody[edge.B], i)
	}
	keep := make([]bool, len(weighted))
	for _, indices := range byBody {
		sort.SliceStable(indices, func(a, b int) bool { return weighted[indices[a]].Weight > weighted[indices[b]].Weight })
		for rank, index := range indices {
			if rank >= perNode {
				break
			}
			keep[index] = true
		}
	}
	kept := make([]WorldEdge, 0, len(weighted))
	for i, edge := range weighted {
		if keep[i] {
			kept = append(kept, edge)
		}
	}
	return kept
}
