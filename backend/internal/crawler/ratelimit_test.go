package crawler

import (
	"os"
	"testing"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/config"
)

func TestRateLimiterDefault(t *testing.T) {
	// Reset config and limiter for clean test
	config.ResetForTest()
	ResetLimiterForTest()

	// Should use default ~1.66 rps
	start := time.Now()
	waitForRateLimit() // First call should be immediate
	waitForRateLimit() // Second call should wait

	elapsed := time.Since(start)
	expectedMin := 500 * time.Millisecond // Allow some tolerance
	if elapsed < expectedMin {
		t.Errorf("Expected rate limit to enforce ~600ms delay, got %v", elapsed)
	}

	// Clean up
	config.ResetForTest()
	ResetLimiterForTest()
}

func TestRateLimiterConfigurable(t *testing.T) {
	// Set higher rate limit for testing
	os.Setenv("CRAWLER_RPS", "10.0")     // 10 requests per second
	os.Setenv("CRAWLER_BURST_SIZE", "2") // Allow burst of 2
	t.Cleanup(func() {
		os.Unsetenv("CRAWLER_RPS")
		os.Unsetenv("CRAWLER_BURST_SIZE")
		config.ResetForTest()
		ResetLimiterForTest()
	})

	config.ResetForTest()
	ResetLimiterForTest()

	// With burst=2, first 2 calls should be immediate
	start := time.Now()
	waitForRateLimit()
	waitForRateLimit()
	burstElapsed := time.Since(start)

	// Burst should complete quickly (< 50ms)
	if burstElapsed > 50*time.Millisecond {
		t.Errorf("Expected burst to complete quickly, took %v", burstElapsed)
	}

	// Third call should wait for token refill (~100ms at 10 rps)
	start = time.Now()
	waitForRateLimit()
	elapsed := time.Since(start)

	expectedMin := 80 * time.Millisecond // Allow tolerance
	if elapsed < expectedMin {
		t.Errorf("Expected rate limit delay of ~100ms, got %v", elapsed)
	}
}

func TestRateLimiterEnforcesRate(t *testing.T) {
	// Set a precise rate for testing
	os.Setenv("CRAWLER_RPS", "5.0")      // 5 requests per second
	os.Setenv("CRAWLER_BURST_SIZE", "1") // No burst
	t.Cleanup(func() {
		os.Unsetenv("CRAWLER_RPS")
		os.Unsetenv("CRAWLER_BURST_SIZE")
		config.ResetForTest()
		ResetLimiterForTest()
	})

	config.ResetForTest()
	ResetLimiterForTest()

	// Make 6 calls and measure time
	start := time.Now()
	for i := 0; i < 6; i++ {
		waitForRateLimit()
	}
	elapsed := time.Since(start)

	// 6 calls at 5 rps should take ~1 second (first is immediate, 5 waits of 200ms each)
	expectedMin := 900 * time.Millisecond
	expectedMax := 1200 * time.Millisecond

	if elapsed < expectedMin {
		t.Errorf("Rate limit too fast: expected >=%v, got %v", expectedMin, elapsed)
	}
	if elapsed > expectedMax {
		t.Errorf("Rate limit too slow: expected <=%v, got %v", expectedMax, elapsed)
	}
}
