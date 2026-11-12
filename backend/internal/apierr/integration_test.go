package apierr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/apierr"
	"github.com/subculture-collective/clustr/backend/internal/middleware"
)

// Test that structured errors are properly serialized
func TestErrorSerialization(t *testing.T) {
	tests := []struct {
		name       string
		err        *apierr.Error
		wantStatus int
		wantCode   apierr.ErrorCode
	}{
		{
			name:       "graph timeout",
			err:        apierr.GraphTimeout(""),
			wantStatus: http.StatusRequestTimeout,
			wantCode:   apierr.ErrGraphTimeout,
		},
		{
			name:       "auth invalid",
			err:        apierr.AuthInvalid(""),
			wantStatus: http.StatusUnauthorized,
			wantCode:   apierr.ErrAuthInvalid,
		},
		{
			name:       "rate limit global",
			err:        apierr.RateLimitGlobal(),
			wantStatus: http.StatusTooManyRequests,
			wantCode:   apierr.ErrRateLimitGlobal,
		},
		{
			name:       "validation missing field",
			err:        apierr.ValidationMissingField("username"),
			wantStatus: http.StatusBadRequest,
			wantCode:   apierr.ErrValidationMissingField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			apierr.WriteError(w, tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			var resp apierr.ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Error == nil {
				t.Fatal("expected error in response")
			}

			if resp.Error.Code != tt.wantCode {
				t.Errorf("code = %s, want %s", resp.Error.Code, tt.wantCode)
			}

			if resp.Error.Message == "" {
				t.Error("expected non-empty message")
			}
		})
	}
}

// Test that request ID is included when using WriteErrorWithContext
func TestErrorWithRequestID(t *testing.T) {
	// Create a mock request with request ID in context
	r := httptest.NewRequest("GET", "/test", nil)

	// Add request ID to context using middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is in context
		reqID := apierr.GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected request ID in context")
		}

		// Write error with request ID
		err := apierr.GraphTimeout("").WithRequestID(reqID)
		apierr.WriteError(w, err)
	})

	// Wrap with RequestID middleware
	wrapped := middleware.RequestID(handler)

	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)

	var resp apierr.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error.RequestID == "" {
		t.Error("expected request_id in error response")
	}

	// Verify request ID is also in response header
	headerID := w.Header().Get("X-Request-ID")
	if headerID == "" {
		t.Error("expected X-Request-ID header")
	}

	if resp.Error.RequestID != headerID {
		t.Errorf("request_id mismatch: body=%s, header=%s", resp.Error.RequestID, headerID)
	}
}

// Test WriteErrorWithContext convenience function
func TestWriteErrorWithContext(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use convenience function that extracts request ID automatically
		apierr.WriteErrorWithContext(w, r, apierr.AuthInvalid("invalid token"))
	})

	// Wrap with RequestID middleware
	wrapped := middleware.RequestID(handler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	wrapped.ServeHTTP(w, r)

	var resp apierr.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error.RequestID == "" {
		t.Error("WriteErrorWithContext should include request_id")
	}
}

// Test error details
func TestErrorDetails(t *testing.T) {
	w := httptest.NewRecorder()

	// Create error with details
	err := apierr.New(apierr.ErrValidationInvalidValue, "must be positive", http.StatusBadRequest).
		WithDetails(map[string]interface{}{
			"field": "age",
			"value": -5,
			"min":   0,
		})

	apierr.WriteError(w, err)

	var resp apierr.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error.Details == nil {
		t.Fatal("expected details in response")
	}

	if field, ok := resp.Error.Details["field"].(string); !ok || field != "age" {
		t.Errorf("expected 'field'='age' in details, got %v", resp.Error.Details["field"])
	}

	if _, ok := resp.Error.Details["value"]; !ok {
		t.Error("expected 'value' in details")
	}

	if _, ok := resp.Error.Details["min"]; !ok {
		t.Error("expected 'min' in details")
	}
}

// Test that GetRequestID returns empty string when not in context
func TestGetRequestIDEmpty(t *testing.T) {
	r := httptest.NewRequest("GET", "/test", nil)
	reqID := apierr.GetRequestID(r.Context())
	if reqID != "" {
		t.Errorf("expected empty request ID, got %s", reqID)
	}
}

// Test that GetRequestID returns the value from context
func TestGetRequestIDFromContext(t *testing.T) {
	r := httptest.NewRequest("GET", "/test", nil)

	// Simulate RequestID middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := apierr.GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.RequestID(handler)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)
}
