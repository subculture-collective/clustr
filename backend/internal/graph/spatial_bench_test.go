package graph

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/subculture-collective/clustr/backend/internal/db"
)

// BenchmarkSpatialQuery_10k benchmarks bounding box query with 10k nodes
func BenchmarkSpatialQuery_10k(b *testing.B) {
	benchmarkSpatialQuery(b, 10000)
}

// BenchmarkSpatialQuery_50k benchmarks bounding box query with 50k nodes
func BenchmarkSpatialQuery_50k(b *testing.B) {
	benchmarkSpatialQuery(b, 50000)
}

// BenchmarkSpatialQuery_100k benchmarks bounding box query with 100k nodes
func BenchmarkSpatialQuery_100k(b *testing.B) {
	benchmarkSpatialQuery(b, 100000)
}

func benchmarkSpatialQuery(b *testing.B, nodeCount int) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		b.Skip("TEST_DATABASE_URL not set; skipping benchmark")
		return
	}

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		b.Fatalf("failed to open db: %v", err)
	}
	defer conn.Close()

	q := db.New(conn)
	ctx := context.Background()

	// Setup: Insert test nodes with random positions
	b.Logf("Setting up %d test nodes...", nodeCount)
	setupTestNodes(b, conn, nodeCount)

	// Cleanup
	b.Cleanup(func() {
		conn.ExecContext(context.Background(), "DELETE FROM graph_nodes WHERE id LIKE 'bench_%'")
	})

	// Reset timer to exclude setup time
	b.ResetTimer()

	// Run benchmark
	for i := 0; i < b.N; i++ {
		// Query a bounding box that contains ~10% of nodes (200x200 area in a 1000x1000 space)
		_, err := q.GetNodesInBoundingBox(ctx, db.GetNodesInBoundingBoxParams{
			PosX:   sql.NullFloat64{Float64: 0.0, Valid: true},
			PosX_2: sql.NullFloat64{Float64: 200.0, Valid: true},
			PosY:   sql.NullFloat64{Float64: 0.0, Valid: true},
			PosY_2: sql.NullFloat64{Float64: 200.0, Valid: true},
			PosZ:   sql.NullFloat64{Float64: -10.0, Valid: true},
			PosZ_2: sql.NullFloat64{Float64: 10.0, Valid: true},
			Limit:  10000,
		})
		if err != nil {
			b.Fatalf("GetNodesInBoundingBox failed: %v", err)
		}
	}
}

func setupTestNodes(b *testing.B, conn *sql.DB, count int) {
	ctx := context.Background()

	// Clear existing benchmark nodes
	_, err := conn.ExecContext(ctx, "DELETE FROM graph_nodes WHERE id LIKE 'bench_%'")
	if err != nil {
		b.Fatalf("failed to clear benchmark nodes: %v", err)
	}

	// Insert nodes in batches
	batchSize := 1000
	for i := 0; i < count; i += batchSize {
		remaining := count - i
		if remaining > batchSize {
			remaining = batchSize
		}

		// Build batch insert query
		query := "INSERT INTO graph_nodes (id, name, val, type, pos_x, pos_y, pos_z) VALUES "
		values := make([]interface{}, 0, remaining*7)

		for j := 0; j < remaining; j++ {
			idx := i + j
			if j > 0 {
				query += ", "
			}
			paramBase := j * 7
			query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
				paramBase+1, paramBase+2, paramBase+3, paramBase+4, paramBase+5, paramBase+6, paramBase+7)

			// Random position in 1000x1000x20 space
			values = append(values,
				fmt.Sprintf("bench_%d", idx),
				fmt.Sprintf("Bench Node %d", idx),
				fmt.Sprintf("%d", rand.Intn(1000)),
				"benchmark",
				rand.Float64()*1000.0,
				rand.Float64()*1000.0,
				(rand.Float64()-0.5)*20.0, // z in range [-10, 10]
			)
		}

		_, err := conn.ExecContext(ctx, query, values...)
		if err != nil {
			b.Fatalf("failed to insert batch at offset %d: %v", i, err)
		}
	}

	b.Logf("Inserted %d benchmark nodes", count)
}
