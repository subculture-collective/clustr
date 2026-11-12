package integrity_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/subculture-collective/clustr/backend/internal/integrity"
)

// ExampleService_CheckAllIntegrity demonstrates how to check data integrity
func ExampleService_CheckAllIntegrity() {
	// Connect to database using DATABASE_URL environment variable
	// Example: export DATABASE_URL="postgres://user:pass@localhost:5432/reddit_cluster?sslmode=disable"
	db, err := sql.Open("postgres", "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:5432/${DB_NAME}?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create service
	svc := integrity.NewService(db)
	ctx := context.Background()

	// Run all integrity checks
	results, err := svc.CheckAllIntegrity(ctx, 100, 0)
	if err != nil {
		log.Fatal(err)
	}

	// Process results
	for _, result := range results {
		if result.HasIssues {
			fmt.Printf("Found issues in %s: %d\n", result.CheckName, result.IssueCount)
		}
	}
}

// ExampleService_CleanupOrphanPosts demonstrates how to clean up orphan posts
func ExampleService_CleanupOrphanPosts() {
	// Use DATABASE_URL from environment
	db, err := sql.Open("postgres", "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:5432/${DB_NAME}?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	svc := integrity.NewService(db)
	ctx := context.Background()

	// Clean up orphan posts in batches of 1000
	deleted, err := svc.CleanupOrphanPosts(ctx, 1000)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Deleted %d orphan posts\n", deleted)
}

// ExampleService_GetDatabaseStatistics demonstrates how to get database statistics
func ExampleService_GetDatabaseStatistics() {
	// Use DATABASE_URL from environment
	db, err := sql.Open("postgres", "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:5432/${DB_NAME}?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	svc := integrity.NewService(db)
	ctx := context.Background()

	// Get statistics for all tables
	stats, err := svc.GetDatabaseStatistics(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Display statistics
	for _, stat := range stats {
		fmt.Printf("Table: %s, Rows: %d, Size: %s\n",
			stat.TableName, stat.RowCount, stat.Size)
	}
}

// ExampleService_GetBloatAnalysis demonstrates how to analyze table bloat
func ExampleService_GetBloatAnalysis() {
	// Use DATABASE_URL from environment
	db, err := sql.Open("postgres", "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:5432/${DB_NAME}?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	svc := integrity.NewService(db)
	ctx := context.Background()

	// Get bloat analysis
	stats, err := svc.GetBloatAnalysis(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Display tables with bloat
	for _, stat := range stats {
		if stat.DeadRows > 0 {
			totalRows := stat.RowCount + stat.DeadRows
			if totalRows > 0 {
				percentDead := float64(stat.DeadRows) / float64(totalRows) * 100
				fmt.Printf("Table: %s, Dead tuples: %.2f%%\n", stat.TableName, percentDead)
			}
		}
	}
}
