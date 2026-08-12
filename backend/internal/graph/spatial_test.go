package graph

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

func TestIntegration_SpatialQueries_BoundingBox(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
		return
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	ctx := context.Background()

	// Insert test nodes with known positions
	testNodes := []struct {
		id      string
		name    string
		val     string
		x, y, z float64
	}{
		{"node_1", "Node 1", "100", 0.0, 0.0, 0.0},
		{"node_2", "Node 2", "90", 5.0, 5.0, 0.0},
		{"node_3", "Node 3", "80", 10.0, 10.0, 0.0},
		{"node_4", "Node 4", "70", 15.0, 15.0, 0.0},
		{"node_5", "Node 5", "60", 20.0, 20.0, 0.0},
		{"node_outside", "Outside", "50", 100.0, 100.0, 0.0},
	}

	// Spatial queries are intentionally global, so isolate the workspace rather
	// than allowing graph rows from another integration case to affect counts.
	_, err = conn.ExecContext(ctx, "TRUNCATE graph_nodes, graph_links CASCADE")
	if err != nil {
		t.Fatalf("failed to clear test data: %v", err)
	}
	t.Cleanup(func() {
		conn.ExecContext(context.Background(), "DELETE FROM graph_nodes WHERE id LIKE 'node_%'")
	})

	// Insert test nodes
	for _, n := range testNodes {
		_, err = conn.ExecContext(ctx,
			`INSERT INTO graph_nodes (id, name, val, type, pos_x, pos_y, pos_z) 
			 VALUES ($1, $2, $3, 'test', $4, $5, $6)`,
			n.id, n.name, n.val, n.x, n.y, n.z)
		if err != nil {
			t.Fatalf("failed to insert test node %s: %v", n.id, err)
		}
	}

	t.Run("GetNodesInBoundingBox", func(t *testing.T) {
		// Query for nodes in the range x: [-1, 16], y: [-1, 16], z: [-1, 1]
		// This should return nodes 1-4 but not node 5 or node_outside
		nodes, err := q.GetNodesInBoundingBox(ctx, db.GetNodesInBoundingBoxParams{
			PosX:   sql.NullFloat64{Float64: -1.0, Valid: true}, // x_min
			PosX_2: sql.NullFloat64{Float64: 16.0, Valid: true}, // x_max
			PosY:   sql.NullFloat64{Float64: -1.0, Valid: true}, // y_min
			PosY_2: sql.NullFloat64{Float64: 16.0, Valid: true}, // y_max
			PosZ:   sql.NullFloat64{Float64: -1.0, Valid: true}, // z_min
			PosZ_2: sql.NullFloat64{Float64: 1.0, Valid: true},  // z_max
			Limit:  100,
		})
		if err != nil {
			t.Fatalf("GetNodesInBoundingBox failed: %v", err)
		}

		// Should get 4 nodes
		if len(nodes) != 4 {
			t.Errorf("expected 4 nodes, got %d", len(nodes))
		}

		// Verify nodes are within bounds
		for _, node := range nodes {
			if !node.PosX.Valid || !node.PosY.Valid || !node.PosZ.Valid {
				t.Errorf("node %s has invalid position", node.ID)
				continue
			}
			if node.PosX.Float64 < -1.0 || node.PosX.Float64 > 16.0 {
				t.Errorf("node %s x=%f out of bounds", node.ID, node.PosX.Float64)
			}
			if node.PosY.Float64 < -1.0 || node.PosY.Float64 > 16.0 {
				t.Errorf("node %s y=%f out of bounds", node.ID, node.PosY.Float64)
			}
			if node.PosZ.Float64 < -1.0 || node.PosZ.Float64 > 1.0 {
				t.Errorf("node %s z=%f out of bounds", node.ID, node.PosZ.Float64)
			}
		}
	})

	t.Run("GetNodesInBoundingBox2D", func(t *testing.T) {
		// Query for nodes in the 2D range x: [-1, 11], y: [-1, 11]
		// This should return nodes 1-3
		nodes, err := q.GetNodesInBoundingBox2D(ctx, db.GetNodesInBoundingBox2DParams{
			PosX:   sql.NullFloat64{Float64: -1.0, Valid: true}, // x_min
			PosX_2: sql.NullFloat64{Float64: 11.0, Valid: true}, // x_max
			PosY:   sql.NullFloat64{Float64: -1.0, Valid: true}, // y_min
			PosY_2: sql.NullFloat64{Float64: 11.0, Valid: true}, // y_max
			Limit:  100,
		})
		if err != nil {
			t.Fatalf("GetNodesInBoundingBox2D failed: %v", err)
		}

		// Should get 3 nodes
		if len(nodes) != 3 {
			t.Errorf("expected 3 nodes, got %d", len(nodes))
		}
	})

	t.Run("CountNodesInBoundingBox", func(t *testing.T) {
		// Count nodes in the range x: [-1, 16], y: [-1, 16], z: [-1, 1]
		count, err := q.CountNodesInBoundingBox(ctx, db.CountNodesInBoundingBoxParams{
			PosX:   sql.NullFloat64{Float64: -1.0, Valid: true}, // x_min
			PosX_2: sql.NullFloat64{Float64: 16.0, Valid: true}, // x_max
			PosY:   sql.NullFloat64{Float64: -1.0, Valid: true}, // y_min
			PosY_2: sql.NullFloat64{Float64: 16.0, Valid: true}, // y_max
			PosZ:   sql.NullFloat64{Float64: -1.0, Valid: true}, // z_min
			PosZ_2: sql.NullFloat64{Float64: 1.0, Valid: true},  // z_max
		})
		if err != nil {
			t.Fatalf("CountNodesInBoundingBox failed: %v", err)
		}

		// Should count 4 nodes
		if count != 4 {
			t.Errorf("expected count 4, got %d", count)
		}
	})

	t.Run("GetLinksForNodesInBoundingBox", func(t *testing.T) {
		// Create test links between nodes
		_, err = conn.ExecContext(ctx,
			`INSERT INTO graph_links (source, target) VALUES ($1, $2), ($3, $4)
			 ON CONFLICT (source, target) DO NOTHING`,
			"node_1", "node_2", "node_2", "node_3")
		if err != nil {
			t.Fatalf("failed to insert test links: %v", err)
		}
		t.Cleanup(func() {
			conn.ExecContext(context.Background(),
				`DELETE FROM graph_links WHERE source LIKE 'node_%' OR target LIKE 'node_%'`)
		})

		// Query for links where both nodes are in the bounding box
		links, err := q.GetLinksForNodesInBoundingBox(ctx, db.GetLinksForNodesInBoundingBoxParams{
			PosX:   sql.NullFloat64{Float64: -1.0, Valid: true}, // x_min
			PosX_2: sql.NullFloat64{Float64: 11.0, Valid: true}, // x_max
			PosY:   sql.NullFloat64{Float64: -1.0, Valid: true}, // y_min
			PosY_2: sql.NullFloat64{Float64: 11.0, Valid: true}, // y_max
			PosZ:   sql.NullFloat64{Float64: -1.0, Valid: true}, // z_min
			PosZ_2: sql.NullFloat64{Float64: 1.0, Valid: true},  // z_max
			Limit:  100,
		})
		if err != nil {
			t.Fatalf("GetLinksForNodesInBoundingBox failed: %v", err)
		}

		// Should get 2 links (node_1->node_2 and node_2->node_3)
		if len(links) != 2 {
			t.Errorf("expected 2 links, got %d", len(links))
		}
	})
}

func TestIntegration_SpatialIndex_Performance(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
		return
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	ctx := context.Background()

	// Query existing nodes with positions
	// This test verifies the spatial index can be used on real data
	nodes, err := q.GetNodesInBoundingBox(ctx, db.GetNodesInBoundingBoxParams{
		PosX:   sql.NullFloat64{Float64: -1000.0, Valid: true}, // x_min
		PosX_2: sql.NullFloat64{Float64: 1000.0, Valid: true},  // x_max
		PosY:   sql.NullFloat64{Float64: -1000.0, Valid: true}, // y_min
		PosY_2: sql.NullFloat64{Float64: 1000.0, Valid: true},  // y_max
		PosZ:   sql.NullFloat64{Float64: -1000.0, Valid: true}, // z_min
		PosZ_2: sql.NullFloat64{Float64: 1000.0, Valid: true},  // z_max
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("GetNodesInBoundingBox failed: %v", err)
	}

	t.Logf("Retrieved %d nodes with positions", len(nodes))

	// Verify all returned nodes have positions
	for _, node := range nodes {
		if !node.PosX.Valid || !node.PosY.Valid || !node.PosZ.Valid {
			t.Errorf("node %s missing position data", node.ID)
		}
	}
}

func TestIntegration_SpatialIndex_Exists(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
		return
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()

	// Check if the spatial index exists - fail if missing to ensure migrations are applied
	var indexExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes 
			WHERE indexname = 'idx_graph_nodes_spatial_nonnull'
		)
	`).Scan(&indexExists)
	if err != nil {
		t.Fatalf("failed to check for spatial index: %v", err)
	}

	if !indexExists {
		t.Fatal("Spatial index 'idx_graph_nodes_spatial_nonnull' not found; ensure migrations are applied in CI/test environment")
	}

	// Check if btree_gist extension is enabled
	var extensionExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_extension 
			WHERE extname = 'btree_gist'
		)
	`).Scan(&extensionExists)
	if err != nil {
		t.Fatalf("failed to check for btree_gist extension: %v", err)
	}

	if !extensionExists {
		t.Fatal("btree_gist extension is not enabled; ensure migrations are applied in CI/test environment")
	}

	t.Logf("Spatial index and btree_gist extension are properly configured")
}
