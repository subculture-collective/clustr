# OAuth Token Management

This document describes the OAuth token management implementation for the Reddit Cluster Map project.

## Overview

The project uses two OAuth flows:
1. **Client Credentials Flow**: Used by the crawler for application-level access to Reddit API
2. **Authorization Code Flow**: Used for user-based authentication and access

## Architecture

### Token Manager (`internal/crawler/token_manager.go`)

The token manager provides thread-safe, proactive OAuth token management for the crawler:

- **Proactive Refresh**: Automatically refreshes tokens 60 seconds before expiry
- **Thread-Safe**: Uses mutex locks to protect concurrent access
- **Background Timer**: Schedules automatic token renewal
- **Zero-Downtime Rotation**: Supports credential rotation without service restart

### Secret Management (`internal/secrets/`)

Provides utilities for safely handling secrets in logs and validation:

- **`Mask(secret)`**: Masks secrets for logging (e.g., "abcd...")
- **`MaskURL(url)`**: Masks passwords in connection strings
- **`ValidateRequired(secrets)`**: Validates required secrets at startup

## Usage

### Crawler OAuth (Client Credentials)

The crawler automatically manages OAuth tokens using the token manager.

**Configuration:**
```bash
# Required environment variables
REDDIT_CLIENT_ID=your_client_id_here
REDDIT_CLIENT_SECRET=your_client_secret_here
```

**Startup Validation:**
```go
// In cmd/crawler/main.go
if err := crawler.ValidateOAuthCredentials(); err != nil {
    log.Fatalf("OAuth credential validation failed: %v", err)
}
```

**Credential Rotation:**
```go
// Rotate credentials at runtime without downtime
err := crawler.RotateOAuthCredentials(newClientID, newClientSecret)
if err != nil {
    log.Printf("Failed to rotate credentials: %v", err)
}
```

### User OAuth (Authorization Code Flow)

User tokens are stored in the database and can be refreshed via the API.

**Configuration:**
```bash
# Required environment variables
REDDIT_CLIENT_ID=your_client_id_here
REDDIT_CLIENT_SECRET=your_client_secret_here
REDDIT_REDIRECT_URI=http://localhost:8080/api/auth/callback
REDDIT_SCOPES=read identity
```

**Endpoints:**
- `GET /api/auth/login` - Initiates OAuth login flow
- `GET /api/auth/callback` - Handles OAuth callback and stores tokens
- `POST /api/auth/refresh?username=USERNAME` - Refreshes a user's token (admin-only)

**Refresh a User Token (requires admin authentication):**
```bash
curl -X POST "http://localhost:8080/api/auth/refresh?username=someuser" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

Response:
```json
{
  "ok": true,
  "expires_in": 3600
}
```

## Token Lifecycle

### Client Credentials Token

1. **Initial Request**: Token manager fetches token on first API call
2. **Caching**: Token stored in memory with expiry time
3. **Proactive Refresh**: Background timer refreshes 60s before expiry
4. **Reactive Refresh**: If proactive refresh fails, refreshes on next API call
5. **Thread Safety**: Mutex ensures only one refresh happens at a time

```
Token Lifecycle:
┌─────────────┐
│   Request   │
└──────┬──────┘
       │
       ▼
   ┌───────┐     Valid?      ┌──────────┐
   │ Check │────────Yes──────▶│  Return  │
   │ Cache │                  │  Token   │
   └───┬───┘                  └──────────┘
       │ No
       ▼
   ┌───────────┐
   │  Refresh  │
   │  Token    │
   └─────┬─────┘
         │
         ▼
   ┌───────────┐
   │   Cache   │
   │ New Token │
   └─────┬─────┘
         │
         ▼
   ┌───────────┐
   │ Schedule  │
   │  Refresh  │
   └───────────┘
```

### User Token

1. **Authorization**: User grants access via Reddit OAuth
2. **Storage**: Access and refresh tokens stored in database
3. **Usage**: Access token used for API calls on behalf of user
4. **Manual Refresh**: Refresh via `/api/auth/refresh` endpoint when needed
5. **Automatic Refresh**: Could be automated in future (not currently implemented)

## Security Best Practices

### Secret Masking

Always mask secrets when logging:

```go
import "github.com/subculture-collective/clustr/backend/internal/secrets"

// DO: Mask secrets in logs
log.Printf("Client ID: %s", secrets.Mask(clientID))

// DON'T: Log secrets in plain text
log.Printf("Client ID: %s", clientID)  // ❌ NEVER DO THIS
```

### Database URLs

Mask database connection strings:

```go
dbURL := os.Getenv("DATABASE_URL")
log.Printf("Connecting to: %s", secrets.MaskURL(dbURL))
// Output: Connecting to: postgres://user:***@localhost:5432/db
```

### Startup Validation

Validate all required secrets at startup:

```go
secrets := map[string]string{
    "REDDIT_CLIENT_ID":     os.Getenv("REDDIT_CLIENT_ID"),
    "REDDIT_CLIENT_SECRET": os.Getenv("REDDIT_CLIENT_SECRET"),
}

if err := secrets.ValidateRequired(secrets); err != nil {
    log.Fatalf("Required secrets missing: %v", err)
}
```

## Error Handling

### Token Refresh Failures

The token manager handles refresh failures gracefully:

1. **Proactive Refresh Failure**: Logged as warning, will retry on next API call
2. **Reactive Refresh Failure**: Returns error to caller, who should handle appropriately
3. **Authentication Failure**: Check credentials and Reddit API status

Example error handling:

```go
token, err := getAccessToken()
if err != nil {
    // Log error without exposing secrets
    log.Printf("Failed to get access token: %v", err)
    
    // Check if credentials are valid
    if strings.Contains(err.Error(), "401") {
        log.Printf("Invalid credentials, please check REDDIT_CLIENT_ID and REDDIT_CLIENT_SECRET")
    }
    
    return err
}
```

### Credential Rotation Failures

When rotating credentials, the old credentials are retained on failure:

```go
err := crawler.RotateOAuthCredentials(newClientID, newClientSecret)
if err != nil {
    // Old credentials still active
    log.Printf("Credential rotation failed: %v", err)
    log.Printf("Continuing with existing credentials")
}
```

## Monitoring

### Metrics to Track

Consider monitoring these metrics in production:

- Token refresh success/failure rate
- Time until token expiry
- Number of token refreshes per day
- Credential rotation events

### Logs

The token manager logs important events:

- `✓ OAuth credentials validated (client_id: abcd...)` - Startup validation success
- `✓ Obtained access token (expires in 59m0s)` - Token refresh success
- `🔄 Proactively refreshing OAuth token` - Background refresh triggered
- `✓ Token refreshed successfully` - Background refresh success
- `⚠️ Proactive token refresh failed: <error>` - Background refresh failure
- `✓ Credentials rotated successfully (old: abcd..., new: efgh...)` - Rotation success

## Testing

### Unit Tests

The token manager includes comprehensive tests:

```bash
go test ./internal/crawler/token_manager*.go -v
go test ./internal/secrets/... -v
```

### Integration Testing

Test the full OAuth flow:

```bash
# Set test credentials
export REDDIT_CLIENT_ID="test-id"
export REDDIT_CLIENT_SECRET="test-secret"

# Run crawler with validation
go run cmd/crawler/main.go
```

### Manual Testing

Test credential rotation:

```bash
# 1. Start crawler with initial credentials
# 2. Update .env with new credentials
# 3. Call rotation endpoint (if implemented)
# 4. Verify crawler continues working with new credentials
```

## Future Improvements

Potential enhancements to consider:

1. **Automatic User Token Refresh**: Background job to refresh user tokens before expiry
2. **Token Metrics**: Export Prometheus metrics for token lifecycle events
3. **Secrets Manager Integration**: Use AWS Secrets Manager, Vault, etc. instead of env vars
4. **Token Cache Persistence**: Persist tokens to database for recovery after restart
5. **Multi-Instance Coordination**: Share token cache across multiple crawler instances

## Troubleshooting

### Token Refresh Failures

**Problem**: Token refresh fails with 401 Unauthorized

**Solutions**:
1. Verify `REDDIT_CLIENT_ID` and `REDDIT_CLIENT_SECRET` are correct
2. Check Reddit API status at https://www.redditstatus.com/
3. Ensure credentials are for a "script" or "web" app type
4. Verify no special characters in credentials that need escaping

### Startup Validation Failures

**Problem**: Crawler fails to start with "OAuth credential validation failed"

**Solutions**:
1. Check that `.env` file exists and is loaded
2. Verify `REDDIT_CLIENT_ID` and `REDDIT_CLIENT_SECRET` are set
3. Check for whitespace or quotes in environment variables
4. Ensure credentials are valid by testing with Reddit API manually

### Credential Rotation Issues

**Problem**: Credential rotation fails but old credentials work

**Solutions**:
1. Verify new credentials are valid before rotating
2. Check that new credentials have same scope as old ones
3. Test new credentials with Reddit API before rotation
4. Use gradual rollout: test new credentials in dev first

## References

- [Reddit OAuth2 Documentation](https://github.com/reddit-archive/reddit/wiki/OAuth2)
- [Reddit API Rules](https://github.com/reddit-archive/reddit/wiki/API)
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
