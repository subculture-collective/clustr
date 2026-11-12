package apierr

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/subculture-collective/clustr/backend/internal/logger"
)

// ErrorCode represents a structured error code
type ErrorCode string

// Error code constants organized by category
const (
	// AUTH_ - Authentication and authorization errors
	ErrAuthMissing        ErrorCode = "AUTH_MISSING"
	ErrAuthInvalid        ErrorCode = "AUTH_INVALID"
	ErrAuthForbidden      ErrorCode = "AUTH_FORBIDDEN"
	ErrAuthOAuthNotConfig ErrorCode = "AUTH_OAUTH_NOT_CONFIGURED"
	ErrAuthOAuthFailed    ErrorCode = "AUTH_OAUTH_FAILED"

	// GRAPH_ - Graph query and processing errors
	ErrGraphTimeout       ErrorCode = "GRAPH_TIMEOUT"
	ErrGraphQuery         ErrorCode = "GRAPH_QUERY_FAILED"
	ErrGraphNoData        ErrorCode = "GRAPH_NO_DATA"
	ErrGraphInvalidParams ErrorCode = "GRAPH_INVALID_PARAMS"

	// CRAWL_ - Crawl job errors
	ErrCrawlInvalidSubreddit ErrorCode = "CRAWL_INVALID_SUBREDDIT"
	ErrCrawlQueueFailed      ErrorCode = "CRAWL_QUEUE_FAILED"
	ErrCrawlRateLimited      ErrorCode = "CRAWL_RATE_LIMITED"
	ErrCrawlNotFound         ErrorCode = "CRAWL_NOT_FOUND"

	// SEARCH_ - Search operation errors
	ErrSearchInvalidQuery ErrorCode = "SEARCH_INVALID_QUERY"
	ErrSearchTimeout      ErrorCode = "SEARCH_TIMEOUT"
	ErrSearchFailed       ErrorCode = "SEARCH_FAILED"

	// SYSTEM_ - System and server errors
	ErrSystemInternal    ErrorCode = "SYSTEM_INTERNAL"
	ErrSystemDatabase    ErrorCode = "SYSTEM_DATABASE"
	ErrSystemUnavailable ErrorCode = "SYSTEM_UNAVAILABLE"
	ErrSystemTimeout     ErrorCode = "SYSTEM_TIMEOUT"

	// VALIDATION_ - Request validation errors
	ErrValidationInvalidJSON   ErrorCode = "VALIDATION_INVALID_JSON"
	ErrValidationInvalidFormat ErrorCode = "VALIDATION_INVALID_FORMAT"
	ErrValidationMissingField  ErrorCode = "VALIDATION_MISSING_FIELD"
	ErrValidationInvalidValue  ErrorCode = "VALIDATION_INVALID_VALUE"

	// RESOURCE_ - Resource errors
	ErrResourceNotFound ErrorCode = "RESOURCE_NOT_FOUND"
	ErrResourceConflict ErrorCode = "RESOURCE_CONFLICT"

	// RATE_LIMIT_ - Rate limiting errors
	ErrRateLimitGlobal ErrorCode = "RATE_LIMIT_GLOBAL"
	ErrRateLimitIP     ErrorCode = "RATE_LIMIT_IP"
)

// Error represents a structured API error
type Error struct {
	Code      ErrorCode              `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	status    int                    // HTTP status code (not serialized)
}

// ErrorResponse is the top-level error response wrapper
type ErrorResponse struct {
	Error *Error `json:"error"`
}

// New creates a new API error
func New(code ErrorCode, message string, status int) *Error {
	return &Error{
		Code:    code,
		Message: message,
		status:  status,
	}
}

// WithDetails adds details to the error
func (e *Error) WithDetails(details map[string]interface{}) *Error {
	e.Details = details
	return e
}

// WithRequestID adds a request ID to the error
func (e *Error) WithRequestID(requestID string) *Error {
	e.RequestID = requestID
	return e
}

// Error implements the error interface
func (e *Error) Error() string {
	return string(e.Code) + ": " + e.Message
}

// Status returns the HTTP status code
func (e *Error) Status() int {
	return e.status
}

// WriteError writes a structured error response to the HTTP response writer
func WriteError(w http.ResponseWriter, err *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status())
	json.NewEncoder(w).Encode(ErrorResponse{Error: err})
}

// Helper functions for common errors

// AuthMissing creates an authentication missing error
func AuthMissing(message string) *Error {
	if message == "" {
		message = "Authentication required"
	}
	return New(ErrAuthMissing, message, http.StatusUnauthorized)
}

// AuthInvalid creates an invalid authentication error
func AuthInvalid(message string) *Error {
	if message == "" {
		message = "Invalid authentication credentials"
	}
	return New(ErrAuthInvalid, message, http.StatusUnauthorized)
}

// AuthForbidden creates a forbidden error
func AuthForbidden(message string) *Error {
	if message == "" {
		message = "Access forbidden"
	}
	return New(ErrAuthForbidden, message, http.StatusForbidden)
}

// AuthOAuthNotConfigured creates an OAuth not configured error
func AuthOAuthNotConfigured() *Error {
	return New(ErrAuthOAuthNotConfig, "OAuth not configured", http.StatusServiceUnavailable)
}

// AuthOAuthFailed creates an OAuth failed error
func AuthOAuthFailed(message string) *Error {
	if message == "" {
		message = "OAuth authentication failed"
	}
	return New(ErrAuthOAuthFailed, message, http.StatusBadGateway)
}

// GraphTimeout creates a graph query timeout error
func GraphTimeout(message string) *Error {
	if message == "" {
		message = "Graph query timeout - dataset may be too large. Try reducing max_nodes or max_links parameters."
	}
	return New(ErrGraphTimeout, message, http.StatusRequestTimeout)
}

// GraphQueryFailed creates a graph query failed error
func GraphQueryFailed(message string) *Error {
	if message == "" {
		message = "Graph query failed"
	}
	return New(ErrGraphQuery, message, http.StatusInternalServerError)
}

// GraphNoData creates a graph no data error
func GraphNoData() *Error {
	return New(ErrGraphNoData, "No graph data available", http.StatusNotFound)
}

// GraphInvalidParams creates a graph invalid parameters error
func GraphInvalidParams(message string) *Error {
	if message == "" {
		message = "Invalid graph query parameters"
	}
	return New(ErrGraphInvalidParams, message, http.StatusBadRequest)
}

// CrawlInvalidSubreddit creates an invalid subreddit error
func CrawlInvalidSubreddit(message string) *Error {
	if message == "" {
		message = "Invalid subreddit name"
	}
	return New(ErrCrawlInvalidSubreddit, message, http.StatusBadRequest)
}

// CrawlQueueFailed creates a crawl queue failed error
func CrawlQueueFailed(message string) *Error {
	if message == "" {
		message = "Failed to queue crawl job"
	}
	return New(ErrCrawlQueueFailed, message, http.StatusInternalServerError)
}

// CrawlRateLimited creates a crawl rate limited error
func CrawlRateLimited(message string) *Error {
	if message == "" {
		message = "Rate limit exceeded - too many requests"
	}
	return New(ErrCrawlRateLimited, message, http.StatusTooManyRequests)
}

// CrawlNotFound creates a crawl not found error
func CrawlNotFound(message string) *Error {
	if message == "" {
		message = "Crawl job not found"
	}
	return New(ErrCrawlNotFound, message, http.StatusNotFound)
}

// SearchInvalidQuery creates a search invalid query error
func SearchInvalidQuery(message string) *Error {
	if message == "" {
		message = "Invalid search query"
	}
	return New(ErrSearchInvalidQuery, message, http.StatusBadRequest)
}

// SearchTimeout creates a search timeout error
func SearchTimeout() *Error {
	return New(ErrSearchTimeout, "Search query timeout", http.StatusRequestTimeout)
}

// SearchFailed creates a search failed error
func SearchFailed(message string) *Error {
	if message == "" {
		message = "Search query failed"
	}
	return New(ErrSearchFailed, message, http.StatusInternalServerError)
}

// SystemInternal creates an internal server error
func SystemInternal(message string) *Error {
	if message == "" {
		message = "Internal server error"
	}
	return New(ErrSystemInternal, message, http.StatusInternalServerError)
}

// SystemDatabase creates a database error
func SystemDatabase(message string) *Error {
	if message == "" {
		message = "Database error"
	}
	return New(ErrSystemDatabase, message, http.StatusInternalServerError)
}

// SystemUnavailable creates a service unavailable error
func SystemUnavailable(message string) *Error {
	if message == "" {
		message = "Service unavailable"
	}
	return New(ErrSystemUnavailable, message, http.StatusServiceUnavailable)
}

// SystemTimeout creates a system timeout error
func SystemTimeout(message string) *Error {
	if message == "" {
		message = "Request timeout"
	}
	return New(ErrSystemTimeout, message, http.StatusRequestTimeout)
}

// ValidationInvalidJSON creates an invalid JSON error
func ValidationInvalidJSON() *Error {
	return New(ErrValidationInvalidJSON, "Invalid JSON request body", http.StatusBadRequest)
}

// ValidationInvalidFormat creates an invalid format error
func ValidationInvalidFormat(message string) *Error {
	if message == "" {
		message = "Invalid request format"
	}
	return New(ErrValidationInvalidFormat, message, http.StatusBadRequest)
}

// ValidationMissingField creates a missing field error
func ValidationMissingField(field string) *Error {
	return New(ErrValidationMissingField, "Missing required field: "+field, http.StatusBadRequest).
		WithDetails(map[string]interface{}{"field": field})
}

// ValidationInvalidValue creates an invalid value error
func ValidationInvalidValue(field string, message string) *Error {
	if message == "" {
		message = "Invalid value for field: " + field
	}
	return New(ErrValidationInvalidValue, message, http.StatusBadRequest).
		WithDetails(map[string]interface{}{"field": field})
}

// ResourceNotFound creates a resource not found error
func ResourceNotFound(resourceType string) *Error {
	return New(ErrResourceNotFound, resourceType+" not found", http.StatusNotFound).
		WithDetails(map[string]interface{}{"resource_type": resourceType})
}

// ResourceConflict creates a resource conflict error
func ResourceConflict(message string) *Error {
	if message == "" {
		message = "Resource conflict"
	}
	return New(ErrResourceConflict, message, http.StatusConflict)
}

// RateLimitGlobal creates a global rate limit error
func RateLimitGlobal() *Error {
	return New(ErrRateLimitGlobal, "Rate limit exceeded - too many requests globally", http.StatusTooManyRequests)
}

// RateLimitIP creates an IP rate limit error
func RateLimitIP() *Error {
	return New(ErrRateLimitIP, "Rate limit exceeded - too many requests from your IP", http.StatusTooManyRequests)
}

// GetRequestID extracts the request ID from the context
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(logger.RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// WriteErrorWithContext writes a structured error response with request ID from context
func WriteErrorWithContext(w http.ResponseWriter, r *http.Request, err *Error) {
	if reqID := GetRequestID(r.Context()); reqID != "" {
		err = err.WithRequestID(reqID)
	}
	WriteError(w, err)
}
