package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/crawler"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/errorreporting"
	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
	"github.com/onnwee/reddit-cluster-map/backend/internal/migrations"
	"github.com/onnwee/reddit-cluster-map/backend/internal/scheduler"
	"github.com/onnwee/reddit-cluster-map/backend/internal/tracing"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize structured logging
	logger.Init(cfg.LogLevel)
	logger.Info("Initializing crawler", "version", cfg.SentryRelease, "log_level", cfg.LogLevel)

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
	shutdownTracing, err := tracing.Init("reddit-cluster-map-crawler")
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

	// Validate OAuth credentials at startup
	logger.Info("Validating OAuth credentials...")
	if err := crawler.ValidateOAuthCredentials(); err != nil {
		logger.Error("OAuth credential validation failed", "error", err)
		log.Fatalf("OAuth credential validation failed: %v", err)
	}
	logger.Info("OAuth credentials validated successfully")

	// Get database connection string from environment
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		logger.Error("DATABASE_URL environment variable is required")
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Connect to database
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close()

	// Configure connection pool for crawler (fewer connections needed than API)
	conn.SetMaxOpenConns(10)                  // Crawler needs fewer concurrent connections
	conn.SetMaxIdleConns(5)                   // Keep some idle connections ready
	conn.SetConnMaxLifetime(10 * time.Minute) // Longer lifetime for background service
	conn.SetConnMaxIdleTime(5 * time.Minute)  // Longer idle time for background service

	// Verify connection is working
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.PingContext(ctx); err != nil {
			logger.Error("Failed to ping database", "error", err)
			log.Fatalf("Failed to ping database: %v", err)
		}
		if err := migrations.VerifyCurrent(ctx, conn); err != nil {
			logger.Error("Database schema is incompatible", "error", err)
			log.Fatal(err)
		}
		logger.Info("Database connection established")
	}

	// Create database queries
	queries := db.New(conn)

	// Create crawler instance
	c := crawler.NewCrawler(queries)

	// Create scheduler instance
	s := scheduler.NewService(queries)

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
		c.Stop()
		s.Stop()
	}()

	// Start the scheduler in a separate goroutine
	go s.Start(ctx)

	// Start the crawler (blocks until context is cancelled)
	c.Start(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	logger.Info("Shutting down crawler")
}
