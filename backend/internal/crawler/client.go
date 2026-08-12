package crawler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/httpx"
)

var httpClient = &http.Client{Timeout: config.Load().HTTPTimeout}

type RedditHTTPError struct {
	StatusCode int
	Retryable  bool
}

func (e *RedditHTTPError) Error() string {
	return fmt.Sprintf("reddit request failed with HTTP %d", e.StatusCode)
}

// RedditCollectionAdapter owns Reddit transport policy. Package-level wrappers
// below remain only for test and migration compatibility.
type RedditCollectionAdapter struct{ client *http.Client }

var redditCollection = &RedditCollectionAdapter{client: httpClient}

// authenticatedGet issues a GET with OAuth Bearer token and Reddit-compliant User-Agent.
// It uses DoWithRetryFactory which applies light retries and a pre-attempt hook
// wired to waitForRateLimit() so we never exceed our global pacing.
var authenticatedGet = func(url string) (*http.Response, error) {
	return authenticatedGetContext(context.Background(), url)
}

var authenticatedGetWithContext = authenticatedGetContext

func authenticatedGetContext(ctx context.Context, url string) (*http.Response, error) {
	return redditCollection.get(ctx, url, true)
}

func (a *RedditCollectionAdapter) get(ctx context.Context, url string, authenticated bool) (*http.Response, error) {
	var token string
	var err error
	if authenticated {
		token, err = getAccessTokenContext(ctx)
	}
	if err != nil {
		return nil, err
	}
	ua := config.Load().UserAgent
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("User-Agent", ua)
		return req, nil
	}
	pre := func(attemptCtx context.Context, attempt int) error { return waitForRateLimitContext(attemptCtx) }
	resp, err := httpx.DoWithRetryFactory(a.client, build, pre)
	if err != nil {
		return nil, err
	}
	if authenticated && resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		globalTokenManager.invalidate()
		token, err = getAccessTokenContext(ctx)
		if err != nil {
			return nil, err
		}
		resp, err = httpx.DoWithRetryFactory(a.client, build, pre)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		status := resp.StatusCode
		resp.Body.Close()
		return nil, &RedditHTTPError{StatusCode: status, Retryable: status == http.StatusTooManyRequests || status >= 500}
	}
	return resp, nil
}

// unauthenticatedGet performs a GET without OAuth, but with Reddit-compliant User-Agent and retries.
var unauthenticatedGet = func(url string) (*http.Response, error) {
	return unauthenticatedGetContext(context.Background(), url)
}

func unauthenticatedGetContext(ctx context.Context, url string) (*http.Response, error) {
	return redditCollection.get(ctx, url, false)
}
