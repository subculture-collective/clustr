package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestParseAcceptEncoding(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "empty header",
			header:   "",
			expected: "",
		},
		{
			name:     "gzip only",
			header:   "gzip",
			expected: "gzip",
		},
		{
			name:     "brotli only",
			header:   "br",
			expected: "br",
		},
		{
			name:     "prefer brotli over gzip",
			header:   "gzip, br",
			expected: "br",
		},
		{
			name:     "prefer brotli with equal quality",
			header:   "gzip;q=1.0, br;q=1.0",
			expected: "br",
		},
		{
			name:     "prefer gzip with higher quality",
			header:   "gzip;q=1.0, br;q=0.5",
			expected: "gzip",
		},
		{
			name:     "prefer brotli with higher quality",
			header:   "gzip;q=0.5, br;q=1.0",
			expected: "br",
		},
		{
			name:     "brotli disabled with q=0",
			header:   "gzip, br;q=0",
			expected: "gzip",
		},
		{
			name:     "gzip disabled with q=0",
			header:   "gzip;q=0, br",
			expected: "br",
		},
		{
			name:     "both disabled",
			header:   "gzip;q=0, br;q=0",
			expected: "",
		},
		{
			name:     "complex header with other encodings",
			header:   "gzip, deflate, br",
			expected: "br",
		},
		{
			name:     "whitespace handling",
			header:   " gzip ; q=0.8 , br ; q=0.9 ",
			expected: "br",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAcceptEncoding(tt.header)
			if result != tt.expected {
				t.Errorf("parseAcceptEncoding(%q) = %q, want %q", tt.header, result, tt.expected)
			}
		})
	}
}

func TestGzip(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"test response that should be compressed"}`))
	})

	tests := []struct {
		name             string
		acceptEncoding   string
		expectEncoding   string
		expectCompressed bool
	}{
		{
			name:             "with gzip support",
			acceptEncoding:   "gzip",
			expectEncoding:   "gzip",
			expectCompressed: true,
		},
		{
			name:             "with gzip and deflate support",
			acceptEncoding:   "gzip, deflate",
			expectEncoding:   "gzip",
			expectCompressed: true,
		},
		{
			name:             "with brotli support",
			acceptEncoding:   "br",
			expectEncoding:   "br",
			expectCompressed: true,
		},
		{
			name:             "with brotli and gzip support (prefer brotli)",
			acceptEncoding:   "br, gzip",
			expectEncoding:   "br",
			expectCompressed: true,
		},
		{
			name:             "with gzip and brotli support (prefer brotli)",
			acceptEncoding:   "gzip, br",
			expectEncoding:   "br",
			expectCompressed: true,
		},
		{
			name:             "without compression support",
			acceptEncoding:   "",
			expectEncoding:   "",
			expectCompressed: false,
		},
		{
			name:             "with only deflate support",
			acceptEncoding:   "deflate",
			expectEncoding:   "",
			expectCompressed: false,
		},
		{
			name:             "brotli disabled with q=0",
			acceptEncoding:   "br;q=0, gzip",
			expectEncoding:   "gzip",
			expectCompressed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := Gzip(testHandler)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rr.Code)
			}

			// Check Vary header is always set
			varyHeader := rr.Header().Get("Vary")
			if !strings.Contains(varyHeader, "Accept-Encoding") {
				t.Errorf("expected Vary header to contain 'Accept-Encoding', got %q", varyHeader)
			}

			// Result snapshots the headers actually written on the wire. Reading
			// rr.Header directly would miss mutations made after WriteHeader.
			contentEncoding := rr.Result().Header.Get("Content-Encoding")
			if tt.expectCompressed {
				if contentEncoding != tt.expectEncoding {
					t.Errorf("expected Content-Encoding: %s, got %s", tt.expectEncoding, contentEncoding)
				}

				var body []byte
				var err error

				// Decompress based on encoding
				if tt.expectEncoding == "gzip" {
					gr, err := gzip.NewReader(rr.Body)
					if err != nil {
						t.Fatalf("failed to create gzip reader: %v", err)
					}
					defer gr.Close()

					body, err = io.ReadAll(gr)
					if err != nil {
						t.Fatalf("failed to read gzipped body: %v", err)
					}
				} else if tt.expectEncoding == "br" {
					br := brotli.NewReader(rr.Body)
					body, err = io.ReadAll(br)
					if err != nil {
						t.Fatalf("failed to read brotli body: %v", err)
					}
				}

				if !strings.Contains(string(body), "test response") {
					t.Error("decompressed body doesn't contain expected content")
				}
			} else {
				if contentEncoding != "" {
					t.Errorf("did not expect Content-Encoding, got %s", contentEncoding)
				}

				body := rr.Body.String()
				if !strings.Contains(body, "test response") {
					t.Error("body doesn't contain expected content")
				}
			}
		})
	}
}

// TestGzipWithPanic verifies that Content-Encoding is not set when handler panics
func TestGzipWithPanic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write header first, then panic before writing body
		w.WriteHeader(http.StatusOK)
		panic("test panic")
	})

	// Wrap with recovery middleware like in production
	handler := RecoverWithSentry(Gzip(panicHandler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// The recovery should have been triggered
	// Content-Encoding should NOT be set since we never actually wrote compressed content
	contentEncoding := rr.Header().Get("Content-Encoding")
	if contentEncoding != "" {
		t.Errorf("expected no Content-Encoding on panic before write, got %s", contentEncoding)
	}
}

// TestGzipHeadersOnFirstWrite verifies that Content-Encoding is set on first write, not before
func TestGzipHeadersOnFirstWrite(t *testing.T) {
	var headerSetBeforeWrite bool

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if Content-Encoding is set before we write
		if w.Header().Get("Content-Encoding") != "" {
			headerSetBeforeWrite = true
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test content"))
	})

	handler := Gzip(testHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if headerSetBeforeWrite {
		t.Error("Content-Encoding was set before first write")
	}

	// After writing, it should be set
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("Content-Encoding not set after write")
	}
}
