package crawler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCrawlCommentsContextRejectsHTTPError(t *testing.T) {
	old := authenticatedGetWithContext
	defer func() { authenticatedGetWithContext = old }()
	authenticatedGetWithContext = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests", Body: io.NopCloser(strings.NewReader(`{"message":"slow down"}`))}, nil
	}
	if _, err := CrawlCommentsContext(context.Background(), "abc"); err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestCrawlCommentsContextRejectsMalformedListing(t *testing.T) {
	old := authenticatedGetWithContext
	defer func() { authenticatedGetWithContext = old }()
	authenticatedGetWithContext = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`[{"data":{}}]`))}, nil
	}
	if _, err := CrawlCommentsContext(context.Background(), "abc"); err == nil {
		t.Fatal("expected malformed listing error")
	}
}

func TestParseCommentsWithLimit_Depth0(t *testing.T) {
	var children []interface{}
	out := parseCommentsWithLimit(children, 0, 0)
	if len(out) != 0 {
		t.Fatalf("expected 0 comments at depth 0, got %d", len(out))
	}
}

func TestParseCommentsWithLimit_StopsAtMaxDepth(t *testing.T) {
	// Construct minimal nested structure: 2 levels
	nested := []interface{}{
		map[string]interface{}{
			"kind": "t1",
			"data": map[string]interface{}{
				"id":          "c1",
				"author":      "user1",
				"body":        "parent",
				"created_utc": float64(1730000000),
				"replies": map[string]interface{}{
					"data": map[string]interface{}{
						"children": []interface{}{
							map[string]interface{}{
								"kind": "t1",
								"data": map[string]interface{}{
									"id":          "c2",
									"author":      "user2",
									"body":        "child",
									"created_utc": float64(1730000001),
								},
							},
						},
					},
				},
			},
		},
	}
	// With maxDepth=0, should include only the parent (depth 0)
	out0 := parseCommentsWithLimit(nested, 0, 0)
	if len(out0) != 1 {
		t.Fatalf("expected 1 at maxDepth=0, got %d", len(out0))
	}
	// With maxDepth=1, includes parent and child
	out1 := parseCommentsWithLimit(nested, 0, 1)
	if len(out1) != 2 {
		t.Fatalf("expected 2 at maxDepth=1, got %d", len(out1))
	}
}
