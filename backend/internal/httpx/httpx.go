package httpx

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
)

// Note: In Go 1.20+, the global random number generator is automatically seeded.
// No explicit seeding is required for rand.Intn used in retry jitter.

// PreAttempt lets callers run logic (e.g., rate limiting) before each try; return context error to abort.
type PreAttempt func(ctx context.Context, attempt int) error

// AttemptInfo describes a single attempt outcome.
type AttemptInfo struct {
	Attempt int
	Method  string
	URL     string
	Status  int
	Err     error
	Wait    time.Duration
}

// Observer callback to report attempt telemetry.
type Observer func(info AttemptInfo)

// DoWithRetryFactory wraps an HTTP request with lightweight retries, honoring Retry-After, using config.
func DoWithRetryFactory(client *http.Client, build func() (*http.Request, error), pre PreAttempt) (*http.Response, error) {
	return DoWithRetryFactoryObs(client, build, pre, nil)
}

// DoWithRetryFactoryObs is like DoWithRetryFactory but reports attempts to an observer.
func DoWithRetryFactoryObs(client *http.Client, build func() (*http.Request, error), pre PreAttempt, obs Observer) (*http.Response, error) {
	cfg := config.Load()
	maxAttempts := cfg.HTTPMaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	baseDelay := cfg.HTTPRetryBase
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := build()
		if err != nil {
			return nil, err
		}
		if pre != nil {
			if err := pre(req.Context(), attempt); err != nil {
				return nil, err
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			// Network or transport error
			metrics.CrawlerHTTPRequests.WithLabelValues("error").Inc()
			if attempt == maxAttempts || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				if cfg.LogHTTPRetries {
					log.Printf("httpx: attempt=%d method=%s url=%s err=%v (no more retries)", attempt, req.Method, req.URL.String(), err)
				}
				if obs != nil {
					obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Err: err})
				}
				return nil, err
			}
			metrics.CrawlerHTTPRetries.Inc()
			if obs != nil {
				obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Err: err})
			}
		} else {
			// success unless 429/5xx
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				metrics.CrawlerHTTPRequests.WithLabelValues("success").Inc()
				if cfg.LogHTTPRetries && attempt > 1 {
					log.Printf("httpx: attempt=%d method=%s url=%s status=%d (success)", attempt, req.Method, req.URL.String(), resp.StatusCode)
				}
				if obs != nil {
					obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Status: resp.StatusCode})
				}
				return resp, nil
			}
			// 429 or 5xx - will retry
			metrics.CrawlerHTTPRequests.WithLabelValues("retry").Inc()
			if attempt == maxAttempts {
				if cfg.LogHTTPRetries {
					log.Printf("httpx: attempt=%d method=%s url=%s status=%d (giving up)", attempt, req.Method, req.URL.String(), resp.StatusCode)
				}
				if obs != nil {
					obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Status: resp.StatusCode})
				}
				return resp, nil
			}
			// Respect Retry-After header
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					resp.Body.Close()
					wait := time.Duration(secs) * time.Second
					metrics.CrawlerRetryAfterWaits.Observe(wait.Seconds())
					if cfg.LogHTTPRetries {
						log.Printf("httpx: attempt=%d 429/5xx Retry-After=%s wait=%s method=%s url=%s", attempt, ra, wait, req.Method, req.URL.String())
					}
					if obs != nil {
						obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Status: resp.StatusCode, Wait: wait})
					}
					if err := waitContext(req.Context(), wait); err != nil {
						return nil, err
					}
					continue
				}
				if t, err := http.ParseTime(ra); err == nil {
					delta := time.Until(t)
					if delta > 0 {
						resp.Body.Close()
						metrics.CrawlerRetryAfterWaits.Observe(delta.Seconds())
						if cfg.LogHTTPRetries {
							log.Printf("httpx: attempt=%d 429/5xx Retry-After=%s wait=%s method=%s url=%s", attempt, ra, delta, req.Method, req.URL.String())
						}
						if obs != nil {
							obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Status: resp.StatusCode, Wait: delta})
						}
						if err := waitContext(req.Context(), delta); err != nil {
							return nil, err
						}
						continue
					}
				}
			}
			resp.Body.Close()
			metrics.CrawlerHTTPRetries.Inc()
		}
		// backoff with jitter
		jitter := time.Duration(rand.Intn(200)) * time.Millisecond
		delay := baseDelay*time.Duration(attempt) + jitter
		if cfg.LogHTTPRetries {
			log.Printf("httpx: attempt=%d backing off=%s method=%s url=%s", attempt, delay, req.Method, req.URL.String())
		}
		if obs != nil {
			obs(AttemptInfo{Attempt: attempt, Method: req.Method, URL: req.URL.String(), Wait: delay})
		}
		if err := waitContext(req.Context(), delay); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("exhausted retries")
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
