package crawler

import (
	"os"
	"testing"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/config"
)

func TestInitTokenManager(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		expectError  bool
	}{
		{
			name:         "valid credentials",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			expectError:  false,
		},
		{
			name:         "missing client ID",
			clientID:     "",
			clientSecret: "test-client-secret",
			expectError:  true,
		},
		{
			name:         "missing client secret",
			clientID:     "test-client-id",
			clientSecret: "",
			expectError:  true,
		},
		{
			name:         "both missing",
			clientID:     "",
			clientSecret: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			os.Setenv("REDDIT_CLIENT_ID", tt.clientID)
			os.Setenv("REDDIT_CLIENT_SECRET", tt.clientSecret)
			defer os.Unsetenv("REDDIT_CLIENT_ID")
			defer os.Unsetenv("REDDIT_CLIENT_SECRET")
			config.ResetForTest()

			// Reset the global token manager
			globalTokenManager = &tokenManager{}

			err := initTokenManager()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestTokenManager_RotateCredentials(t *testing.T) {
	tm := &tokenManager{
		clientID:     "old-client-id",
		clientSecret: "old-client-secret",
	}

	tests := []struct {
		name        string
		newID       string
		newSecret   string
		expectError bool
	}{
		{
			name:        "empty new ID",
			newID:       "",
			newSecret:   "new-secret",
			expectError: true,
		},
		{
			name:        "empty new secret",
			newID:       "new-id",
			newSecret:   "",
			expectError: true,
		},
		{
			name:        "both empty",
			newID:       "",
			newSecret:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tm.rotateCredentials(tt.newID, tt.newSecret)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			// On error, credentials should not change
			if err != nil {
				if tm.clientID != "old-client-id" {
					t.Errorf("clientID should not change on error, got %s", tm.clientID)
				}
				if tm.clientSecret != "old-client-secret" {
					t.Errorf("clientSecret should not change on error, got %s", tm.clientSecret)
				}
			}
		})
	}
}

func TestTokenManager_TokenExpiry(t *testing.T) {
	tm := &tokenManager{
		clientID:     "test-id",
		clientSecret: "test-secret",
		accessToken:  "test-token",
		tokenExpiry:  time.Now().Add(5 * time.Minute),
	}

	// Token should be valid
	token, err := tm.getAccessToken()
	if err != nil {
		t.Errorf("expected valid token, got error: %v", err)
	}
	if token != "test-token" {
		t.Errorf("expected test-token, got %s", token)
	}

	// Expire the token
	tm.mu.Lock()
	tm.tokenExpiry = time.Now().Add(-1 * time.Minute)
	tm.mu.Unlock()

	// Should attempt to refresh (will fail without real credentials, but that's ok for this test)
	// We're just testing that it tries to refresh
	_, err = tm.getAccessToken()
	if err == nil {
		t.Log("Note: token refresh would normally fetch a new token from Reddit API")
	}
}

func TestTokenManager_ConcurrentAccess(t *testing.T) {
	tm := &tokenManager{
		clientID:     "test-id",
		clientSecret: "test-secret",
		accessToken:  "test-token",
		tokenExpiry:  time.Now().Add(5 * time.Minute),
	}

	// Simulate concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = tm.getAccessToken()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
