package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
	"github.com/subculture-collective/clustr/backend/internal/config"
	"github.com/subculture-collective/clustr/backend/internal/db"
)

// TestAdminAuthMiddleware tests the admin authentication middleware
func TestAdminAuthMiddleware(t *testing.T) {
	// Reset config for each test
	defer config.ResetForTest()

	tests := []struct {
		name           string
		adminToken     string
		authHeader     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid token",
			adminToken:     "test-admin-token-123",
			authHeader:     "Bearer test-admin-token-123",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "invalid token",
			adminToken:     "test-admin-token-123",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized\n",
		},
		{
			name:           "missing token",
			adminToken:     "test-admin-token-123",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized\n",
		},
		{
			name:           "malformed bearer token",
			adminToken:     "test-admin-token-123",
			authHeader:     "Bearertest-admin-token-123",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized\n",
		},
		{
			name:           "wrong auth scheme",
			adminToken:     "test-admin-token-123",
			authHeader:     "Basic dGVzdDp0ZXN0",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized\n",
		},
		{
			name:           "admin token not configured",
			adminToken:     "",
			authHeader:     "Bearer test-admin-token-123",
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "admin token not configured\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			os.Setenv("ADMIN_API_TOKEN", tt.adminToken)
			config.ResetForTest()

			// Create a test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			})

			// Create router with admin middleware
			router := createTestRouterWithAdmin(testHandler)

			// Create request
			req := httptest.NewRequest("GET", "/api/admin/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Serve request
			router.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// Check response body
			if rr.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}
		})
	}
}

// createTestRouterWithAdmin creates a minimal router with admin middleware for testing
func createTestRouterWithAdmin(handler http.Handler) *mux.Router {
	r := mux.NewRouter()
	cfg := config.Load()

	// Admin auth middleware - same logic as in routes.go
	adminOnly := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.AdminAPIToken == "" {
				http.Error(w, "admin token not configured", http.StatusServiceUnavailable)
				return
			}
			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix || auth[len(prefix):] != cfg.AdminAPIToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	r.Handle("/api/admin/test", adminOnly(handler))
	return r
}

// TestAdminEndpointsRequireAuth tests that all admin endpoints are protected
func TestAdminEndpointsRequireAuth(t *testing.T) {
	// Set admin token
	os.Setenv("ADMIN_API_TOKEN", "test-token")
	defer config.ResetForTest()

	// Create a mock queries object (nil is fine for this test)
	var q *db.Queries

	// Create router
	router := NewRouter(q)

	// Test admin endpoints without authentication
	adminEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/admin/services"},
		{"POST", "/api/admin/services"},
		{"GET", "/api/admin/backups"},
		{"GET", "/api/admin/backups/test.sql"},
	}

	for _, endpoint := range adminEndpoints {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// Should return 401 Unauthorized without auth
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401 for %s %s without auth, got %d",
					endpoint.method, endpoint.path, rr.Code)
			}
		})
	}
}

// TestAdminEndpointsWithAuth tests that admin endpoints work with valid auth
func TestAdminEndpointsWithAuth(t *testing.T) {
	// Set admin token
	adminToken := "test-admin-token-secure-123"
	os.Setenv("ADMIN_API_TOKEN", adminToken)
	defer config.ResetForTest()

	// Create a mock queries object (nil is fine for this test)
	var q *db.Queries

	// Create router
	router := NewRouter(q)

	// Test that admin endpoints accept valid token
	// Note: These may return 500 or other errors due to nil queries,
	// but they should not return 401 Unauthorized
	adminEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/admin/services"},
		{"GET", "/api/admin/backups"},
	}

	for _, endpoint := range adminEndpoints {
		t.Run(endpoint.method+" "+endpoint.path+" with auth", func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// Should NOT return 401 Unauthorized with valid token
			// (may return 500 due to nil db, but that's okay for this test)
			if rr.Code == http.StatusUnauthorized {
				t.Errorf("expected non-401 status for %s %s with valid auth, got %d",
					endpoint.method, endpoint.path, rr.Code)
			}
		})
	}
}
