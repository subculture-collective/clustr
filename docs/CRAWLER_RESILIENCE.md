# Crawler Rate Limiting, Retries, and Resilience

This document describes the enhanced crawler features for rate limiting, retry logic, resilience, and observability.

## Overview

The crawler has been enhanced with:
1. **Configurable Rate Limiting** - Token bucket rate limiter with configurable RPS
2. **Prometheus Metrics** - Comprehensive monitoring of crawler operations
3. **Circuit Breaker** - Resilience pattern to prevent cascading failures
4. **Enhanced Error Detection** - Reddit-specific error classification and handling

## Configuration

### Rate Limiting

Control the rate of requests to Reddit's API:

```bash
# Requests per second (default: 1.66, ~60 per minute)
CRAWLER_RPS=1.66

# Burst capacity (default: 1)
CRAWLER_BURST_SIZE=1
```

The rate limiter uses a token bucket algorithm from `golang.org/x/time/rate`, which provides:
- Smooth rate limiting without fixed time windows
- Burst capacity for occasional spikes
- Fair resource distribution

### HTTP Retry Configuration

Fine-tune the retry behavior for HTTP requests:

```bash
# Maximum number of retry attempts (default: 3)
HTTP_MAX_RETRIES=3

# Base delay for exponential backoff in milliseconds (default: 300)
HTTP_RETRY_BASE_MS=300

# HTTP request timeout in milliseconds (default: 15000)
HTTP_TIMEOUT_MS=15000

# Enable detailed retry logging (default: false)
LOG_HTTP_RETRIES=true
```

### Retry Behavior

The crawler automatically handles:
- **429 Too Many Requests**: Respects `Retry-After` header
- **5xx Server Errors**: Exponential backoff with jitter
- **Network Errors**: Automatic retry with backoff
- **401 Unauthorized**: Retries to allow token refresh

Errors NOT retried (permanent failures):
- Private/banned/quarantined subreddits
- 404 Not Found
- 400 Bad Request
- 403 Forbidden

## Metrics

The crawler exposes Prometheus metrics at `/metrics` endpoint.

### Available Metrics

#### Crawler Operations

```
# Total crawl jobs by status (success/failed)
crawler_jobs_total{status="success|failed"}

# Duration of crawl jobs in seconds
crawler_job_duration_seconds{status="success|failed"}

# Total HTTP requests by outcome
crawler_http_requests_total{status="success|retry|error"}

# Total number of HTTP retries
crawler_http_retries_total

# Total rate limit waits
crawler_rate_limit_waits_total

# Retry-After wait durations in seconds
crawler_retry_after_wait_seconds

# Posts processed
crawler_posts_processed_total

# Comments processed
crawler_comments_processed_total
```

#### Database Operations

```
# Database operation duration
db_operation_duration_seconds{operation="query_name"}

# Database operation errors
db_operation_errors_total{operation="query_name"}
```

#### Circuit Breaker

```
# Circuit breaker state (0=closed, 1=open, 2=half-open)
circuit_breaker_state{component="db"}

# Number of circuit breaker trips
circuit_breaker_trips_total{component="db"}
```

### Prometheus Configuration

Example Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: 'reddit-crawler'
    scrape_interval: 15s
    static_configs:
      - targets: ['api:8000']
```

### Example Queries

```promql
# Request success rate
rate(crawler_http_requests_total{status="success"}[5m]) 
  / rate(crawler_http_requests_total[5m])

# Average job duration
rate(crawler_job_duration_seconds_sum[5m]) 
  / rate(crawler_job_duration_seconds_count[5m])

# Retry rate
rate(crawler_http_retries_total[5m])

# Posts per minute
rate(crawler_posts_processed_total[1m]) * 60
```

## Circuit Breaker

The circuit breaker prevents cascading failures by stopping requests to failing components.

### States

1. **Closed** (Normal operation)
   - Requests pass through
   - Failures increment counter
   - Opens after threshold failures

2. **Open** (Failing)
   - Requests fail immediately
   - Waits for timeout period
   - Transitions to Half-Open

3. **Half-Open** (Testing)
   - Limited requests allowed
   - Success → Closed
   - Failure → Open

### Configuration

Circuit breakers are configured in code:

```go
cb := circuitbreaker.New(circuitbreaker.Config{
    Name:             "database",
    FailureThreshold: 5,              // Open after 5 failures
    SuccessThreshold: 2,              // Close after 2 successes
    Timeout:          60 * time.Second, // Wait 60s before half-open
})
```

## Error Detection

The crawler now classifies Reddit API errors into specific types:

### Error Types

- `ErrorRateLimited` - 429 Too Many Requests (retryable)
- `ErrorPrivateSubreddit` - Private subreddit (permanent)
- `ErrorBannedSubreddit` - Banned subreddit (permanent)
- `ErrorQuarantined` - Quarantined subreddit (permanent)
- `ErrorNotFound` - Resource not found (permanent)
- `ErrorForbidden` - Access forbidden (permanent)
- `ErrorUnauthorized` - Invalid/expired token (retryable)
- `ErrorServerError` - Reddit 5xx error (retryable)
- `ErrorBadRequest` - Invalid request (permanent)

### Usage

```go
import "github.com/subculture-collective/clustr/backend/internal/redditapi"

resp, err := client.Get(url)
if err != nil {
    return err
}

if resp.StatusCode != 200 {
    apiErr := redditapi.ClassifyError(resp)
    if redditapi.IsPermanent(apiErr) {
        // Don't retry, mark as failed
        log.Printf("Permanent error: %v", apiErr)
        return apiErr
    }
    // Retryable error, will be handled by retry logic
}
```

## Monitoring Best Practices

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: crawler
    rules:
      - alert: HighCrawlerFailureRate
        expr: |
          rate(crawler_jobs_total{status="failed"}[5m]) 
          / rate(crawler_jobs_total[5m]) > 0.1
        for: 10m
        annotations:
          summary: "High crawler failure rate"

      - alert: CircuitBreakerOpen
        expr: circuit_breaker_state{component="db"} == 1
        for: 5m
        annotations:
          summary: "Circuit breaker open for {{ $labels.component }}"

      - alert: HighRetryRate
        expr: rate(crawler_http_retries_total[5m]) > 0.5
        for: 10m
        annotations:
          summary: "High HTTP retry rate"
```

### Dashboards

Key metrics to visualize:
1. **Request rate and latency**
2. **Success/failure rates**
3. **Retry counts and reasons**
4. **Circuit breaker states**
5. **Posts/comments processed**
6. **Queue depth and processing time**

## Testing

### Rate Limiter Tests

```bash
# Run rate limiter tests
go test ./internal/crawler -run TestRateLimiter -v
```

### Circuit Breaker Tests

```bash
# Run circuit breaker tests
go test ./internal/circuitbreaker -v
```

### Error Classification Tests

```bash
# Run Reddit API error tests
go test ./internal/redditapi -v
```

## Performance Tuning

### For Higher Throughput

```bash
# Increase rate limit (use with caution)
CRAWLER_RPS=3.0
CRAWLER_BURST_SIZE=2

# Reduce retry delays (faster failure detection)
HTTP_RETRY_BASE_MS=200
HTTP_MAX_RETRIES=2
```

### For More Reliability

```bash
# Conservative rate limiting
CRAWLER_RPS=1.0
CRAWLER_BURST_SIZE=1

# More retry attempts with longer delays
HTTP_MAX_RETRIES=5
HTTP_RETRY_BASE_MS=500
```

## Troubleshooting

### High Retry Rate

1. Check `crawler_http_requests_total{status="retry"}` by URL pattern
2. Look for 429 or 5xx responses
3. Consider reducing `CRAWLER_RPS`
4. Check Reddit API status

### Circuit Breaker Trips

1. Check `circuit_breaker_trips_total` metrics
2. Investigate underlying component health (usually database)
3. Review database slow query logs
4. Consider increasing timeout or failure threshold

### Rate Limiting Issues

1. Verify `CRAWLER_RPS` is not too high (Reddit allows ~60 req/min)
2. Check `crawler_rate_limit_waits_total` for excessive waits
3. Monitor `crawler_retry_after_wait_seconds` histogram
4. Enable `LOG_HTTP_RETRIES=true` for detailed logs

## Migration Notes

The new rate limiter is backward compatible. The default configuration maintains the previous behavior (~1.66 RPS).

### Breaking Changes

None. All changes are additive.

### Recommended Actions

1. Set up Prometheus scraping of `/metrics` endpoint
2. Create alerting rules for critical failures
3. Monitor retry rates and adjust thresholds if needed
4. Consider enabling `LOG_HTTP_RETRIES` temporarily for verification
