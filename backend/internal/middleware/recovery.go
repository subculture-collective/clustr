package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/getsentry/sentry-go"
	"github.com/subculture-collective/clustr/backend/internal/errorreporting"
	"github.com/subculture-collective/clustr/backend/internal/logger"
)

// RecoverWithSentry recovers from panics and reports them to Sentry
func RecoverWithSentry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Get stack trace
				stack := debug.Stack()

				// Log the panic
				logger.ErrorContext(r.Context(), "Panic recovered",
					"error", err,
					"stack", string(stack),
					"method", r.Method,
					"path", r.URL.Path,
				)

				// Report to Sentry if enabled
				if errorreporting.IsSentryEnabled() {
					hub := sentry.CurrentHub().Clone()
					hub.Scope().SetRequest(r)
					hub.Scope().SetLevel(sentry.LevelError)

					// Add request context
					hub.Scope().SetTag("method", r.Method)
					hub.Scope().SetTag("path", r.URL.Path)

					// Capture the panic
					if e, ok := err.(error); ok {
						hub.CaptureException(e)
					} else {
						hub.CaptureMessage(errorreporting.ScrubPII(string(debug.Stack())))
					}
				}

				// Return 500 error
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
