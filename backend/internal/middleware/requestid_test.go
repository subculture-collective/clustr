package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/logger"
)

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("generateRequestID should not return empty string")
	}

	if id1 == id2 {
		t.Error("generateRequestID should return unique IDs")
	}

	// Should be a valid hex string
	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("Request ID length should be 32, got %d", len(id1))
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request ID is in context
		reqID, ok := r.Context().Value(logger.RequestIDKey).(string)
		if !ok || reqID == "" {
			t.Error("Request ID not found in context")
		}

		// Check if request ID is in response header
		responseID := w.Header().Get(RequestIDHeader)
		if responseID == "" {
			t.Error("Request ID not found in response header")
		}

		if reqID != responseID {
			t.Error("Request ID in context doesn't match response header")
		}

		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestRequestIDMiddleware_ExistingID(t *testing.T) {
	existingID := "existing-request-id"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if existing request ID is preserved
		reqID, ok := r.Context().Value(logger.RequestIDKey).(string)
		if !ok || reqID != existingID {
			t.Errorf("Expected request ID %s, got %s", existingID, reqID)
		}

		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(RequestIDHeader, existingID)
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check if response header has the existing ID
	if w.Header().Get(RequestIDHeader) != existingID {
		t.Errorf("Expected request ID %s in response, got %s", existingID, w.Header().Get(RequestIDHeader))
	}
}
