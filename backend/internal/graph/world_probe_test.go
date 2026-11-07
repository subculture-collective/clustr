package graph

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"
)

// TestWorldLayoutLocalityProbe lays out a clone export of the subreddit
// overlap graph and reports how well links and communities group, before and
// after. It runs only when WORLD_PROBE_DIR names a directory holding
// edges.csv (source,target,overlap), members.csv (node_id,community_id) and
// current.csv (id,x,y,z,community_id), exported with COPY ... TO STDOUT WITH
// CSV from subreddit_relationships, graph_revision_community_members and
// spatial_catalog_entities. WORLD_PROBE_ITERATIONS, WORLD_PROBE_GRAVITY,
// WORLD_PROBE_MODEL=atlas, WORLD_PROBE_LINLOG=1, WORLD_PROBE_WEIGHTING=cosine
// and WORLD_PROBE_TOPK tune the run; the catalog uses atlas, LinLog and
// cosine weights. Positions are written next to the inputs, or to
// WORLD_PROBE_OUT, for rendering.
func TestWorldLayoutLocalityProbe(t *testing.T) {
	dir := os.Getenv("WORLD_PROBE_DIR")
	if dir == "" {
		t.Skip("WORLD_PROBE_DIR not set")
	}
	iterations := 300
	if v, err := strconv.Atoi(os.Getenv("WORLD_PROBE_ITERATIONS")); err == nil && v > 0 {
		iterations = v
	}
	gravity := 0.0
	if v, err := strconv.ParseFloat(os.Getenv("WORLD_PROBE_GRAVITY"), 64); err == nil {
		gravity = v
	}
	weighting := os.Getenv("WORLD_PROBE_WEIGHTING")
	topK, _ := strconv.Atoi(os.Getenv("WORLD_PROBE_TOPK"))
	model := WorldModelFruchtermanReingold
	if os.Getenv("WORLD_PROBE_MODEL") == "atlas" {
		model = WorldModelForceAtlas
	}
	readCSV := func(name string) [][]string {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		rows, err := r.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	community := map[string]string{}
	for _, row := range readCSV("members.csv") {
		community[row[0]] = row[1]
	}
	current := map[string][3]float64{}
	currentCommunity := map[string]string{}
	for _, row := range readCSV("current.csv") {
		x, _ := strconv.ParseFloat(row[1], 64)
		y, _ := strconv.ParseFloat(row[2], 64)
		z, _ := strconv.ParseFloat(row[3], 64)
		current[row[0]] = [3]float64{x, y, z}
		if len(row) > 4 {
			currentCommunity[row[0]] = row[4]
		}
	}
	index := map[string]int{}
	var ids, communities []string
	var nodes []WorldNode
	var edges []WorldEdge
	seen := map[[2]int]int{}
	for _, row := range readCSV("edges.csv") {
		a, b := "subreddit_"+row[0], "subreddit_"+row[1]
		w, _ := strconv.ParseFloat(row[2], 64)
		for _, id := range []string{a, b} {
			if _, ok := index[id]; !ok {
				index[id] = len(nodes)
				c := community[id]
				if c == "" {
					c = currentCommunity[id]
				}
				ids = append(ids, id)
				communities = append(communities, c)
				nodes = append(nodes, WorldNode{ID: id})
			}
		}
		ia, ib := index[a], index[b]
		if ia == ib {
			continue
		}
		key := [2]int{ia, ib}
		if ia > ib {
			key = [2]int{ib, ia}
		}
		if at, ok := seen[key]; ok {
			if w > edges[at].Weight {
				edges[at].Weight = w
			}
			continue
		}
		seen[key] = len(edges)
		edges = append(edges, WorldEdge{A: key[0], B: key[1], Weight: w})
	}
	withCommunity := 0
	for _, c := range communities {
		if c != "" {
			withCommunity++
		}
	}
	t.Logf("bodies %d (%d with a community), edges %d, weighting %q, top-k %d", len(nodes), withCommunity, len(edges), weighting, topK)
	rawEdges := edges
	if weighting == "cosine" {
		edges = PrepareWorldEdges(len(nodes), edges, topK)
	} else {
		edges = make([]WorldEdge, len(rawEdges))
		for i, e := range rawEdges {
			edges[i] = WorldEdge{A: e.A, B: e.B, Weight: math.Log2(1 + e.Weight)}
		}
	}
	t.Logf("simulated edges %d, model %d", len(edges), model)
	for _, e := range rawEdges {
		nodes[e.A].Mass++
		nodes[e.B].Mass++
	}
	for i := range nodes {
		nodes[i].Mass++
	}

	currentPositions := make([][3]float64, len(nodes))
	for i, id := range ids {
		currentPositions[i] = current[id]
	}
	adjacency := make([]map[int]struct{}, len(nodes))
	for i := range adjacency {
		adjacency[i] = map[int]struct{}{}
	}
	for _, e := range rawEdges {
		adjacency[e.A][e.B] = struct{}{}
		adjacency[e.B][e.A] = struct{}{}
	}
	t.Logf("current: %s", localityReport(communities, currentPositions, rawEdges, adjacency))
	dumpPositions(t, filepath.Join(dir, "current-positions.csv"), ids, communities, currentPositions)

	start := time.Now()
	positions, stats := LayoutWorld(nodes, edges, WorldLayoutOptions{Iterations: iterations, TargetRadius: 60000, Gravity: gravity, Model: model, LinLog: os.Getenv("WORLD_PROBE_LINLOG") == "1"})
	t.Logf("layout %+v in %s", stats, time.Since(start).Truncate(time.Millisecond))
	t.Logf("force:   %s", localityReport(communities, positions, rawEdges, adjacency))
	out := os.Getenv("WORLD_PROBE_OUT")
	if out == "" {
		out = filepath.Join(dir, "force-positions.csv")
	}
	dumpPositions(t, out, ids, communities, positions)
}

func dumpPositions(t *testing.T, path string, ids, communities []string, positions [][3]float64) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	for i := range ids {
		_ = w.Write([]string{ids[i], strconv.FormatFloat(positions[i][0], 'f', 1, 64), strconv.FormatFloat(positions[i][1], 'f', 1, 64), strconv.FormatFloat(positions[i][2], 'f', 1, 64), communities[i]})
	}
	w.Flush()
}

func localityReport(communities []string, pos [][3]float64, edges []WorldEdge, adjacency []map[int]struct{}) string {
	n := len(pos)
	dist := func(a, b [3]float64) float64 {
		dx, dy, dz := a[0]-b[0], a[1]-b[1], a[2]-b[2]
		return math.Sqrt(dx*dx + dy*dy + dz*dz)
	}
	var sx, sy, sz float64
	radii := make([]float64, n)
	for i, p := range pos {
		sx += p[0] * p[0]
		sy += p[1] * p[1]
		sz += p[2] * p[2]
		radii[i] = math.Sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2])
	}
	sort.Float64s(radii)
	r95 := radii[int(0.95*float64(n))-1]
	var edgeTotal float64
	for _, e := range edges {
		edgeTotal += dist(pos[e.A], pos[e.B])
	}
	var randomTotal float64
	pairs := 0
	for i := 0; i < 20000; i++ {
		a := (i * 7919) % n
		b := (i*104729 + 13) % n
		if a == b {
			continue
		}
		randomTotal += dist(pos[a], pos[b])
		pairs++
	}
	// Community sizes first: purity only means something for communities
	// with several members.
	centroids := map[string][3]float64{}
	counts := map[string]int{}
	for i, c := range communities {
		if c == "" {
			continue
		}
		acc := centroids[c]
		acc[0] += pos[i][0]
		acc[1] += pos[i][1]
		acc[2] += pos[i][2]
		centroids[c] = acc
		counts[c]++
	}
	// Over a deterministic sample: the share of the 10 nearest bodies that
	// are graph neighbours, and that share the body's community.
	sample := 1000
	if sample > n {
		sample = n
	}
	type candidate struct {
		d float64
		j int
	}
	var purityTotal, linkTotal float64
	counted, linkCounted := 0, 0
	candidates := make([]candidate, 0, n)
	for s := 0; s < sample; s++ {
		i := (s * 7907) % n
		candidates = candidates[:0]
		for j := 0; j < n; j++ {
			if j != i {
				candidates = append(candidates, candidate{dist(pos[i], pos[j]), j})
			}
		}
		sort.Slice(candidates, func(a, b int) bool { return candidates[a].d < candidates[b].d })
		linked := 0
		for _, c := range candidates[:10] {
			if _, ok := adjacency[i][c.j]; ok {
				linked++
			}
		}
		linkTotal += float64(linked) / 10
		linkCounted++
		if communities[i] == "" || counts[communities[i]] < 5 {
			continue
		}
		same := 0
		for _, c := range candidates[:10] {
			if communities[c.j] == communities[i] {
				same++
			}
		}
		purityTotal += float64(same) / 10
		counted++
	}
	var keys []string
	for k, acc := range centroids {
		acc[0] /= float64(counts[k])
		acc[1] /= float64(counts[k])
		acc[2] /= float64(counts[k])
		centroids[k] = acc
		if counts[k] >= 5 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var ownTotal, otherTotal float64
	ownCount := 0
	for i, c := range communities {
		if c == "" || counts[c] < 5 {
			continue
		}
		nearest := math.Inf(1)
		for _, k := range keys {
			if k == c {
				continue
			}
			if d := dist(pos[i], centroids[k]); d < nearest {
				nearest = d
			}
		}
		ownTotal += dist(pos[i], centroids[c])
		otherTotal += nearest
		ownCount++
	}
	return strconv.Itoa(n) + " bodies; std x/y/z " +
		strconv.FormatFloat(math.Sqrt(sx/float64(n)), 'f', 0, 64) + "/" +
		strconv.FormatFloat(math.Sqrt(sy/float64(n)), 'f', 0, 64) + "/" +
		strconv.FormatFloat(math.Sqrt(sz/float64(n)), 'f', 0, 64) +
		"; r95 " + strconv.FormatFloat(r95, 'f', 0, 64) +
		"; edge/random-pair length " + strconv.FormatFloat((edgeTotal/float64(len(edges)))/(randomTotal/float64(pairs)), 'f', 3, 64) +
		"; knn10 linked share " + strconv.FormatFloat(linkTotal/float64(linkCounted), 'f', 3, 64) +
		"; knn10 community purity " + strconv.FormatFloat(purityTotal/float64(counted), 'f', 3, 64) + " (" + strconv.Itoa(counted) + " sampled)" +
		"; own/nearest-other centroid " + strconv.FormatFloat((ownTotal/float64(ownCount))/(otherTotal/float64(ownCount)), 'f', 3, 64) +
		" over " + strconv.Itoa(len(keys)) + " communities"
}
