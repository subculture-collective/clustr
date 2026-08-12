package crawler

import (
	"context"
	"sync"

	"golang.org/x/time/rate"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
)

var (
	limiter     *rate.Limiter
	limiterOnce sync.Once
)

// initLimiter creates the rate limiter based on config
func initLimiter() {
	cfg := config.Load()
	// Create token bucket rate limiter with configured RPS and burst
	limiter = rate.NewLimiter(rate.Limit(cfg.CrawlerRPS), cfg.CrawlerBurstSize)
}

// getLimiter returns the singleton rate limiter instance
func getLimiter() *rate.Limiter {
	limiterOnce.Do(initLimiter)
	return limiter
}

// waitForRateLimit blocks until a token is available from the rate limiter
func waitForRateLimitContext(ctx context.Context) error {
	if err := getLimiter().Wait(ctx); err != nil {
		return err
	}
	metrics.CrawlerRateLimitWaits.Inc()
	return nil
}

// waitForRateLimit remains for older callers. New collection paths must pass
// their crawl context so shutdown can interrupt pacing.
func waitForRateLimit() {
	_ = waitForRateLimitContext(context.Background())
}

// ResetLimiterForTest resets the rate limiter singleton for testing
func ResetLimiterForTest() {
	limiterOnce = sync.Once{}
	limiter = nil
}
