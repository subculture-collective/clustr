package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/apierr"
	"github.com/subculture-collective/clustr/backend/internal/authstore"
	"github.com/subculture-collective/clustr/backend/internal/config"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"github.com/subculture-collective/clustr/backend/internal/secrets"
)

// AuthHandlers bundles dependencies for OAuth endpoints.
type AuthHandlers struct {
	q *db.Queries
}

func NewAuthHandlers(q *db.Queries) *AuthHandlers { return &AuthHandlers{q: q} }

// Login initiates Reddit OAuth authorization code flow.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	cfg := config.Load()
	if cfg.RedditClientID == "" || cfg.RedditRedirectURI == "" {
		apierr.WriteErrorWithContext(w, r, apierr.AuthOAuthNotConfigured())
		return
	}
	log.Printf("Initiating OAuth login (client_id: %s)", secrets.Mask(cfg.RedditClientID))
	state := "rcm-" + time.Now().Format("20060102150405")
	// NOTE: In production, persist state in session/cookie to validate on callback.
	v := url.Values{}
	v.Set("client_id", cfg.RedditClientID)
	v.Set("response_type", "code")
	v.Set("state", state)
	v.Set("redirect_uri", cfg.RedditRedirectURI)
	scope := cfg.RedditScopes
	if scope == "" {
		scope = "read identity"
	}
	v.Set("scope", scope)
	http.Redirect(w, r, "https://www.reddit.com/api/v1/authorize?"+v.Encode(), http.StatusFound)
}

// Callback handles Reddit redirect, exchanges code for tokens, fetches user, and stores account.
func (h *AuthHandlers) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := config.Load()
	code := r.URL.Query().Get("code")
	if code == "" {
		apierr.WriteErrorWithContext(w, r, apierr.ValidationMissingField("code"))
		return
	}
	tok, err := exchangeCode(ctx, cfg, code)
	if err != nil {
		log.Printf("OAuth exchange failed: %v", err)
		apierr.WriteErrorWithContext(w, r, apierr.AuthOAuthFailed("Token exchange failed"))
		return
	}
	me, err := fetchMe(ctx, cfg, tok.AccessToken)
	if err != nil {
		log.Printf("Fetch me failed: %v", err)
		apierr.WriteErrorWithContext(w, r, apierr.AuthOAuthFailed("Failed to fetch identity"))
		return
	}
	store := authstore.New(h.q)
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	if _, err := store.Upsert(ctx, me.ID, me.Name, tok.AccessToken, tok.RefreshToken, tok.Scope, expiresAt); err != nil {
		log.Printf("Store token failed: %v", err)
		apierr.WriteErrorWithContext(w, r, apierr.SystemDatabase("Failed to persist account"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": me.Name})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

func exchangeCode(ctx context.Context, cfg *config.Config, code string) (*tokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", cfg.RedditRedirectURI)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.reddit.com/api/v1/access_token", bytes.NewBufferString(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", cfg.UserAgent)
	basic := base64.StdEncoding.EncodeToString([]byte(cfg.RedditClientID + ":" + cfg.RedditClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &httpError{Code: resp.StatusCode, Body: string(b)}
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

type meResponse struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

func fetchMe(ctx context.Context, cfg *config.Config, accessToken string) (*meResponse, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://oauth.reddit.com/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", cfg.UserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &httpError{Code: resp.StatusCode, Body: string(b)}
	}
	var me meResponse
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return nil, err
	}
	return &me, nil
}

type httpError struct {
	Code int
	Body string
}

func (e *httpError) Error() string { return http.StatusText(e.Code) + ": " + e.Body }

// refreshAccessToken refreshes an access token using a refresh token.
// This is used for user OAuth tokens stored in the database.
func refreshAccessToken(ctx context.Context, cfg *config.Config, refreshToken string) (*tokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.reddit.com/api/v1/access_token", bytes.NewBufferString(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", cfg.UserAgent)
	basic := base64.StdEncoding.EncodeToString([]byte(cfg.RedditClientID + ":" + cfg.RedditClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, &httpError{Code: resp.StatusCode, Body: string(b)}
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

// RefreshUserToken refreshes a user's OAuth token and updates it in the database.
// This endpoint is admin-only for security reasons.
func (h *AuthHandlers) RefreshUserToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := config.Load()

	// Get username from query parameter
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "username parameter required", http.StatusBadRequest)
		return
	}

	// Fetch existing account
	store := authstore.New(h.q)
	account, err := store.ByUsername(ctx, username)
	if err != nil {
		log.Printf("Failed to fetch account for %s: %v", username, err)
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	// Refresh the token
	tok, err := refreshAccessToken(ctx, cfg, account.RefreshToken)
	if err != nil {
		log.Printf("Failed to refresh token for %s: %v", username, err)
		http.Error(w, "token refresh failed", http.StatusBadGateway)
		return
	}

	// Update stored token
	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	newRefreshToken := tok.RefreshToken
	if newRefreshToken == "" {
		// Reddit may not return a new refresh token, keep the old one
		newRefreshToken = account.RefreshToken
	}

	if _, err := store.Upsert(ctx, account.RedditUserID, account.RedditUsername, tok.AccessToken, newRefreshToken, tok.Scope, expiresAt); err != nil {
		log.Printf("Failed to update token for %s: %v", username, err)
		http.Error(w, "failed to persist token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "expires_in": tok.ExpiresIn})
}
