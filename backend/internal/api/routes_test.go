package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
)

func TestCatalogProductionRequiresStrongCursorSecret(t *testing.T) {
	config.ResetForTest()
	t.Cleanup(config.ResetForTest)
	t.Setenv("ENV", "production")
	t.Setenv("CATALOG_ENABLED", "true")
	t.Setenv("CATALOG_CURSOR_SECRET", "short")
	defer func() {
		if recover() == nil {
			t.Fatal("expected production Catalog configuration to fail closed")
		}
	}()
	_ = NewRouter(nil)
}

func TestPublicAPIHealthAlias(t *testing.T) {
	router := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/health status = %d, want 200", rr.Code)
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode API health response: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("API health status = %q, want ok", response.Status)
	}
}

// TestSearchEndpointRegistered verifies the search endpoint is registered.
// This test only validates route registration; handler functionality
// is comprehensively tested in the handlers package.
func TestSearchEndpointRegistered(t *testing.T) {
	router := NewRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// A 404 means the route doesn't exist; any other status (even 500)
	// means the route is registered and we reached the handler
	if rr.Code == http.StatusNotFound {
		t.Error("search endpoint not registered")
	}
}

// TestExportEndpointRegistered verifies the export endpoint is registered.
// This test only validates route registration; handler functionality
// is comprehensively tested in the handlers package.
func TestExportEndpointRegistered(t *testing.T) {
	router := NewRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/export", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// A 404 means the route doesn't exist; any other status means registered
	if rr.Code == http.StatusNotFound {
		t.Error("export endpoint not registered")
	}
}

// TestGraphEndpointCompression verifies the graph endpoint has compression middleware applied.
// This test validates that Vary and compression middleware are in the handler chain.
// Note: With nil queries, the handler will panic and be recovered, but the middleware
// behavior can still be validated.
func TestGraphEndpointCompression(t *testing.T) {
	router := NewRouter(nil)

	tests := []struct {
		name           string
		acceptEncoding string
		expectVary     bool
	}{
		{
			name:           "with brotli support",
			acceptEncoding: "br",
			expectVary:     true,
		},
		{
			name:           "with gzip support",
			acceptEncoding: "gzip",
			expectVary:     true,
		},
		{
			name:           "without compression",
			acceptEncoding: "",
			expectVary:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			// Endpoint should be registered (not 404)
			if rr.Code == http.StatusNotFound {
				t.Error("graph endpoint not registered")
			}

			// Check Vary header is set (indicates compression middleware is applied)
			if tt.expectVary {
				varyHeader := rr.Header().Get("Vary")
				if !strings.Contains(varyHeader, "Accept-Encoding") {
					t.Errorf("expected Vary header to contain 'Accept-Encoding', got %q", varyHeader)
				}
			}

			// When handler panics (due to nil queries), Content-Encoding should NOT be set
			// because our middleware only sets it on first write
			contentEncoding := rr.Header().Get("Content-Encoding")
			if contentEncoding != "" {
				t.Errorf("Content-Encoding should not be set when handler doesn't write: got %s", contentEncoding)
			}
		})
	}
}
