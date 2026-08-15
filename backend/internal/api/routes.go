package api

import (
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/onnwee/reddit-cluster-map/backend/internal/api/handlers"
	"github.com/onnwee/reddit-cluster-map/backend/internal/apierr"
	"github.com/onnwee/reddit-cluster-map/backend/internal/cache"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
	"github.com/onnwee/reddit-cluster-map/backend/internal/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(q *db.Queries) *mux.Router {
	// Create the root router. All routes below are relative to this router.
	r := mux.NewRouter()

	// Load configuration
	cfg := config.Load()
	if cfg.CatalogEnabled && strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") && len(os.Getenv("CATALOG_CURSOR_SECRET")) < 32 {
		panic("CATALOG_CURSOR_SECRET must be at least 32 characters when Catalog is enabled in production")
	}

	// Initialize LRU cache
	graphCache, err := cache.NewLRU(cfg.CacheMaxSizeMB, cfg.CacheMaxEntries, cfg.CacheTTL)
	if err != nil {
		// If cache initialization fails, panic since this is a critical component
		panic("Failed to initialize cache: " + err.Error())
	}

	// Start background cache metrics collector
	go collectCacheMetrics(graphCache)

	// Apply global middleware in order
	// 1. Request ID middleware (first to track all requests)
	r.Use(middleware.RequestID)

	// 2. Security headers
	r.Use(middleware.SecurityHeaders)

	// 3. CORS middleware
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}
	r.Use(middleware.CORS(corsConfig))

	// 4. Request body validation
	r.Use(middleware.ValidateRequestBody)

	// 5. Error recovery middleware
	r.Use(middleware.RecoverWithSentry)

	// 6. Rate limiting (if enabled)
	if cfg.EnableRateLimit {
		rateLimiter := middleware.NewRateLimiter(
			cfg.RateLimitGlobal,
			cfg.RateLimitGlobalBurst,
			cfg.RateLimitPerIP,
			cfg.RateLimitPerIPBurst,
		)
		r.Use(rateLimiter.Limit)
	}

	// Lightweight healthcheck: GET /health -> {"status":"ok"}
	r.HandleFunc("/health", handlers.Health).Methods("GET")
	// Public ingress preserves the /api prefix. Keep explicit aliases so
	// readiness does not depend on proxy-specific path rewriting.
	r.HandleFunc("/api/health", handlers.Health).Methods("GET")
	if q != nil {
		readiness := handlers.Readiness(q.DB())
		r.Handle("/ready", readiness).Methods("GET")
		r.Handle("/api/ready", readiness).Methods("GET")
	}
	r.HandleFunc("/api/config/effective", handlers.EffectiveConfig).Methods("GET")

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// OAuth login/callback for Reddit user authorization
	auth := handlers.NewAuthHandlers(q)
	r.HandleFunc("/auth/login", auth.Login).Methods("GET")
	r.HandleFunc("/auth/callback", auth.Callback).Methods("GET")

	// Alternate paths to support externally configured redirect URIs
	// e.g., https://<host>/oauth/reddit/callback used in production
	r.HandleFunc("/oauth/reddit/login", auth.Login).Methods("GET")
	r.HandleFunc("/oauth/reddit/callback", auth.Callback).Methods("GET")

	// REST-style resources (no /api prefix) for internal/admin queries
	// Subreddits list: GET /subreddits?limit=&offset=
	r.HandleFunc("/subreddits", handlers.GetSubreddits(q)).Methods("GET")

	// Explicit API endpoints with /api prefix intended for frontend use
	// Enqueue a subreddit crawl: POST /api/crawl {"subreddit":"name"}
	r.HandleFunc("/api/crawl", handlers.PostCrawl(q)).Methods("POST")

	// Users list: GET /users?limit=&offset=
	r.HandleFunc("/users", handlers.GetUsers(q)).Methods("GET")

	// Posts by subreddit: GET /posts?subreddit_id=&limit=&offset=
	r.HandleFunc("/posts", handlers.GetPosts(q)).Methods("GET")

	// Comments by post: GET /comments?post_id=
	r.HandleFunc("/comments", handlers.GetComments(q)).Methods("GET")

	// Crawl jobs list: GET /jobs?limit=&offset=
	r.HandleFunc("/jobs", handlers.GetCrawlJobs(q)).Methods("GET")

	// Graph data for the frontend: GET /api/graph
	graphHandler := handlers.NewHandler(q, graphCache)
	r.Handle("/api/graph", middleware.Gzip(middleware.ETag(http.HandlerFunc(graphHandler.GetGraphData)))).Methods("GET")
	// Immutable explorer publication routes. revision_id is optional and defaults to
	// the current published snapshot; all data in a response is revision-scoped.
	if q != nil && cfg.RevisionReadsEnabled { // permits isolated router-security tests that intentionally use no database.
		revisionHandler := handlers.NewRevisionHandler(q.DB())
		r.Handle("/api/graph/manifest", middleware.Gzip(http.HandlerFunc(revisionHandler.Manifest))).Methods("GET")
		r.Handle("/api/graph/telemetry", middleware.Gzip(http.HandlerFunc(revisionHandler.Telemetry))).Methods("GET")
		r.Handle("/api/graph/communities", middleware.Gzip(http.HandlerFunc(revisionHandler.Communities))).Methods("GET")
		if cfg.CatalogEnabled {
			r.Handle("/api/graph/catalog", middleware.Gzip(http.HandlerFunc(revisionHandler.Catalog))).Methods("GET")
			r.Handle("/api/graph/catalog/facets", middleware.Gzip(http.HandlerFunc(revisionHandler.CatalogFacets))).Methods("GET")
			r.Handle("/api/graph/catalog/tree/{id}", middleware.Gzip(http.HandlerFunc(revisionHandler.CatalogTree))).Methods("GET")
			r.Handle("/api/graph/macro-groups", middleware.Gzip(http.HandlerFunc(revisionHandler.MacroGroups))).Methods("GET")
		}
		r.Handle("/api/graph/overview", middleware.Gzip(http.HandlerFunc(revisionHandler.Overview))).Methods("GET")
		r.Handle("/api/graph/region", middleware.Gzip(http.HandlerFunc(revisionHandler.Region))).Methods("GET")
		r.Handle("/api/graph/community/{stable_id}", middleware.Gzip(http.HandlerFunc(revisionHandler.Community))).Methods("GET")
		r.Handle("/api/graph/entity/{id}", middleware.Gzip(http.HandlerFunc(revisionHandler.Entity))).Methods("GET")
		r.Handle("/api/graph/diff", middleware.Gzip(http.HandlerFunc(revisionHandler.Diff))).Methods("GET")
		r.Handle("/api/search", middleware.Gzip(middleware.ETag(http.HandlerFunc(revisionHandler.Search)))).Methods("GET")
		r.Handle("/api/nodes/{id}", middleware.Gzip(middleware.ETag(http.HandlerFunc(revisionHandler.NodeDetails)))).Methods("GET")
	}
	if q != nil && cfg.CatalogEnabled {
		catalogPages := handlers.NewCatalogPageHandler(q.DB())
		r.HandleFunc("/catalog/{type:clusters|subreddits|users|posts|comments}/{id}", catalogPages.Entity).Methods("GET")
		r.HandleFunc("/sitemap.xml", catalogPages.SitemapIndex).Methods("GET")
		r.HandleFunc("/sitemaps/{tier:[1-3]}/{shard:[0-9]+}.xml", catalogPages.SitemapShard).Methods("GET")
	}

	// Compatibility names for the previous mutable tiered graph during rollback.
	r.Handle("/api/graph/legacy-overview", middleware.Gzip(middleware.ETag(http.HandlerFunc(graphHandler.GetGraphOverview)))).Methods("GET")
	r.Handle("/api/graph/legacy-region", middleware.Gzip(middleware.ETag(http.HandlerFunc(graphHandler.GetGraphRegion)))).Methods("GET")
	if q == nil || !cfg.RevisionReadsEnabled {
		r.Handle("/api/graph/overview", middleware.Gzip(middleware.ETag(http.HandlerFunc(graphHandler.GetGraphOverview)))).Methods("GET")
		r.Handle("/api/graph/region", middleware.Gzip(middleware.ETag(http.HandlerFunc(graphHandler.GetGraphRegion)))).Methods("GET")
	}

	// Edge bundles endpoint with gzip and ETag: GET /api/graph/bundles
	r.Handle("/api/graph/bundles", middleware.Gzip(middleware.ETag(http.HandlerFunc(graphHandler.GetEdgeBundles)))).Methods("GET")

	// Graph versioning endpoints
	versionHandler := handlers.NewVersionHandler(q, graphCache)
	r.Handle("/api/graph/version", middleware.Gzip(http.HandlerFunc(versionHandler.GetCurrentVersion))).Methods("GET")
	r.Handle("/api/graph/legacy-diff", middleware.Gzip(http.HandlerFunc(versionHandler.GetDiffSince))).Methods("GET")
	if q == nil || !cfg.RevisionReadsEnabled {
		r.Handle("/api/graph/diff", middleware.Gzip(http.HandlerFunc(versionHandler.GetDiffSince))).Methods("GET")
	}

	// Search endpoint with gzip and ETag: GET /api/search?node=...
	if q == nil || !cfg.RevisionReadsEnabled {
		searchHandler := middleware.Gzip(middleware.ETag(http.HandlerFunc(handlers.SearchNode(q))))
		r.Handle("/api/search", searchHandler).Methods("GET")
	}

	// Node details endpoint: GET /api/nodes/{id}
	if q == nil || !cfg.RevisionReadsEnabled {
		nodeDetailsHandler := middleware.Gzip(middleware.ETag(http.HandlerFunc(handlers.GetNodeDetails(q))))
		r.Handle("/api/nodes/{id}", nodeDetailsHandler).Methods("GET")
	}

	// Export endpoint with gzip and ETag: GET /api/export?format=json|csv
	exportHandler := middleware.Gzip(middleware.ETag(http.HandlerFunc(handlers.ExportGraph(q))))
	r.Handle("/api/export", exportHandler).Methods("GET")

	// Community aggregation endpoints with gzip and ETag
	communityHandler := handlers.NewCommunityHandler(q, graphCache)
	r.Handle("/api/communities", middleware.Gzip(middleware.ETag(http.HandlerFunc(communityHandler.GetCommunities)))).Methods("GET")
	r.Handle("/api/communities/{id}", middleware.Gzip(middleware.ETag(http.HandlerFunc(communityHandler.GetCommunityByID)))).Methods("GET")
	if !cfg.RevisionReadsEnabled {
		r.Handle("/api/graph/community/{id}", middleware.Gzip(middleware.ETag(http.HandlerFunc(communityHandler.GetCommunityByID)))).Methods("GET")
	}

	// Admin: toggle background services (gated)
	admin := handlers.NewAdminHandler(q)
	// We'll define adminOnly below, so temporarily register after it's declared.
	// precalc is handled in its own service; no run-now route
	// Crawl status
	r.HandleFunc("/api/crawl/status", handlers.GetCrawlStatus(q)).Methods("GET")

	// Admin auth middleware using a static bearer token from env
	adminOnly := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.AdminAPIToken == "" {
				apierr.WriteErrorWithContext(w, r, apierr.SystemUnavailable("Admin token not configured"))
				return
			}
			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix || auth[len(prefix):] != cfg.AdminAPIToken {
				apierr.WriteErrorWithContext(w, r, apierr.AuthInvalid(""))
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// Backups (read-only): list and download files from mounted volume, gated by adminOnly
	r.Handle("/api/admin/backups", adminOnly(http.HandlerFunc(handlers.ListBackups))).Methods("GET")
	r.Handle("/api/admin/backups/{name}", adminOnly(http.HandlerFunc(handlers.DownloadBackup))).Methods("GET")
	// Services endpoints gated by adminOnly
	r.Handle("/api/admin/services", adminOnly(http.HandlerFunc(admin.GetServices))).Methods("GET")
	r.Handle("/api/admin/services", adminOnly(http.HandlerFunc(admin.UpdateServices))).Methods("POST")
	// User token refresh endpoint (admin-only for security)
	r.Handle("/api/auth/refresh", adminOnly(http.HandlerFunc(auth.RefreshUserToken))).Methods("POST")

	// Admin job management endpoints
	adminJobs := handlers.NewAdminJobsHandler(q)
	r.Handle("/api/admin/jobs/stats", adminOnly(http.HandlerFunc(adminJobs.GetJobStats))).Methods("GET")
	r.Handle("/api/admin/jobs", adminOnly(http.HandlerFunc(adminJobs.ListJobsByStatus))).Methods("GET")
	r.Handle("/api/admin/jobs/{id}/status", adminOnly(http.HandlerFunc(adminJobs.UpdateJobStatus))).Methods("PUT")
	r.Handle("/api/admin/jobs/{id}/priority", adminOnly(http.HandlerFunc(adminJobs.UpdateJobPriority))).Methods("PUT")
	r.Handle("/api/admin/jobs/{id}/retry", adminOnly(http.HandlerFunc(adminJobs.RetryJob))).Methods("POST")
	r.Handle("/api/admin/jobs/{id}/boost", adminOnly(http.HandlerFunc(adminJobs.BoostJobPriority))).Methods("POST")
	r.Handle("/api/admin/jobs/bulk/status", adminOnly(http.HandlerFunc(adminJobs.BulkUpdateJobStatus))).Methods("PUT")
	r.Handle("/api/admin/jobs/bulk/retry", adminOnly(http.HandlerFunc(adminJobs.BulkRetryJobs))).Methods("POST")

	// Admin scheduled job management endpoints
	scheduledJobs := handlers.NewScheduledJobsHandler(q)
	r.Handle("/api/admin/scheduled-jobs", adminOnly(http.HandlerFunc(scheduledJobs.ListScheduledJobs))).Methods("GET")
	r.Handle("/api/admin/scheduled-jobs", adminOnly(http.HandlerFunc(scheduledJobs.CreateScheduledJob))).Methods("POST")
	r.Handle("/api/admin/scheduled-jobs/{id}", adminOnly(http.HandlerFunc(scheduledJobs.GetScheduledJob))).Methods("GET")
	r.Handle("/api/admin/scheduled-jobs/{id}", adminOnly(http.HandlerFunc(scheduledJobs.UpdateScheduledJob))).Methods("PUT")
	r.Handle("/api/admin/scheduled-jobs/{id}", adminOnly(http.HandlerFunc(scheduledJobs.DeleteScheduledJob))).Methods("DELETE")
	r.Handle("/api/admin/scheduled-jobs/{id}/toggle", adminOnly(http.HandlerFunc(scheduledJobs.ToggleScheduledJob))).Methods("POST")

	// Admin settings endpoints
	adminSettings := handlers.NewAdminSettingsHandler(q)
	r.Handle("/api/admin/settings", adminOnly(http.HandlerFunc(adminSettings.GetSettings))).Methods("GET")
	r.Handle("/api/admin/settings", adminOnly(http.HandlerFunc(adminSettings.UpdateSettings))).Methods("PUT")
	r.Handle("/api/admin/audit-log", adminOnly(http.HandlerFunc(adminSettings.GetAuditLog))).Methods("GET")
	if q != nil {
		suppressions := handlers.NewSuppressionHandler(q.DB())
		r.Handle("/api/admin/suppressions", adminOnly(http.HandlerFunc(suppressions.List))).Methods("GET")
		r.Handle("/api/admin/suppressions", adminOnly(http.HandlerFunc(suppressions.Create))).Methods("POST")
		r.Handle("/api/admin/suppressions/restore", adminOnly(http.HandlerFunc(suppressions.Restore))).Methods("POST")
	}

	// Cache admin endpoints
	cacheAdmin := handlers.NewCacheAdminHandler(graphCache)
	r.Handle("/api/admin/cache/invalidate", adminOnly(http.HandlerFunc(cacheAdmin.InvalidateCache))).Methods("POST")
	r.Handle("/api/admin/cache/stats", adminOnly(http.HandlerFunc(cacheAdmin.GetCacheStats))).Methods("GET")

	// Performance profiling endpoints (admin-only for security)
	// These endpoints expose runtime profiling data for performance analysis
	if cfg.EnableProfiling {
		pprofRouter := r.PathPrefix("/debug/pprof").Subrouter()
		pprofRouter.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Log pprof access attempts for security monitoring
				handlers.LogPprofAccess(r.Context(), r.URL.Path, r.RemoteAddr)
				adminOnly(next).ServeHTTP(w, r)
			})
		})
		pprofRouter.HandleFunc("/", pprof.Index)
		pprofRouter.HandleFunc("/cmdline", pprof.Cmdline)
		pprofRouter.HandleFunc("/profile", pprof.Profile)
		pprofRouter.HandleFunc("/symbol", pprof.Symbol)
		pprofRouter.HandleFunc("/trace", pprof.Trace)
		pprofRouter.Handle("/goroutine", pprof.Handler("goroutine"))
		pprofRouter.Handle("/heap", pprof.Handler("heap"))
		pprofRouter.Handle("/threadcreate", pprof.Handler("threadcreate"))
		pprofRouter.Handle("/block", pprof.Handler("block"))
		pprofRouter.Handle("/mutex", pprof.Handler("mutex"))
		pprofRouter.Handle("/allocs", pprof.Handler("allocs"))
	}

	return r
}

// collectCacheMetrics periodically updates Prometheus metrics with cache statistics.
// This runs in a background goroutine for the lifetime of the application.
// The goroutine stops when the ticker is garbage collected (on process exit).
func collectCacheMetrics(c cache.Cache) {
	const interval = 15 // seconds
	ticker := time.NewTicker(interval * time.Second)
	defer ticker.Stop()

	var prevEvictions uint64
	var havePrevEvictions bool

	for range ticker.C {
		stats := c.Stats()
		// Update Prometheus gauges
		// We use "graph" as the endpoint label since the cache is shared
		metrics.APICacheSize.WithLabelValues("graph").Set(float64(stats.Size))
		metrics.APICacheItems.WithLabelValues("graph").Set(float64(stats.Items))

		// Evictions are exposed as a Prometheus counter. The cache reports a cumulative
		// eviction count, so we compute the delta since the last sample and add that.
		if !havePrevEvictions {
			prevEvictions = stats.Evictions
			havePrevEvictions = true
			continue
		}

		if stats.Evictions >= prevEvictions {
			delta := stats.Evictions - prevEvictions
			if delta > 0 {
				metrics.APICacheEvictions.WithLabelValues("graph").Add(float64(delta))
			}
		} else {
			// The underlying counter was reset (e.g., cache cleared or process restarted).
			// Treat the current value as a fresh cumulative count.
			if stats.Evictions > 0 {
				metrics.APICacheEvictions.WithLabelValues("graph").Add(float64(stats.Evictions))
			}
		}

		prevEvictions = stats.Evictions
	}
}
