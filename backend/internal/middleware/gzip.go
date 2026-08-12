package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
)

// compressionResponseWriter wraps http.ResponseWriter to support compression.
// It delays setting Content-Encoding until the first write to avoid issues with
// panic recovery that might write uncompressed error responses.
type compressionResponseWriter struct {
	http.ResponseWriter
	writer      io.WriteCloser
	encoding    string
	wroteHeader bool
	headerSet   bool
	status      int
}

func (w *compressionResponseWriter) WriteHeader(status int) {
	if !w.wroteHeader && w.status == 0 {
		w.status = status
	}
}

func (w *compressionResponseWriter) setCompressionHeaders() {
	if w.headerSet {
		return
	}
	w.headerSet = true
	w.ResponseWriter.Header().Set("Content-Encoding", w.encoding)
	w.ResponseWriter.Header().Del("Content-Length")
}

func (w *compressionResponseWriter) Write(b []byte) (int, error) {
	w.setCompressionHeaders()
	if !w.wroteHeader {
		w.wroteHeader = true
		if w.status == 0 {
			w.status = http.StatusOK
		}
		w.ResponseWriter.WriteHeader(w.status)
	}

	return w.writer.Write(b)
}

func (w *compressionResponseWriter) finishEmpty() {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)
}

// parseAcceptEncoding parses the Accept-Encoding header and returns the best
// supported encoding based on q-values. Returns "br" for brotli, "gzip" for gzip,
// or "" for no compression.
func parseAcceptEncoding(header string) string {
	if header == "" {
		return ""
	}

	var bestEncoding string
	var bestQuality float64

	// Parse comma-separated encodings
	encodings := strings.Split(header, ",")
	for _, enc := range encodings {
		enc = strings.TrimSpace(enc)

		// Split on semicolon to separate encoding from q-value
		parts := strings.Split(enc, ";")
		encoding := strings.TrimSpace(parts[0])

		// Default quality is 1.0
		quality := 1.0

		// Parse q-value if present
		if len(parts) > 1 {
			for _, param := range parts[1:] {
				param = strings.TrimSpace(param)
				if strings.HasPrefix(param, "q=") {
					if q, err := strconv.ParseFloat(param[2:], 64); err == nil {
						quality = q
					}
				}
			}
		}

		// Skip if quality is 0 (explicitly disabled)
		if quality <= 0 {
			continue
		}

		// Check if this is better than what we have
		// Prefer brotli over gzip when quality is equal
		if encoding == "br" && quality >= bestQuality {
			bestEncoding = "br"
			bestQuality = quality
		} else if encoding == "gzip" && quality > bestQuality {
			bestEncoding = "gzip"
			bestQuality = quality
		}
	}

	return bestEncoding
}

// Gzip returns a middleware that compresses HTTP responses with brotli or gzip
// based on client's Accept-Encoding header. Prefers brotli over gzip.
func Gzip(next http.Handler) http.Handler {
	// Pool gzip writers to reduce allocations
	gzPool := sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(io.Discard)
		},
	}

	// Pool brotli writers to reduce allocations
	brPool := sync.Pool{
		New: func() interface{} {
			return brotli.NewWriter(io.Discard)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Vary header for proper caching behavior
		w.Header().Add("Vary", "Accept-Encoding")

		// Parse Accept-Encoding header with q-value support
		encoding := parseAcceptEncoding(r.Header.Get("Accept-Encoding"))

		// Use brotli if preferred
		if encoding == "br" {
			// Get brotli writer from pool
			br := brPool.Get().(*brotli.Writer)
			br.Reset(w)
			completed := false
			defer func() {
				if !completed {
					br.Reset(io.Discard)
					brPool.Put(br)
				}
			}()

			// Wrap response writer (Content-Encoding will be set on first write)
			brw := &compressionResponseWriter{
				ResponseWriter: w,
				writer:         br,
				encoding:       "br",
			}
			next.ServeHTTP(brw, r)
			if brw.wroteHeader {
				_ = br.Close()
			} else {
				brw.finishEmpty()
				br.Reset(io.Discard)
			}
			brPool.Put(br)
			completed = true
			return
		}

		// Use gzip if preferred
		if encoding == "gzip" {
			// Get gzip writer from pool
			gz := gzPool.Get().(*gzip.Writer)
			gz.Reset(w)
			completed := false
			defer func() {
				if !completed {
					gz.Reset(io.Discard)
					gzPool.Put(gz)
				}
			}()

			// Wrap response writer (Content-Encoding will be set on first write)
			gzw := &compressionResponseWriter{
				ResponseWriter: w,
				writer:         gz,
				encoding:       "gzip",
			}
			next.ServeHTTP(gzw, r)
			if gzw.wroteHeader {
				_ = gz.Close()
			} else {
				gzw.finishEmpty()
				gz.Reset(io.Discard)
			}
			gzPool.Put(gz)
			completed = true
			return
		}

		// No compression support
		next.ServeHTTP(w, r)
	})
}
