package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/config"
)

func TestDoWithRetry_RespectsRetryAfterSeconds(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "2")
	os.Setenv("HTTP_RETRY_BASE_MS", "1")
	t.Cleanup(func() {
		os.Unsetenv("HTTP_MAX_RETRIES")
		os.Unsetenv("HTTP_RETRY_BASE_MS")
	})
	// reset cached config so env takes effect
	config.ResetForTest()
	config.Load()

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{}
	start := time.Now()
	resp, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatalf("expected to wait for Retry-After; waited %v", time.Since(start))
	}
}

func TestDoWithRetry_StopsOnSuccess(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "3")
	os.Setenv("HTTP_RETRY_BASE_MS", "1")
	t.Cleanup(func() {
		os.Unsetenv("HTTP_MAX_RETRIES")
		os.Unsetenv("HTTP_RETRY_BASE_MS")
	})
	config.ResetForTest()
	config.Load()

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := &http.Client{}
	resp, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestDoWithRetry_ObserverAndBackoffOn5xx(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "3")
	os.Setenv("HTTP_RETRY_BASE_MS", "5")
	t.Cleanup(func() {
		os.Unsetenv("HTTP_MAX_RETRIES")
		os.Unsetenv("HTTP_RETRY_BASE_MS")
	})
	config.ResetForTest()
	config.Load()

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// return 500 twice, then 200
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var preCalls []int
	pre := func(ctx context.Context, attempt int) error {
		preCalls = append(preCalls, attempt)
		return nil
	}

	var observed []AttemptInfo
	obs := func(info AttemptInfo) { observed = append(observed, info) }

	client := &http.Client{}
	start := time.Now()
	resp, err := DoWithRetryFactoryObs(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, pre, obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// We expect 3 attempts total (2 failures + 1 success)
	if len(preCalls) != 3 {
		t.Fatalf("expected preAttempt called 3 times, got %d", len(preCalls))
	}
	// Ensure some backoff happened (rough lower bound ~5ms + 10ms with jitter)
	elapsed := time.Since(start)
	if elapsed < 12*time.Millisecond {
		t.Fatalf("expected backoff to take effect, elapsed=%v", elapsed)
	}
	// Observer should have recorded at least the failed attempts with waits
	hadWait := false
	for _, oi := range observed {
		if oi.Wait > 0 {
			hadWait = true
			break
		}
	}
	if !hadWait {
		t.Fatalf("expected observer to record at least one wait > 0")
	}
}
