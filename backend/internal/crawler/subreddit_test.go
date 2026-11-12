package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/config"
)

// fakeAccessToken avoids hitting Reddit OAuth in tests
// no-op
func init() {}

func TestCrawlSubreddit_Pagination(t *testing.T) {
	config.ResetForTest()
	// Set config defaults via env if needed. Using defaults is fine here.
	about := map[string]interface{}{"data": map[string]interface{}{"title": "t", "public_description": "d", "subscribers": 1}}
	page := func(after string, ids []string, next string) map[string]interface{} {
		var children []map[string]interface{}
		for _, id := range ids {
			children = append(children, map[string]interface{}{"data": map[string]interface{}{"id": id, "title": id, "author": "u", "created_utc": float64(1730000000)}})
		}
		return map[string]interface{}{"data": map[string]interface{}{"children": children, "after": next}}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		_ = r.ParseForm()
		if path == "/r/test/about" {
			json.NewEncoder(w).Encode(about)
			return
		}
		if path == "/r/test/top" {
			after := r.URL.Query().Get("after")
			switch after {
			case "":
				json.NewEncoder(w).Encode(page("", []string{"p1", "p2"}, "t3_after"))
			case "t3_after":
				json.NewEncoder(w).Encode(page("t3_after", []string{"p3"}, ""))
			default:
				json.NewEncoder(w).Encode(page(after, []string{}, ""))
			}
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	// Patch authenticatedGet to redirect to test server and avoid OAuth
	oldAuth := authenticatedGet
	authenticatedGet = func(u string) (*http.Response, error) {
		u = strings.Replace(u, "https://oauth.reddit.com", server.URL, 1)
		return http.Get(u)
	}
	defer func() { authenticatedGet = oldAuth }()

	info, posts, err := CrawlSubreddit("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil || info.Title != "t" {
		t.Fatalf("unexpected about: %+v", info)
	}
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}
}
