package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/onnwee/reddit-cluster-map/backend/internal/api"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/errorreporting"
	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
	"github.com/onnwee/reddit-cluster-map/backend/internal/server"
	"github.com/onnwee/reddit-cluster-map/backend/internal/tracing"
)

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	// Load configuration
	cfg := config.Load()

	// Initialize structured logging
	logger.Init(cfg.LogLevel)
	logger.Info("Initializing API server", "version", cfg.SentryRelease, "log_level", cfg.LogLevel)

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
	shutdownTracing, err := tracing.Init("reddit-cluster-map-api")
	if err != nil {
		logger.Warn("Failed to initialize tracing", "error", err)
	} else if cfg.OTELEnabled {
		logger.Info("Tracing initialized", "endpoint", cfg.OTELEndpoint, "sample_rate", cfg.OTELSampleRate)
		defer func() {
			logger.Info("Shutting down tracer...")
			if err := shutdownTracing(ctx); err != nil {
				logger.Error("Failed to shutdown tracer", "error", err)
			}
		}()
	}

	queries, err := server.InitDB()
	if err != nil {
		logger.Error("DB init failed", "error", err)
		log.Fatalf("❌ DB init failed: %v", err)
	}

	srv := server.NewServer(queries)
	if err := srv.Start(ctx); err != nil {
		logger.Error("Server start failed", "error", err)
		log.Fatalf("❌ Server start failed: %v", err)
	}

	router := api.NewRouter(queries)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8000"
	}
	address := net.JoinHostPort("", port)
	logger.Info("Server running", "address", address, "url", "http://localhost:"+port)
	log.Fatal(http.ListenAndServe(address, router))
}
