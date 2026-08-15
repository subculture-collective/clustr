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
	buildID := flag.Int64("build", 0, "validated affinity build ID")
	catalogID := flag.Int64("catalog", 0, "published immutable catalog ID at the same watermark")
	confirm := flag.Bool("confirm-production-promotion", false, "required explicit promotion authorization")
	flag.Parse()
	if !*confirm {
		log.Fatal("refusing promotion without --confirm-production-promotion")
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	revisionID, err := graph.PromoteAffinityShadow(ctx, database, *buildID, *catalogID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("promoted affinity revision %d with build %d and catalog %d\n", revisionID, *buildID, *catalogID)
}
