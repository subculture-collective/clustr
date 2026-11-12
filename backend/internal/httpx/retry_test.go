package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/config"
)

func TestDoWithRetry_MaxRetriesExceeded(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "2")
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
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := &http.Client{}
	resp, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, nil)

	// httpx returns response (not error) when max retries exceeded for 5xx
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	// Should attempt initial + 1 retry (maxRetries=2 means 2 total attempts)
	expectedAttempts := 2
	if attempts != expectedAttempts {
		t.Errorf("expected %d attempts, got %d", expectedAttempts, attempts)
	}
}

func TestDoWithRetry_ContextCanceled(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "3")
	os.Setenv("HTTP_RETRY_BASE_MS", "100")
	t.Cleanup(func() {
		os.Unsetenv("HTTP_MAX_RETRIES")
		os.Unsetenv("HTTP_RETRY_BASE_MS")
	})
	config.ResetForTest()
	config.Load()

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := &http.Client{}
	_, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	}, nil)

	if err == nil {
		t.Fatal("expected error from canceled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestDoWithRetry_RequestFactoryError(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "2")
	t.Cleanup(func() {
		os.Unsetenv("HTTP_MAX_RETRIES")
	})
	config.ResetForTest()
	config.Load()

	client := &http.Client{}
	expectedErr := errors.New("factory error")

	_, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return nil, expectedErr
	}, nil)

	if err == nil {
		t.Fatal("expected error from request factory")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected factory error, got %v", err)
	}
}

func TestDoWithRetry_PreAttemptError(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "2")
	os.Setenv("HTTP_RETRY_BASE_MS", "1")
	t.Cleanup(func() {
		os.Unsetenv("HTTP_MAX_RETRIES")
		os.Unsetenv("HTTP_RETRY_BASE_MS")
	})
	config.ResetForTest()
	config.Load()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	expectedErr := errors.New("pre-attempt error")
	preAttempt := func(ctx context.Context, attempt int) error {
		return expectedErr
	}

	client := &http.Client{}
	_, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, preAttempt)

	if err == nil {
		t.Fatal("expected error from preAttempt")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected preAttempt error, got %v", err)
	}
}

func TestDoWithRetry_4xxNoRetry(t *testing.T) {
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
		w.WriteHeader(http.StatusBadRequest) // 400 should not retry
	}))
	defer ts.Close()

	client := &http.Client{}
	resp, err := DoWithRetryFactory(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	// Should only attempt once since 4xx errors shouldn't retry
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestDoWithRetry_RetryAfterDate(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "2")
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
		if attempts == 1 {
			// Set Retry-After as HTTP date in the future
			future := time.Now().Add(100 * time.Millisecond).UTC().Format(http.TimeFormat)
			w.Header().Set("Retry-After", future)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
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

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify 2 attempts were made (1 failure with retry-after, 1 success)
	// Don't assert on timing since time.Until() can vary based on processing time
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestDoWithRetryFactoryObs_ObserverCalled(t *testing.T) {
	os.Setenv("HTTP_MAX_RETRIES", "2")
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
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var observedAttempts []AttemptInfo
	observer := func(info AttemptInfo) {
		observedAttempts = append(observedAttempts, info)
	}

	client := &http.Client{}
	resp, err := DoWithRetryFactoryObs(client, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	}, nil, observer)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Should observe 2 attempts (1 failure + 1 success)
	if len(observedAttempts) != 2 {
		t.Errorf("expected 2 observed attempts, got %d", len(observedAttempts))
	}

	// First attempt should have non-zero wait
	if len(observedAttempts) >= 2 && observedAttempts[0].Wait == 0 {
		t.Error("expected first failed attempt to have wait time")
	}
}
