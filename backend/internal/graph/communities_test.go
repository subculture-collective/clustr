package graph

import (
	"context"
	"database/sql"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// TestHierarchicalCommunityDetection tests the basic hierarchical Louvain implementation
func TestHierarchicalCommunityDetection(t *testing.T) {
	// Create a small test graph with clear hierarchical structure
	nodes := []db.ListGraphNodesByWeightRow{
		{ID: "n1", Name: "Node1", Val: sql.NullString{String: "10", Valid: true}, Type: sql.NullString{String: "test", Valid: true}},
		{ID: "n2", Name: "Node2", Val: sql.NullString{String: "10", Valid: true}, Type: sql.NullString{String: "test", Valid: true}},
		{ID: "n3", Name: "Node3", Val: sql.NullString{String: "10", Valid: true}, Type: sql.NullString{String: "test", Valid: true}},
		{ID: "n4", Name: "Node4", Val: sql.NullString{String: "10", Valid: true}, Type: sql.NullString{String: "test", Valid: true}},
		{ID: "n5", Name: "Node5", Val: sql.NullString{String: "10", Valid: true}, Type: sql.NullString{String: "test", Valid: true}},
		{ID: "n6", Name: "Node6", Val: sql.NullString{String: "10", Valid: true}, Type: sql.NullString{String: "test", Valid: true}},
	}

	// Links forming two clusters: {n1, n2, n3} and {n4, n5, n6}
	// With weak inter-cluster link
	links := []db.ListGraphLinksAmongRow{
		// Cluster 1
		{Source: "n1", Target: "n2"},
		{Source: "n2", Target: "n3"},
		{Source: "n3", Target: "n1"},
		// Cluster 2
		{Source: "n4", Target: "n5"},
		{Source: "n5", Target: "n6"},
		{Source: "n6", Target: "n4"},
		// Weak inter-cluster link
		{Source: "n3", Target: "n4"},
	}

	fs := newFakeStore()
	svc := NewService(fs)
	queries := &db.Queries{} // Will not be used in test, but needed for signature

	hierarchy, err := svc.detectHierarchicalCommunities(context.Background(), queries, nodes, links)
	if err != nil {
		t.Fatalf("hierarchical detection failed: %v", err)
	}

	// Verify we got at least level 0 and level 1
	if len(hierarchy) < 2 {
		t.Fatalf("expected at least 2 levels, got %d", len(hierarchy))
	}

	// Verify level 0: all nodes present
	level0 := hierarchy[0]
	if level0.Level != 0 {
		t.Errorf("level 0: expected Level=0, got %d", level0.Level)
	}
	if len(level0.NodeToCommunity) != len(nodes) {
		t.Errorf("level 0: expected %d nodes, got %d", len(nodes), len(level0.NodeToCommunity))
	}

	// Verify level 1: should have fewer communities than nodes
	level1 := hierarchy[1]
	if level1.Level != 1 {
		t.Errorf("level 1: expected Level=1, got %d", level1.Level)
	}

	uniqueComms := make(map[int]bool)
	for _, comm := range level1.NodeToCommunity {
		uniqueComms[comm] = true
	}
	if len(uniqueComms) >= len(nodes) {
		t.Errorf("level 1: expected fewer communities than nodes, got %d communities for %d nodes", len(uniqueComms), len(nodes))
	}
	if len(uniqueComms) < 1 {
		t.Errorf("level 1: expected at least 1 community, got %d", len(uniqueComms))
	}

	t.Logf("✅ Hierarchy has %d levels", len(hierarchy))
	for i, level := range hierarchy {
		unique := make(map[int]bool)
		for _, comm := range level.NodeToCommunity {
			unique[comm] = true
		}
		t.Logf("  Level %d: %d nodes, %d communities, modularity=%.3f", i, len(level.NodeToCommunity), len(unique), level.Modularity)
	}
}

// TestHierarchyValidation ensures hierarchy properties are maintained
func TestHierarchyValidation(t *testing.T) {
	nodes := []db.ListGraphNodesByWeightRow{
		{ID: "a", Name: "A", Val: sql.NullString{String: "5", Valid: true}},
		{ID: "b", Name: "B", Val: sql.NullString{String: "5", Valid: true}},
		{ID: "c", Name: "C", Val: sql.NullString{String: "5", Valid: true}},
		{ID: "d", Name: "D", Val: sql.NullString{String: "5", Valid: true}},
	}

	links := []db.ListGraphLinksAmongRow{
		{Source: "a", Target: "b"},
		{Source: "b", Target: "c"},
		{Source: "c", Target: "d"},
		{Source: "d", Target: "a"},
	}

	fs := newFakeStore()
	svc := NewService(fs)
	queries := &db.Queries{}

	hierarchy, err := svc.detectHierarchicalCommunities(context.Background(), queries, nodes, links)
	if err != nil {
		t.Fatalf("hierarchical detection failed: %v", err)
	}

	// Validate all nodes present at every level
	for _, level := range hierarchy {
		if len(level.NodeToCommunity) != len(nodes) {
			t.Errorf("level %d: missing nodes, expected %d, got %d", level.Level, len(nodes), len(level.NodeToCommunity))
		}

		// Check all original nodes are present
		for _, node := range nodes {
			if _, ok := level.NodeToCommunity[node.ID]; !ok {
				t.Errorf("level %d: missing node %s", level.Level, node.ID)
			}
		}
	}

	// Validate parent references exist (except level 0)
	for i := 1; i < len(hierarchy); i++ {
		level := hierarchy[i]
		if len(level.CommunityToParent) == 0 {
			t.Logf("⚠️ level %d: no parent references (may be OK if single community)", i)
		}
	}
}

// TestRunSinglePassLouvain tests the single-pass Louvain function
func TestRunSinglePassLouvain(t *testing.T) {
	// Create a simple graph
	nodeIDs := []string{"a", "b", "c", "d"}
	adjacency := map[string]map[string]int{
		"a": {"b": 1, "c": 1},
		"b": {"a": 1, "c": 1},
		"c": {"a": 1, "b": 1, "d": 1},
		"d": {"c": 1},
	}
	degrees := map[string]int{
		"a": 2,
		"b": 2,
		"c": 3,
		"d": 1,
	}
	totalWeight := 4 // Each edge counted once

	result := runSinglePassLouvain(nodeIDs, adjacency, degrees, totalWeight)

	// Verify all nodes have a community assignment
	if len(result) != len(nodeIDs) {
		t.Errorf("expected %d node assignments, got %d", len(nodeIDs), len(result))
	}

	for _, nodeID := range nodeIDs {
		if _, ok := result[nodeID]; !ok {
			t.Errorf("node %s missing from result", nodeID)
		}
	}

	// Count unique communities
	unique := make(map[int]bool)
	for _, comm := range result {
		unique[comm] = true
	}

	t.Logf("Single-pass Louvain: %d nodes -> %d communities", len(nodeIDs), len(unique))
}

// TestCalculateCentroidsForLevel tests centroid calculation
func TestCalculateCentroidsForLevel(t *testing.T) {
	nodes := []db.ListGraphNodesByWeightRow{
		{ID: "n1", Name: "N1", PosX: sql.NullFloat64{Float64: 0, Valid: true}, PosY: sql.NullFloat64{Float64: 0, Valid: true}, PosZ: sql.NullFloat64{Float64: 0, Valid: true}},
		{ID: "n2", Name: "N2", PosX: sql.NullFloat64{Float64: 10, Valid: true}, PosY: sql.NullFloat64{Float64: 0, Valid: true}, PosZ: sql.NullFloat64{Float64: 0, Valid: true}},
		{ID: "n3", Name: "N3", PosX: sql.NullFloat64{Float64: 0, Valid: true}, PosY: sql.NullFloat64{Float64: 10, Valid: true}, PosZ: sql.NullFloat64{Float64: 0, Valid: true}},
		{ID: "n4", Name: "N4", PosX: sql.NullFloat64{Float64: 10, Valid: true}, PosY: sql.NullFloat64{Float64: 10, Valid: true}, PosZ: sql.NullFloat64{Float64: 0, Valid: true}},
	}

	nodeToCommunity := map[string]int{
		"n1": 0,
		"n2": 0,
		"n3": 1,
		"n4": 1,
	}

	fs := newFakeStore()
	svc := NewService(fs)
	queries := &db.Queries{}

	centroids := svc.calculateCentroidsForLevel(context.Background(), queries, nodeToCommunity, nodes)

	// Check we have centroids for both communities
	if len(centroids) != 2 {
		t.Errorf("expected 2 centroids, got %d", len(centroids))
	}

	// Community 0: average of (0,0,0) and (10,0,0) = (5,0,0)
	if cent, ok := centroids[0]; ok {
		if cent[0] != 5.0 || cent[1] != 0.0 || cent[2] != 0.0 {
			t.Errorf("community 0: expected centroid (5,0,0), got (%.1f,%.1f,%.1f)", cent[0], cent[1], cent[2])
		}
	} else {
		t.Error("community 0: centroid not found")
	}

	// Community 1: average of (0,10,0) and (10,10,0) = (5,10,0)
	if cent, ok := centroids[1]; ok {
		if cent[0] != 5.0 || cent[1] != 10.0 || cent[2] != 0.0 {
			t.Errorf("community 1: expected centroid (5,10,0), got (%.1f,%.1f,%.1f)", cent[0], cent[1], cent[2])
		}
	} else {
		t.Error("community 1: centroid not found")
	}
}
