package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/admin"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/errorreporting"
	"github.com/onnwee/reddit-cluster-map/backend/internal/graph"
	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
	"github.com/onnwee/reddit-cluster-map/backend/internal/migrations"
	"github.com/onnwee/reddit-cluster-map/backend/internal/tracing"
)

func main() {
	// Parse command-line flags
	fullRebuild := flag.Bool("full", false, "Force a full rebuild instead of incremental update")
	once := flag.Bool("once", false, "Run one calculation/publication and exit")
	publishOnly := flag.Bool("publish-only", false, "Publish the existing graph workspace without rebuilding it (requires --once)")
	fullCatalog := flag.Bool("full-catalog", false, "Build and publish a complete spatial catalog before the graph revision (requires --once)")
	flag.Parse()
	if *publishOnly && !*once {
		log.Fatal("--publish-only requires --once")
	}
	if *fullCatalog && !*once {
		log.Fatal("--full-catalog requires --once")
	}
	forceClear := forceClearEnabled()
	if err := validateRunOptions(*publishOnly, forceClear); err != nil {
		log.Fatal(err)
	}
	effectiveFullRebuild := effectiveFullRebuild(*fullRebuild, forceClear)

	// Load configuration
	cfg := config.Load()

	// Initialize structured logging
	logger.Init(cfg.LogLevel)
	logger.Info("Initializing graph precalculation", "version", cfg.SentryRelease, "log_level", cfg.LogLevel,
		"requested_full_rebuild", *fullRebuild, "force_clear", forceClear, "effective_full_rebuild", effectiveFullRebuild,
		"publish_only", *publishOnly, "full_catalog", *fullCatalog)

	// Initialize error reporting
	if err := errorreporting.Init(cfg.SentryEnvironment); err != nil {
		logger.Warn("Failed to initialize error reporting", "error", err)
	} else if errorreporting.IsSentryEnabled() {
		logger.Info("Error reporting initialized", "environment", cfg.SentryEnvironment)
		defer func() {
			logger.Info("Flushing error reports...")
			errorreporting.Flush(2 * time.Second)
		}()
	}

	// Initialize tracing
	shutdownTracing, err := tracing.Init("reddit-cluster-map-precalculate")
	if err != nil {
		logger.Warn("Failed to initialize tracing", "error", err)
	} else if cfg.OTELEnabled {
		logger.Info("Tracing initialized", "endpoint", cfg.OTELEndpoint, "sample_rate", cfg.OTELSampleRate)
		defer func() {
			logger.Info("Shutting down tracer...")
			if err := shutdownTracing(context.Background()); err != nil {
				logger.Error("Failed to shutdown tracer", "error", err)
			}
		}()
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Error("DATABASE_URL environment variable not set")
		log.Fatal("DATABASE_URL environment variable not set")
	}
	dbConn, err := sql.Open("postgres", dbURL)
	if err != nil {
		logger.Error("Failed to initialize DB", "error", err)
		log.Fatalf("Failed to initialize DB: %v", err)
	}
	defer dbConn.Close()

	// A bounded CPU-first worker must not consume the API's database pool.
	poolCap := positiveIntEnv("PRECALC_DB_MAX_OPEN_CONNS", 5)
	dbConn.SetMaxOpenConns(poolCap)
	dbConn.SetMaxIdleConns(min(poolCap, 2))
	dbConn.SetConnMaxLifetime(10 * time.Minute) // Longer lifetime for batch jobs
	dbConn.SetConnMaxIdleTime(5 * time.Minute)  // Longer idle time for batch jobs

	// Verify connection is working
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := dbConn.PingContext(ctx); err != nil {
			logger.Error("Failed to ping database", "error", err)
			log.Fatalf("Failed to ping database: %v", err)
		}
		if err := migrations.VerifyCurrent(ctx, dbConn); err != nil {
			logger.Error("Database schema is incompatible", "error", err)
			log.Fatal(err)
		}
		logger.Info("Database connection established")
	}

	queries := db.New(dbConn)
	// Honor admin toggle; if disabled, exit cleanly
	if ok, _ := admin.GetBool(context.Background(), queries, "precalc_enabled", true); !ok {
		logger.Info("Precalculation disabled by admin flag; exiting")
		return
	}
	graphService := graph.NewService(queries)

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	// Crawl leases, stale scheduling, and retry recovery belong to the crawler
	// lifecycle service. A graph worker must never mutate crawl scheduling state.

	// Always run once at start when enabled (may defer if not enough data yet).
	// A one-shot invocation is used by remote workers and deployment gates, so it
	// must fail closed instead of reporting success after a skipped/failed build.
	if err := runOnce(ctx, dbConn, queries, graphService, effectiveFullRebuild, *publishOnly, *fullCatalog); err != nil {
		logger.Error("Graph calculation/publication run failed", "error", err)
		if *once {
			log.Fatal(err)
		}
	}
	if *once {
		return
	}

	// Run continuously on a configurable interval (default 1h)
	interval := cfg.PublicationInterval
	if iv := os.Getenv("PRECALC_INTERVAL"); iv != "" {
		if d, err := time.ParseDuration(iv); err == nil {
			interval = d
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// If admin disabled, skip run
			if ok, _ := admin.GetBool(context.Background(), queries, "precalc_enabled", true); !ok {
				logger.Info("Precalc disabled by admin flag; skipping this run")
				continue
			}
			// Scheduled runs use incremental mode by default
			if err := runOnce(ctx, dbConn, queries, graphService, false, false, false); err != nil {
				logger.Error("Scheduled graph calculation/publication run failed", "error", err)
			}
		}
	}
}

func positiveIntEnv(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func forceClearEnabled() bool {
	value := os.Getenv("PRECALC_FORCE_CLEAR")
	return value == "1" || value == "true"
}

func effectiveFullRebuild(requested, forceClear bool) bool {
	return requested || forceClear
}

func validateRunOptions(publishOnly, forceClear bool) error {
	if forceClear && publishOnly {
		return fmt.Errorf("PRECALC_FORCE_CLEAR cannot be combined with --publish-only")
	}
	return nil
}

// hasMinSubredditsWithPosts returns true if at least `min` distinct subreddits have posts stored
func hasMinSubredditsWithPosts(ctx context.Context, dbc *sql.DB, min int) (bool, int, error) {
	var cnt int
	err := dbc.QueryRowContext(ctx, "SELECT COUNT(DISTINCT subreddit_id) FROM posts").Scan(&cnt)
	if err != nil {
		return false, 0, fmt.Errorf("count distinct subreddit_id in posts failed: %w", err)
	}
	return cnt >= min, cnt, nil
}

func runOnce(ctx context.Context, dbc *sql.DB, queries *db.Queries, graphService *graph.Service, fullRebuild, publishOnly, fullCatalog bool) error {
	sourceWatermark := time.Now().UTC()
	// Defer precalc until at least two subreddits have been crawled (i.e., produced posts)
	if ok, cnt, err := hasMinSubredditsWithPosts(ctx, dbc, 2); err != nil {
		return fmt.Errorf("precalculation readiness check: %w", err)
	} else if !ok {
		return fmt.Errorf("precalculation deferred: only %d subreddits have posts; require 2", cnt)
	}
	if !fullCatalog {
		if _, err := graph.RequirePublishedSpatialCatalog(ctx, dbc); err != nil {
			return fmt.Errorf("graph-only publication prerequisite: %w", err)
		}
	}
	if !publishOnly {
		if err := graphService.PrecalculateGraphDataWithMode(ctx, fullRebuild); err != nil {
			errorreporting.CaptureError(err)
			return fmt.Errorf("precalculate graph data: %w", err)
		}
	} else {
		logger.Info("Publishing existing graph workspace without recalculation")
	}
	if fullCatalog {
		catalogID, err := graph.BuildSpatialCatalogAtWatermark(ctx, dbc, sourceWatermark)
		if err != nil {
			return fmt.Errorf("build full spatial catalog: %w", err)
		}
		logger.Info("Published full spatial catalog", "catalog_id", catalogID)
	}
	// Publication is separate from the mutable build workspace.  The publisher
	// stages a complete immutable snapshot and swaps its current pointer atomically;
	// a publication failure leaves the last explorer revision intact.
	if revisionID, err := graph.PublishRevisionAtWatermark(ctx, dbc, sourceWatermark); err != nil {
		return fmt.Errorf("publish immutable graph revision: %w", err)
	} else {
		logger.Info("Published immutable graph revision", "revision_id", revisionID)
	}
	logger.Info("Graph data precalculated successfully")
	return nil
}
