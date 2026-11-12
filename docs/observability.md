# Observability Guide

This guide covers the observability features available in the Reddit Cluster Map project, including structured logging, distributed tracing, and error reporting.

## Overview

The project includes three main observability features:

1. **Structured Logging** - Context-aware logging with request IDs and log levels
2. **Distributed Tracing** - OpenTelemetry-based tracing for API, crawler, and graph operations
3. **Error Reporting** - Sentry integration with PII scrubbing for production error tracking

All features are opt-in and controlled via environment variables.

## Structured Logging

### Configuration

Set the log level via environment variable:

```bash
LOG_LEVEL=info  # Options: debug, info, warn, error
ENV=production  # Options: development, production
```

- **Development mode**: Human-readable text format
- **Production mode**: JSON format for log aggregation

### Log Levels

- `debug` - Detailed debugging information
- `info` - General informational messages (default)
- `warn` - Warning messages for potential issues
- `error` - Error messages for failures

### Request ID Tracking

Every API request receives a unique request ID that's:
- Added to the response header as `X-Request-ID`
- Included in all logs for that request
- Automatically propagated through the request context

You can also provide your own request ID via the `X-Request-ID` request header.

### Example Logs

**Development (text format):**
```
time=2024-10-26T12:34:56.789Z level=INFO msg="Starting API server" version=dev log_level=info
time=2024-10-26T12:34:57.123Z level=INFO msg="Server running" address=:8000 request_id=a1b2c3d4e5f6
```

**Production (JSON format):**
```json
{"time":"2024-10-26T12:34:56.789Z","level":"INFO","msg":"Starting API server","version":"dev","log_level":"info"}
{"time":"2024-10-26T12:34:57.123Z","level":"INFO","msg":"Server running","address":":8000","request_id":"a1b2c3d4e5f6"}
```

## Distributed Tracing

### Configuration

Enable OpenTelemetry tracing:

```bash
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318  # OTLP HTTP endpoint
OTEL_TRACE_SAMPLE_RATE=0.1                  # Sample 10% of traces
SERVICE_VERSION=v1.0.0                       # Service version tag
```

### Supported Collectors

The implementation uses OTLP HTTP exporter, which is compatible with:

- **Jaeger** (with OTLP receiver)
- **Zipkin** (with OTLP receiver)
- **OpenTelemetry Collector**
- **Honeycomb**
- **Grafana Tempo**
- **AWS X-Ray** (via OTel Collector)
- **Google Cloud Trace** (via OTel Collector)

### Setting Up Jaeger (Local Development)

The easiest way to get started is with Jaeger:

```bash
# Run Jaeger with OTLP support
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest

# Configure your app
export OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
```

Access Jaeger UI at http://localhost:16686

### Traced Operations

The following operations are instrumented with tracing spans:

#### API Server
- `handlers.GetGraphData` - Graph data retrieval
  - Attributes: max_nodes, max_links, cache_hit, type_filter
  - Events: cache miss, precalc query failed

#### Crawler
- `crawler.handleJob` - Job processing
  - Attributes: job_id, subreddit_id, subreddit, posts_count, job_status
  - Error recording on failures

#### Graph Precalculation
- `graph.PrecalculateGraphData` - Graph computation
  - Attributes: users_count, subreddits_count, detailed_graph, total_duration
  - Events: clearing_graph_tables

### Sampling

Sampling reduces the volume of traces collected:

- `OTEL_TRACE_SAMPLE_RATE=0.1` - Sample 10% of traces (default)
- `OTEL_TRACE_SAMPLE_RATE=1.0` - Sample 100% (all traces)
- `OTEL_TRACE_SAMPLE_RATE=0.01` - Sample 1%

Use lower rates in production to reduce overhead and costs.

## Error Reporting

### Configuration

Enable Sentry error reporting:

```bash
SENTRY_DSN=https://examplePublicKey@o0.ingest.sentry.io/0
SENTRY_ENVIRONMENT=production  # Options: development, staging, production
SENTRY_RELEASE=v1.0.0          # Release version (defaults to SERVICE_VERSION)
SENTRY_SAMPLE_RATE=1.0         # Sample rate (0.0 to 1.0)
```

Leave `SENTRY_DSN` empty to disable error reporting.

### PII Scrubbing

The following personally identifiable information is automatically scrubbed:

- **Email addresses** - Replaced with `[REDACTED]`
- **Bearer tokens** - OAuth tokens removed
- **API keys** - Various API key patterns removed
- **IP addresses** - IPv4 addresses redacted
- **Credit card numbers** - Basic pattern matching

Additionally, sensitive HTTP headers are removed:
- `Authorization`
- `Cookie`
- `X-Api-Key`

### Panic Recovery

Panics in HTTP handlers are automatically:
1. Recovered without crashing the server
2. Logged with stack trace
3. Reported to Sentry (if enabled)
4. Returned as 500 Internal Server Error to client

### Manual Error Reporting

```go
import "github.com/subculture-collective/clustr/backend/internal/errorreporting"

// Capture an error
errorreporting.CaptureError(err)

// Capture with context
errorreporting.CaptureErrorWithContext(
    err,
    map[string]string{"component": "crawler"},
    map[string]interface{}{"job_id": 123},
)

// Add breadcrumbs for debugging context
errorreporting.AddBreadcrumb("database", "query executed", sentry.LevelInfo)
```

## Docker Compose Setup

To enable full observability in Docker Compose, add to your `docker-compose.yml`:

```yaml
services:
  api:
    environment:
      # Logging
      - LOG_LEVEL=info
      - ENV=production
      
      # Tracing (optional)
      - OTEL_ENABLED=true
      - OTEL_EXPORTER_OTLP_ENDPOINT=jaeger:4318
      - OTEL_TRACE_SAMPLE_RATE=0.1
      
      # Error reporting (optional)
      - SENTRY_DSN=${SENTRY_DSN}
      - SENTRY_ENVIRONMENT=production
      - SERVICE_VERSION=v1.0.0

  # Optional: Jaeger for tracing
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"  # Jaeger UI
      - "4318:4318"    # OTLP HTTP
```

## Best Practices

### Logging

1. **Use appropriate log levels**
   - `debug` for detailed diagnostics (disabled in production)
   - `info` for normal operations
   - `warn` for potential issues
   - `error` for failures

2. **Include context** - Add relevant fields to help debugging:
   ```go
   logger.InfoContext(ctx, "Processing request",
       "user_id", userID,
       "operation", "update",
   )
   ```

3. **Use request IDs** - Always log with context to include request IDs:
   ```go
   logger.InfoContext(ctx, "message")  // Includes request_id automatically
   ```

### Tracing

1. **Add spans for key operations** - Not every function needs a span, focus on:
   - HTTP request handlers
   - Database queries (bulk operations)
   - External API calls
   - Long-running operations

2. **Add meaningful attributes**:
   ```go
   span.SetAttributes(
       attribute.String("user_id", userID),
       attribute.Int("batch_size", len(items)),
   )
   ```

3. **Record errors**:
   ```go
   if err != nil {
       span.RecordError(err)
       span.SetStatus(codes.Error, "operation failed")
       return err
   }
   ```

### Error Reporting

1. **Sample in production** - Set `SENTRY_SAMPLE_RATE` to reduce costs:
   - High-traffic APIs: 0.1 (10%)
   - Medium-traffic: 0.5 (50%)
   - Low-traffic: 1.0 (100%)

2. **Capture context** - Use tags and extras:
   ```go
   errorreporting.CaptureErrorWithContext(err,
       map[string]string{"component": "crawler"},
       map[string]interface{}{"job_id": jobID},
   )
   ```

3. **Set user context** when authenticated:
   ```go
   errorreporting.SetUser(userID, username)
   ```

## Troubleshooting

### Logs not appearing

Check:
1. `LOG_LEVEL` is set appropriately
2. Application stderr/stdout is being captured
3. Log format matches your log aggregation tool

### Traces not appearing in Jaeger

Check:
1. `OTEL_ENABLED=true` is set
2. `OTEL_EXPORTER_OTLP_ENDPOINT` points to correct host:port
3. Jaeger is running and accessible
4. Sampling rate is high enough to capture traces
5. Check application logs for OTLP export errors

### Errors not appearing in Sentry

Check:
1. `SENTRY_DSN` is set correctly
2. DSN format is valid (https://...)
3. Network connectivity to sentry.io
4. Sampling rate is high enough
5. Error reporting is actually triggered

### High overhead

If observability features cause performance issues:

1. **Reduce log level** - Set to `info` or `warn` in production
2. **Lower sampling rates**:
   - Tracing: `OTEL_TRACE_SAMPLE_RATE=0.01` (1%)
   - Errors: `SENTRY_SAMPLE_RATE=0.1` (10%)
3. **Use asynchronous exporters** (already configured)
4. **Disable features** not actively used

## Integration with Existing Monitoring

The observability features complement the existing Prometheus metrics:

- **Metrics** (Prometheus) - Quantitative data, aggregates, time series
- **Logs** (slog) - Event records, debugging information
- **Traces** (OpenTelemetry) - Request flow, latency analysis
- **Errors** (Sentry) - Error tracking, stack traces, reproduction

Together, they provide complete observability:
1. Prometheus alerts on anomalies
2. Logs provide event context
3. Traces show request flow
4. Sentry captures production errors

## Security Considerations

### PII Protection

All error reporting automatically scrubs:
- Email addresses
- OAuth tokens
- API keys
- IP addresses
- Sensitive headers

### Secrets Management

Never log or trace:
- Passwords
- API keys
- OAuth tokens
- Database credentials

Use environment variables for configuration, never hardcode.

### Network Security

For production:
1. Use TLS for OTLP exports
2. Restrict Sentry/collector network access
3. Use firewall rules to limit exposure
4. Rotate Sentry DSN if compromised

## Further Reading

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Sentry Documentation](https://docs.sentry.io/)
- [Go slog Package](https://pkg.go.dev/log/slog)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Prometheus + OpenTelemetry](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/)
