package server

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/crawler"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/graph"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
	"github.com/onnwee/reddit-cluster-map/backend/internal/migrations"
)

type Server struct {
	DB               *db.Queries
	graphService     *graph.Service
	metricsCollector *metrics.Collector
}

func InitDB() (*db.Queries, error) {
	conn, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}

	// Configure connection pool for optimal performance
	// These settings prevent connection exhaustion and ensure efficient resource usage
	conn.SetMaxOpenConns(25)                 // Max concurrent connections (adjust based on load)
	conn.SetMaxIdleConns(10)                 // Max idle connections in pool
	conn.SetConnMaxLifetime(5 * time.Minute) // Recycle connections every 5 minutes
	conn.SetConnMaxIdleTime(2 * time.Minute) // Close idle connections after 2 minutes

	// Verify connection is working
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	if err := migrations.VerifyCurrent(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}

	// Check for position columns on startup
	checkPositionColumns(conn)

	return db.New(conn), nil
}

// checkPositionColumns verifies that graph_nodes table has position columns
func checkPositionColumns(conn *sql.DB) {
	if conn == nil {
		log.Printf("⚠️  Unable to check position columns: connection is nil")
		return
	}

	query := `
		SELECT 
			EXISTS (
				SELECT 1 
				FROM information_schema.columns 
				WHERE table_name = 'graph_nodes' 
				AND column_name = 'pos_x'
			) AS has_pos_x,
			EXISTS (
				SELECT 1 
				FROM information_schema.columns 
				WHERE table_name = 'graph_nodes' 
				AND column_name = 'pos_y'
			) AS has_pos_y,
			EXISTS (
				SELECT 1 
				FROM information_schema.columns 
				WHERE table_name = 'graph_nodes' 
				AND column_name = 'pos_z'
			) AS has_pos_z
	`

	var hasPosX, hasPosY, hasPosZ bool
	err := conn.QueryRow(query).Scan(&hasPosX, &hasPosY, &hasPosZ)
	if err != nil {
		log.Printf("⚠️  Unable to check position columns: %v", err)
		return
	}

	if hasPosX && hasPosY && hasPosZ {
		log.Println("✓ Position columns (pos_x, pos_y, pos_z) are present in graph_nodes table")
	} else {
		log.Printf("⚠️  Position columns missing in graph_nodes table: pos_x=%v, pos_y=%v, pos_z=%v", hasPosX, hasPosY, hasPosZ)
		log.Println("   Run migrations to add position columns: make migrate-up")
	}
}

func NewServer(q *db.Queries) *Server {
	graphService := graph.NewService(q)
	metricsCollector := metrics.NewCollector(q, 30*time.Second) // Collect metrics every 30 seconds

	return &Server{
		DB:               q,
		graphService:     graphService,
		metricsCollector: metricsCollector,
	}
}

func (s *Server) Start(ctx context.Context) error {
	// Start metrics collector
	go s.metricsCollector.Start(ctx)

	// The graph precalculation runs in a dedicated service now; API will not start it.
	// Seed default subreddits if queue is empty
	go func() {
		// tiny delay to allow DB init
		time.Sleep(2 * time.Second)
		// Try to fetch pending jobs; if none, enqueue a few defaults
		jobs, err := s.DB.ListQueueWithNames(ctx)
		if err == nil && len(jobs) == 0 {
			defaults := []string{"AskReddit", "worldnews", "technology"}
			for _, name := range defaults {
				id, err := s.DB.EnsureSubreddit(ctx, db.EnsureSubredditParams{
					Name:        name,
					Title:       sql.NullString{String: name, Valid: true},
					Description: sql.NullString{String: "", Valid: true},
					Subscribers: sql.NullInt32{Int32: 0, Valid: true},
				})
				if err == nil {
					_ = crawler.EnsureJob(ctx, s.DB, id, "startup")
				}
			}
		}
	}()
	return nil
}
