package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/httpx"
	"github.com/onnwee/reddit-cluster-map/backend/internal/secrets"
)

// tokenManager handles OAuth token lifecycle with proactive refresh.
type tokenManager struct {
	mu           sync.RWMutex
	accessToken  string
	tokenExpiry  time.Time
	refreshTimer *time.Timer

	// credentials - can be updated for rotation
	clientID     string
	clientSecret string
}

var globalTokenManager = &tokenManager{}

// ValidateOAuthCredentials validates OAuth credentials at startup.
// This should be called before starting the crawler to ensure credentials are valid.
func ValidateOAuthCredentials() error {
	return initTokenManager()
}

// RotateOAuthCredentials allows updating OAuth credentials at runtime.
// This enables zero-downtime credential rotation.
func RotateOAuthCredentials(newClientID, newClientSecret string) error {
	return globalTokenManager.rotateCredentials(newClientID, newClientSecret)
}

// initTokenManager initializes the token manager with credentials from config.
// This should be called at startup to validate credentials.
func initTokenManager() error {
	cfg := config.Load()

	globalTokenManager.mu.Lock()
	defer globalTokenManager.mu.Unlock()

	globalTokenManager.clientID = cfg.RedditClientID
	globalTokenManager.clientSecret = cfg.RedditClientSecret

	// Validate credentials
	if globalTokenManager.clientID == "" || globalTokenManager.clientSecret == "" {
		return fmt.Errorf("REDDIT_CLIENT_ID and REDDIT_CLIENT_SECRET are required")
	}

	log.Printf("✓ OAuth credentials validated (client_id: %s)", secrets.Mask(globalTokenManager.clientID))
	return nil
}

// getAccessToken returns a valid access token, refreshing if necessary.
func (tm *tokenManager) getAccessToken() (string, error) {
	return tm.getAccessTokenContext(context.Background())
}

func (tm *tokenManager) getAccessTokenContext(ctx context.Context) (string, error) {
	tm.mu.RLock()
	// Check if we have a valid token (with 60s buffer)
	if tm.accessToken != "" && time.Now().Add(60*time.Second).Before(tm.tokenExpiry) {
		token := tm.accessToken
		tm.mu.RUnlock()
		return token, nil
	}
	tm.mu.RUnlock()

	// Need to refresh - acquire write lock
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if tm.accessToken != "" && time.Now().Add(60*time.Second).Before(tm.tokenExpiry) {
		return tm.accessToken, nil
	}

	// Fetch new token
	return tm.refreshTokenLocked(ctx)
}

func (tm *tokenManager) invalidate() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.accessToken = ""
	tm.tokenExpiry = time.Time{}
}

// refreshTokenLocked fetches a new access token. Must be called with write lock held.
func (tm *tokenManager) refreshTokenLocked(ctx context.Context) (string, error) {
	if tm.clientID == "" || tm.clientSecret == "" {
		return "", fmt.Errorf("OAuth credentials not initialized")
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("scope", "read")

	ua := config.Load().UserAgent
	build := func() (*http.Request, error) {
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://www.reddit.com/api/v1/access_token", strings.NewReader(data.Encode()))
		req.SetBasicAuth(tm.clientID, tm.clientSecret)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", ua)
		return req, nil
	}

	// Token requests also respect the global pacing to avoid burst traffic during retries.
	pre := func(ctx context.Context, attempt int) error {
		waitForRateLimit()
		return nil
	}

	resp, err := httpx.DoWithRetryFactory(httpClient, build, pre)
	if err != nil {
		// Don't log secrets in error messages
		log.Printf("⚠️ Failed to request access token: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("⚠️ Token request failed with status: %s", resp.Status)
		return "", fmt.Errorf("token request failed: %s", resp.Status)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tokenResp); err != nil {
		log.Printf("⚠️ Failed to decode token response: %v", err)
		return "", err
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("received empty access token")
	}

	// Store token with safety buffer (renew 60s before expiry)
	tm.accessToken = tokenResp.AccessToken
	expiryDuration := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiryDuration > 120*time.Second {
		expiryDuration -= 60 * time.Second // Renew 60s early
	} else {
		expiryDuration = expiryDuration / 2 // For short-lived tokens, renew at half-life
	}
	tm.tokenExpiry = time.Now().Add(expiryDuration)

	// Schedule proactive refresh
	if tm.refreshTimer != nil {
		tm.refreshTimer.Stop()
	}
	tm.refreshTimer = time.AfterFunc(expiryDuration, func() {
		tm.mu.Lock()
		defer tm.mu.Unlock()
		// Proactively refresh token
		log.Printf("🔄 Proactively refreshing OAuth token")
		if _, err := tm.refreshTokenLocked(context.Background()); err != nil {
			log.Printf("⚠️ Proactive token refresh failed: %v", err)
		} else {
			log.Printf("✓ Token refreshed successfully")
		}
	})

	log.Printf("✓ Obtained access token (expires in %v)", expiryDuration)
	return tm.accessToken, nil
}

// rotateCredentials allows updating credentials without downtime.
// This enables secret rotation while the service is running.
func (tm *tokenManager) rotateCredentials(newClientID, newClientSecret string) error {
	if newClientID == "" || newClientSecret == "" {
		return fmt.Errorf("new credentials cannot be empty")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	oldID := tm.clientID
	oldSecret := tm.clientSecret
	tm.clientID = newClientID
	tm.clientSecret = newClientSecret

	// Immediately try to get a token with new credentials
	_, err := tm.refreshTokenLocked(context.Background())
	if err != nil {
		// Rollback on failure
		tm.clientID = oldID
		tm.clientSecret = oldSecret
		return fmt.Errorf("failed to authenticate with new credentials: %w", err)
	}

	log.Printf("✓ Credentials rotated successfully (old: %s, new: %s)",
		secrets.Mask(oldID), secrets.Mask(newClientID))
	return nil
}

// getAccessToken is the package-level function used by the rest of the crawler.
func getAccessToken() (string, error) {
	return globalTokenManager.getAccessToken()
}

func getAccessTokenContext(ctx context.Context) (string, error) {
	return globalTokenManager.getAccessTokenContext(ctx)
}
