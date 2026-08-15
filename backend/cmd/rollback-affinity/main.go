package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/graph"
)

func main() {
	revisionID := flag.Int64("revision", 0, "retained published graph revision ID")
	catalogID := flag.Int64("catalog", 0, "retained published catalog referenced by the revision")
	confirm := flag.Bool("confirm-production-rollback", false, "required explicit rollback authorization")
	flag.Parse()
	if !*confirm {
		log.Fatal("refusing rollback without --confirm-production-rollback")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := graph.RollbackPublishedArtifacts(ctx, database, *revisionID, *catalogID); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("restored revision %d and catalog %d; disable v2/Catalog feature flags and redeploy the v1-compatible frontend separately\n", *revisionID, *catalogID)
}
