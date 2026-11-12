package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/subculture-collective/clustr/backend/internal/config"
	"github.com/subculture-collective/clustr/backend/internal/integrity"
)

func main() {
	// Define command line flags
	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
	cleanCmd := flag.NewFlagSet("clean", flag.ExitOnError)
	statsCmd := flag.NewFlagSet("stats", flag.ExitOnError)
	bloatCmd := flag.NewFlagSet("bloat", flag.ExitOnError)

	cleanType := cleanCmd.String("type", "all", "Type of cleanup: all, posts, comments, graph-links, graph-nodes")
	cleanBatch := cleanCmd.Int("batch", 1000, "Batch size for cleanup operations")
	cleanDryRun := cleanCmd.Bool("dry-run", false, "Run in dry-run mode (don't actually delete)")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Load config (for any future config needs)
	_ = config.Load()

	// Get database connection string from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Connect to database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Create service
	svc := integrity.NewService(db)
	ctx := context.Background()

	// Parse command
	switch os.Args[1] {
	case "check":
		checkCmd.Parse(os.Args[2:])
		runCheck(ctx, svc)
	case "clean":
		cleanCmd.Parse(os.Args[2:])
		runClean(ctx, svc, *cleanType, *cleanBatch, *cleanDryRun)
	case "stats":
		statsCmd.Parse(os.Args[2:])
		runStats(ctx, svc)
	case "bloat":
		bloatCmd.Parse(os.Args[2:])
		runBloat(ctx, svc)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Reddit Cluster Map - Data Integrity Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  integrity check                    - Run all integrity checks")
	fmt.Println("  integrity clean [options]          - Clean up data integrity issues")
	fmt.Println("  integrity stats                    - Show database statistics")
	fmt.Println("  integrity bloat                    - Analyze table bloat")
	fmt.Println()
	fmt.Println("Clean options:")
	fmt.Println("  -type string     Type of cleanup (default: all)")
	fmt.Println("                   Options: all, posts, comments, graph-links, graph-nodes")
	fmt.Println("  -batch int       Batch size for cleanup (default: 1000)")
	fmt.Println("  -dry-run         Run in dry-run mode (default: false)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  integrity check")
	fmt.Println("  integrity clean -type posts -batch 500")
	fmt.Println("  integrity clean -dry-run")
	fmt.Println("  integrity stats")
}

func runCheck(ctx context.Context, svc *integrity.Service) {
	log.Println("Running integrity checks...")

	results, err := svc.CheckAllIntegrity(ctx, 100, 0)
	if err != nil {
		log.Fatalf("Failed to run integrity checks: %v", err)
	}

	fmt.Println()
	fmt.Println("=== Integrity Check Results ===")
	fmt.Println()

	hasAnyIssues := false
	for _, result := range results {
		status := "✓ OK"
		if result.HasIssues {
			status = fmt.Sprintf("⚠ ISSUES FOUND: %d", result.IssueCount)
			hasAnyIssues = true
		}

		fmt.Printf("%-30s %s\n", result.CheckName+":", status)
		fmt.Printf("  %s\n", result.Details)
		fmt.Println()
	}

	if hasAnyIssues {
		fmt.Println("Run 'integrity clean' to fix issues")
		os.Exit(1)
	} else {
		fmt.Println("All integrity checks passed!")
	}
}

func runClean(ctx context.Context, svc *integrity.Service, cleanType string, batchSize int, dryRun bool) {
	if dryRun {
		log.Println("Running in DRY-RUN mode (no changes will be made)")
		// For dry-run, just show what would be deleted
		results, err := svc.CheckAllIntegrity(ctx, int32(batchSize), 0)
		if err != nil {
			log.Fatalf("Failed to check integrity: %v", err)
		}

		fmt.Println()
		fmt.Println("=== Dry-Run: Would Clean ===")
		for _, result := range results {
			if result.HasIssues {
				fmt.Printf("%s: %d items\n", result.CheckName, result.IssueCount)
			}
		}
		return
	}

	log.Printf("Cleaning up data integrity issues (type: %s, batch: %d)...", cleanType, batchSize)

	startTime := time.Now()
	var totalDeleted int64
	var err error

	switch cleanType {
	case "posts":
		totalDeleted, err = svc.CleanupOrphanPosts(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup orphan posts: %v", err)
		}
		fmt.Printf("Cleaned up %d orphan posts\n", totalDeleted)

	case "comments":
		totalDeleted, err = svc.CleanupOrphanComments(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup orphan comments: %v", err)
		}
		fmt.Printf("Cleaned up %d orphan comments\n", totalDeleted)

	case "graph-links":
		totalDeleted, err = svc.CleanupDanglingGraphLinks(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup dangling graph links: %v", err)
		}
		fmt.Printf("Cleaned up %d dangling graph links\n", totalDeleted)

	case "graph-nodes":
		totalDeleted, err = svc.CleanupOrphanGraphNodes(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup orphan graph nodes: %v", err)
		}
		fmt.Printf("Cleaned up %d orphan graph nodes\n", totalDeleted)

	case "all":
		// Clean in order: comments -> posts -> graph-links -> graph-nodes
		log.Println("Cleaning orphan comments...")
		count, err := svc.CleanupOrphanComments(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup orphan comments: %v", err)
		}
		fmt.Printf("  - Cleaned up %d orphan comments\n", count)
		totalDeleted += count

		log.Println("Cleaning orphan posts...")
		count, err = svc.CleanupOrphanPosts(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup orphan posts: %v", err)
		}
		fmt.Printf("  - Cleaned up %d orphan posts\n", count)
		totalDeleted += count

		log.Println("Cleaning dangling graph links...")
		count, err = svc.CleanupDanglingGraphLinks(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup dangling graph links: %v", err)
		}
		fmt.Printf("  - Cleaned up %d dangling graph links\n", count)
		totalDeleted += count

		log.Println("Cleaning orphan graph nodes...")
		count, err = svc.CleanupOrphanGraphNodes(ctx, int32(batchSize))
		if err != nil {
			log.Fatalf("Failed to cleanup orphan graph nodes: %v", err)
		}
		fmt.Printf("  - Cleaned up %d orphan graph nodes\n", count)
		totalDeleted += count

	default:
		log.Fatalf("Unknown cleanup type: %s", cleanType)
	}

	duration := time.Since(startTime)
	fmt.Printf("\nTotal items cleaned: %d\n", totalDeleted)
	fmt.Printf("Time taken: %v\n", duration)
}

func runStats(ctx context.Context, svc *integrity.Service) {
	log.Println("Retrieving database statistics...")

	stats, err := svc.GetDatabaseStatistics(ctx)
	if err != nil {
		log.Fatalf("Failed to get database statistics: %v", err)
	}

	fmt.Println()
	fmt.Println("=== Database Statistics ===")
	fmt.Println()
	fmt.Printf("%-25s %12s %12s %12s %20s %20s\n",
		"Table", "Size", "Rows", "Dead Rows", "Last Vacuum", "Last Analyze")
	fmt.Println(strings.Repeat("-", 120))
	for _, stat := range stats {
		lastVacuum := "Never"
		if stat.LastVacuum != nil {
			lastVacuum = stat.LastVacuum.Format("2006-01-02 15:04")
		} else if stat.LastAutoVacuum != nil {
			lastVacuum = stat.LastAutoVacuum.Format("2006-01-02 15:04") + " (auto)"
		}

		lastAnalyze := "Never"
		if stat.LastAnalyze != nil {
			lastAnalyze = stat.LastAnalyze.Format("2006-01-02 15:04")
		} else if stat.LastAutoAnalyze != nil {
			lastAnalyze = stat.LastAutoAnalyze.Format("2006-01-02 15:04") + " (auto)"
		}

		fmt.Printf("%-25s %12s %12d %12d %20s %20s\n",
			stat.TableName, stat.Size, stat.RowCount, stat.DeadRows, lastVacuum, lastAnalyze)
	}
}

func runBloat(ctx context.Context, svc *integrity.Service) {
	log.Println("Analyzing table bloat...")

	stats, err := svc.GetBloatAnalysis(ctx)
	if err != nil {
		log.Fatalf("Failed to analyze bloat: %v", err)
	}

	fmt.Println()
	fmt.Println("=== Table Bloat Analysis ===")
	fmt.Println()
	fmt.Printf("%-25s %12s %12s %12s %10s\n",
		"Table", "Size", "Live Rows", "Dead Rows", "% Dead")
	fmt.Println(strings.Repeat("-", 80))

	for _, stat := range stats {
		percentDead := 0.0
		if stat.RowCount+stat.DeadRows > 0 {
			percentDead = float64(stat.DeadRows) / float64(stat.RowCount+stat.DeadRows) * 100
		}

		fmt.Printf("%-25s %12s %12d %12d %9.2f%%\n",
			stat.TableName, stat.Size, stat.RowCount, stat.DeadRows, percentDead)
	}

	fmt.Println()
	fmt.Println("Tables with >10% dead tuples should be vacuumed.")
	fmt.Println("Run: VACUUM ANALYZE <table_name>;")
}
