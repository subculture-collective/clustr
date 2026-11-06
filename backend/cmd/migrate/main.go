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
	"github.com/onnwee/reddit-cluster-map/backend/internal/migrations"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "Print build provenance and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("version=%s commit=%s build_time=%s\n", version, commit, buildTime)
		return
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "/app/migrations"
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("database is not ready: %v", err)
	}
	if err := (migrations.Runner{DB: db, Dir: dir}).Run(ctx); err != nil {
		log.Fatalf("schema migration failed: %v", err)
	}
	log.Print("schema is compatible and current")
}
