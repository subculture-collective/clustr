package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
)

// Health returns a simple JSON payload to indicate the API is alive.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type readinessDB interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

// Readiness verifies the durable contracts needed to serve the spatial client.
// /health remains a pure liveness probe so migration failures stay diagnosable.
func Readiness(database readinessDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := config.Load()
		response := struct {
			Status      string            `json:"status"`
			Checks      map[string]string `json:"checks"`
			RevisionID  *int64            `json:"revision_id,omitempty"`
			PublishedAt *time.Time        `json:"published_at,omitempty"`
		}{Status: "ready", Checks: map[string]string{}}

		var migrationsOK bool
		err := database.QueryRowContext(r.Context(), `SELECT to_regclass('public.clustr_schema_migrations') IS NOT NULL AND EXISTS (SELECT 1 FROM clustr_schema_migrations WHERE filename='000029_graph_revisions.up.sql')`).Scan(&migrationsOK)
		if err != nil || !migrationsOK {
			response.Status = "not_ready"
			response.Checks["schema"] = "migration 000029 is not recorded"
		} else {
			response.Checks["schema"] = "current"
		}

		if cfg.RevisionReadsEnabled {
			var id int64
			var published time.Time
			err = database.QueryRowContext(r.Context(), `SELECT r.id,r.published_at FROM graph_revision_current c JOIN graph_revisions r ON r.id=c.revision_id WHERE c.singleton AND r.status='published'`).Scan(&id, &published)
			if err != nil {
				response.Status = "not_ready"
				response.Checks["publication"] = "no active published revision"
			} else {
				response.RevisionID = &id
				response.PublishedAt = &published
				age := time.Since(published)
				if age > cfg.PublicationMaxAge {
					response.Status = "not_ready"
					response.Checks["publication"] = "active revision is stale"
				} else {
					response.Checks["publication"] = "current"
				}
			}
		} else {
			response.Checks["publication"] = "revision reads disabled"
		}

		var expired int
		if err = database.QueryRowContext(r.Context(), `SELECT count(*) FROM crawl_attempts WHERE state='leased' AND lease_expires_at < now()`).Scan(&expired); err != nil || expired > 0 {
			response.Status = "not_ready"
			response.Checks["crawl_leases"] = "expired leases require recovery"
		} else {
			response.Checks["crawl_leases"] = "healthy"
		}

		status := http.StatusOK
		if response.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}
}

// EffectiveConfig exposes validated non-secret runtime values to operators.
func EffectiveConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := config.Load()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"crawler": map[string]any{
			"lifecycle_enabled":          cfg.CrawlerLifecycleEnabled,
			"lease":                      cfg.CrawlLeaseDuration.String(),
			"heartbeat":                  cfg.CrawlHeartbeatInterval.String(),
			"max_attempts":               cfg.CrawlMaxAttempts,
			"freshness":                  cfg.CrawlFreshness.String(),
			"discovery_author_budget":    cfg.DiscoveryAuthorBudget,
			"discovery_candidate_budget": cfg.DiscoveryCandidateBudget,
			"discovery_daily_budget":     cfg.DiscoveryDailyBudget,
			"reconsider_horizon":         cfg.DiscoveryReconsiderHorizon.String(),
		},
		"publication": map[string]any{"interval": cfg.PublicationInterval.String(), "max_age": cfg.PublicationMaxAge.String(), "revision_reads_enabled": cfg.RevisionReadsEnabled},
		"layout":      map[string]any{"max_nodes": cfg.LayoutMaxNodes, "iterations": cfg.LayoutIterations, "theta": cfg.LayoutTheta},
		"renderer":    map[string]any{"manifest_version": cfg.RendererManifestVersion},
	})
}
