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
	watermarkFlag := flag.String("watermark", "", "fixed RFC3339 source watermark; defaults to database clock")
	flag.Parse()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	var watermark time.Time
	if *watermarkFlag != "" {
		watermark, err = time.Parse(time.RFC3339, *watermarkFlag)
	} else {
		err = database.QueryRowContext(ctx, `SELECT now()`).Scan(&watermark)
	}
	if err != nil {
		log.Fatal(err)
	}
	options := graph.DefaultSpatialCatalogOptions()
	options.ShadowOnly = true
	catalogID, err := graph.BuildSpatialCatalogWithOptions(ctx, database, watermark, options)
	if err != nil {
		log.Fatalf("catalog shadow %d failed: %v", catalogID, err)
	}
	fmt.Printf("validated full-text catalog shadow %d at %s; current catalog pointer unchanged\n", catalogID, watermark.UTC().Format(time.RFC3339))
}
