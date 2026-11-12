package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/config"
)

// The raw table listings, the crawl queue and crawl enqueueing are operator
// tools. They return source rows without the public suppression filter, so
// they must never answer an anonymous request.
func TestInternalDataRoutesRequireAdminToken(t *testing.T) {
	config.ResetForTest()
	t.Cleanup(config.ResetForTest)
	t.Setenv("ADMIN_API_TOKEN", "test-only-admin-token-with-enough-length")
	router := NewRouter(nil)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/subreddits?limit=1"},
		{http.MethodGet, "/users?limit=1"},
		{http.MethodGet, "/posts?subreddit_id=1&limit=1"},
		{http.MethodGet, "/comments?post_id=abc"},
		{http.MethodGet, "/jobs?limit=1"},
		{http.MethodGet, "/api/crawl/status"},
		{http.MethodPost, "/api/crawl"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			anonymous := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"subreddit":"golang"}`))
			anonymous.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, anonymous)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous %s %s status = %d, want 401", route.method, route.path, rr.Code)
			}

			wrongToken := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"subreddit":"golang"}`))
			wrongToken.Header.Set("Content-Type", "application/json")
			wrongToken.Header.Set("Authorization", "Bearer not-the-token")
			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, wrongToken)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("wrong token %s %s status = %d, want 401", route.method, route.path, rr.Code)
			}
		})
	}
}

// A correct token reaches the handler. The router has no database here, so
// anything other than 401 and 404 shows the route is registered behind the gate.
func TestInternalDataRoutesAcceptAdminToken(t *testing.T) {
	config.ResetForTest()
	t.Cleanup(config.ResetForTest)
	const token = "test-only-admin-token-with-enough-length"
	t.Setenv("ADMIN_API_TOKEN", token)
	router := NewRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/jobs?limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusNotFound {
		t.Fatalf("GET /jobs with the admin token status = %d, want the handler to be reached", rr.Code)
	}
}
