package config

import (
	"os"
	"strings"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
)

// Config holds application configuration derived from environment variables.
type Config struct {
	UserAgent          string
	HTTPMaxRetries     int
	HTTPRetryBase      time.Duration
	HTTPTimeout        time.Duration
	LogHTTPRetries     bool
	GraphQueryTimeout  time.Duration
	DBStatementTimeout time.Duration
	DetailedGraph      bool
	PostsPerSubInGraph int
	CommentsPerPost    int
	MaxAuthorLinks     int
	MaxPostsPerSub     int
	PostsSort          string
	PostsTimeFilter    string
	// Reddit OAuth (user-auth) configuration
	RedditClientID     string
	RedditClientSecret string
	RedditRedirectURI  string
	RedditScopes       string
	// Crawler scheduling
	StaleDays             int
	ResetCrawlingAfterMin int
	// API background graph job control
	DisableAPIGraphJob bool
	// Admin API token for gating admin endpoints (Bearer token)
	AdminAPIToken string
	// Security settings
	RateLimitGlobal      float64  // requests per second globally
	RateLimitGlobalBurst int      // burst size for global rate limit
	RateLimitPerIP       float64  // requests per second per IP
	RateLimitPerIPBurst  int      // burst size for per-IP rate limit
	CORSAllowedOrigins   []string // allowed CORS origins
	EnableRateLimit      bool     // enable rate limiting middleware
	// Crawler rate limiting (Reddit API)
	CrawlerRPS                 float64 // requests per second to Reddit API
	CrawlerBurstSize           int     // burst size for crawler rate limit
	CrawlLeaseDuration         time.Duration
	CrawlHeartbeatInterval     time.Duration
	CrawlMaxAttempts           int
	CrawlFreshness             time.Duration
	BacklogActivationLimit     int
	BacklogActivationWindow    time.Duration
	DiscoveryAuthorBudget      int
	DiscoveryCandidateBudget   int
	DiscoveryDailyBudget       int
	DiscoveryReconsiderHorizon time.Duration
	PublicationInterval        time.Duration
	PublicationMaxAge          time.Duration
	RevisionReadsEnabled       bool
	CrawlerLifecycleEnabled    bool
	RendererManifestVersion    string
	// Layout computation settings
	LayoutMaxNodes   int     // maximum nodes to include in layout computation
	LayoutIterations int     // number of force-directed iterations
	LayoutBatchSize  int     // batch size for position updates
	LayoutEpsilon    float64 // minimum distance threshold for position updates (0 = update all)
	LayoutTheta      float64 // Barnes-Hut theta parameter (0.0 = exact, 0.8 = standard approximation)
	// Observability settings
	LogLevel          string  // log level: debug, info, warn, error
	OTELEnabled       bool    // enable OpenTelemetry tracing
	OTELEndpoint      string  // OpenTelemetry collector endpoint
	OTELSampleRate    float64 // trace sampling rate (0.0 to 1.0)
	SentryDSN         string  // Sentry DSN for error reporting
	SentryEnvironment string  // Sentry environment (dev, staging, production)
	SentryRelease     string  // Sentry release version
	SentrySampleRate  float64 // Sentry error sampling rate (0.0 to 1.0)
	// Performance profiling
	EnableProfiling bool // enable pprof endpoints for performance profiling
	// Cache settings
	CacheMaxSizeMB  int64         // maximum cache size in megabytes
	CacheMaxEntries int64         // maximum number of cache entries
	CacheTTL        time.Duration // default time-to-live for cache entries
}

var cached *Config

// Load reads env vars once and caches them.
func Load() *Config {
	if cached != nil {
		return cached
	}
	ua := os.Getenv("REDDIT_USER_AGENT")
	if strings.TrimSpace(ua) == "" {
		ua = "reddit-cluster-map/0.1"
	}
	cached = &Config{
		UserAgent:             ua,
		HTTPMaxRetries:        utils.GetEnvAsInt("HTTP_MAX_RETRIES", 3),
		HTTPRetryBase:         time.Duration(utils.GetEnvAsInt("HTTP_RETRY_BASE_MS", 300)) * time.Millisecond,
		HTTPTimeout:           time.Duration(utils.GetEnvAsInt("HTTP_TIMEOUT_MS", 15000)) * time.Millisecond,
		LogHTTPRetries:        utils.GetEnvAsBool("LOG_HTTP_RETRIES", false),
		GraphQueryTimeout:     time.Duration(utils.GetEnvAsInt("GRAPH_QUERY_TIMEOUT_MS", 5000)) * time.Millisecond,
		DBStatementTimeout:    time.Duration(utils.GetEnvAsInt("DB_STATEMENT_TIMEOUT_MS", 5000)) * time.Millisecond,
		DetailedGraph:         utils.GetEnvAsBool("DETAILED_GRAPH", false),
		PostsPerSubInGraph:    utils.GetEnvAsInt("POSTS_PER_SUB_IN_GRAPH", 10),
		CommentsPerPost:       utils.GetEnvAsInt("COMMENTS_PER_POST_IN_GRAPH", 50),
		MaxAuthorLinks:        utils.GetEnvAsInt("MAX_AUTHOR_CONTENT_LINKS", 3),
		MaxPostsPerSub:        utils.GetEnvAsInt("MAX_POSTS_PER_SUB", 25),
		PostsSort:             strings.ToLower(strings.TrimSpace(os.Getenv("POSTS_SORT"))),
		PostsTimeFilter:       strings.ToLower(strings.TrimSpace(os.Getenv("POSTS_TIME_FILTER"))),
		RedditClientID:        strings.TrimSpace(os.Getenv("REDDIT_CLIENT_ID")),
		RedditClientSecret:    strings.TrimSpace(os.Getenv("REDDIT_CLIENT_SECRET")),
		RedditRedirectURI:     strings.TrimSpace(os.Getenv("REDDIT_REDIRECT_URI")),
		RedditScopes:          strings.TrimSpace(os.Getenv("REDDIT_SCOPES")),
		StaleDays:             utils.GetEnvAsInt("STALE_DAYS", 30),
		ResetCrawlingAfterMin: utils.GetEnvAsInt("RESET_CRAWLING_AFTER_MIN", 15),
		DisableAPIGraphJob:    utils.GetEnvAsBool("DISABLE_API_GRAPH_JOB", false),
		AdminAPIToken:         strings.TrimSpace(os.Getenv("ADMIN_API_TOKEN")),
		// Security settings with sensible defaults
		RateLimitGlobal:      utils.GetEnvAsFloat("RATE_LIMIT_GLOBAL", 100.0),
		RateLimitGlobalBurst: utils.GetEnvAsInt("RATE_LIMIT_GLOBAL_BURST", 200),
		RateLimitPerIP:       utils.GetEnvAsFloat("RATE_LIMIT_PER_IP", 10.0),
		RateLimitPerIPBurst:  utils.GetEnvAsInt("RATE_LIMIT_PER_IP_BURST", 20),
		EnableRateLimit:      utils.GetEnvAsBool("ENABLE_RATE_LIMIT", true),
		// Crawler rate limiting: default to ~1.66 rps (60 requests per minute)
		CrawlerRPS:             utils.GetEnvAsFloat("CRAWLER_RPS", 1.66),
		CrawlerBurstSize:       utils.GetEnvAsInt("CRAWLER_BURST_SIZE", 1),
		CrawlLeaseDuration:     durationEnv("CRAWL_LEASE_DURATION", 2*time.Minute),
		CrawlHeartbeatInterval: durationEnv("CRAWL_HEARTBEAT_INTERVAL", 30*time.Second),
		CrawlMaxAttempts:       utils.GetEnvAsInt("CRAWL_MAX_ATTEMPTS", 3),
		CrawlFreshness:         durationEnv("CRAWL_FRESHNESS", 7*24*time.Hour),
		// Imported backlog is intentionally throttled even when the lifecycle is
		// enabled. Explicit manual/API work uses the active cohort instead.
		BacklogActivationLimit:     utils.GetEnvAsInt("CRAWL_BACKLOG_ACTIVATION_LIMIT", 24),
		BacklogActivationWindow:    durationEnv("CRAWL_BACKLOG_ACTIVATION_WINDOW", time.Hour),
		DiscoveryAuthorBudget:      utils.GetEnvAsInt("DISCOVERY_AUTHOR_BUDGET", 25),
		DiscoveryCandidateBudget:   utils.GetEnvAsInt("DISCOVERY_CANDIDATE_BUDGET", 50),
		DiscoveryDailyBudget:       utils.GetEnvAsInt("DISCOVERY_DAILY_BUDGET", 500),
		DiscoveryReconsiderHorizon: durationEnv("DISCOVERY_RECONSIDER_HORIZON", 30*24*time.Hour),
		PublicationInterval:        durationEnv("PUBLICATION_INTERVAL", time.Hour),
		PublicationMaxAge:          durationEnv("PUBLICATION_MAX_AGE", 2*time.Hour),
		RevisionReadsEnabled:       utils.GetEnvAsBool("REVISION_READS_ENABLED", true),
		CrawlerLifecycleEnabled:    utils.GetEnvAsBool("CRAWLER_LIFECYCLE_ENABLED", true),
		RendererManifestVersion:    strings.TrimSpace(os.Getenv("RENDERER_MANIFEST_VERSION")),
		// Layout computation: sensible defaults for force-directed layout
		LayoutMaxNodes:   utils.GetEnvAsInt("LAYOUT_MAX_NODES", 5000),
		LayoutIterations: utils.GetEnvAsInt("LAYOUT_ITERATIONS", 400),
		LayoutBatchSize:  utils.GetEnvAsInt("LAYOUT_BATCH_SIZE", 5000),
		LayoutEpsilon:    utils.GetEnvAsFloat("LAYOUT_EPSILON", 0.0),
		LayoutTheta:      utils.GetEnvAsFloat("LAYOUT_THETA", 0.8),
		// Observability settings
		LogLevel:          strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))),
		OTELEnabled:       utils.GetEnvAsBool("OTEL_ENABLED", false),
		OTELEndpoint:      strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		OTELSampleRate:    utils.GetEnvAsFloat("OTEL_TRACE_SAMPLE_RATE", 0.1),
		SentryDSN:         strings.TrimSpace(os.Getenv("SENTRY_DSN")),
		SentryEnvironment: strings.TrimSpace(os.Getenv("SENTRY_ENVIRONMENT")),
		SentryRelease:     strings.TrimSpace(os.Getenv("SENTRY_RELEASE")),
		SentrySampleRate:  utils.GetEnvAsFloat("SENTRY_SAMPLE_RATE", 1.0),
		// Performance profiling: disabled by default for security
		EnableProfiling: utils.GetEnvAsBool("ENABLE_PROFILING", false),
		// Cache settings: sensible defaults for graph API caching
		CacheMaxSizeMB:  int64(utils.GetEnvAsInt("CACHE_MAX_SIZE_MB", 512)),
		CacheMaxEntries: int64(utils.GetEnvAsInt("CACHE_MAX_ENTRIES", 10000)),
		CacheTTL:        time.Duration(utils.GetEnvAsInt("CACHE_TTL_SECONDS", 60)) * time.Second,
	}
	if cached.PostsSort == "" {
		cached.PostsSort = "top"
	}
	if cached.PostsTimeFilter == "" {
		cached.PostsTimeFilter = "day"
	}
	if cached.LogLevel == "" {
		cached.LogLevel = "info"
	}
	if cached.SentryEnvironment == "" {
		if env := os.Getenv("ENV"); env != "" {
			cached.SentryEnvironment = env
		} else {
			cached.SentryEnvironment = "development"
		}
	}
	if cached.RendererManifestVersion == "" {
		cached.RendererManifestVersion = "spatial-scene-v1"
	}
	if cached.CrawlMaxAttempts < 1 {
		cached.CrawlMaxAttempts = 3
	}
	if cached.BacklogActivationLimit < 1 {
		cached.BacklogActivationLimit = 24
	}
	if cached.CrawlHeartbeatInterval >= cached.CrawlLeaseDuration {
		cached.CrawlHeartbeatInterval = cached.CrawlLeaseDuration / 4
	}

	// Parse CORS allowed origins
	corsOrigins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if corsOrigins == "" {
		// Default to common development origins
		cached.CORSAllowedOrigins = []string{"http://localhost:5173", "http://localhost:3000"}
	} else {
		cached.CORSAllowedOrigins = strings.Split(corsOrigins, ",")
		for i := range cached.CORSAllowedOrigins {
			cached.CORSAllowedOrigins[i] = strings.TrimSpace(cached.CORSAllowedOrigins[i])
		}
	}

	return cached
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// ResetForTest clears cached config; for use in tests only.
func ResetForTest() { cached = nil }

// GetEnvBool reads a boolean environment variable with a default.
// Use this when you need to check a flag not present in the cached config.
func (c *Config) GetEnvBool(key string, def bool) bool {
	return utils.GetEnvAsBool(key, def)
}

// GetEnvInt reads an integer environment variable with a default.
// Use this when you need to check a value not present in the cached config.
func (c *Config) GetEnvInt(key string, def int) int {
	return utils.GetEnvAsInt(key, def)
}
