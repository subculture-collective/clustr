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
	"github.com/onnwee/reddit-cluster-map/backend/internal/affinity"
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
		log.Fatalf("resolve watermark: %v", err)
	}
	buildID, err := affinity.BuildShadow(ctx, database, watermark)
	if err != nil {
		log.Fatalf("shadow build %d failed: %v", buildID, err)
	}
	fmt.Printf("validated affinity shadow build %d at %s; current publication pointer unchanged\n", buildID, watermark.UTC().Format(time.RFC3339))
}
