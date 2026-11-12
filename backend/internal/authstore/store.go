package authstore

import (
	"context"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// Account is a minimal representation of a stored OAuth account.
type Account struct {
	RedditUserID   string
	RedditUsername string
	AccessToken    string
	RefreshToken   string
	ExpiresAt      time.Time
	Scopes         string
}

// Store wraps db.Queries for OAuth account persistence.
type Store struct{ q *db.Queries }

func New(q *db.Queries) *Store { return &Store{q: q} }

// Upsert saves/updates an OAuth account using raw SQL to avoid needing regenerated sqlc code.
func (s *Store) Upsert(ctx context.Context, redditUserID, username, accessToken, refreshToken, scopes string, expiresAt time.Time) (Account, error) {
	// Use sqlc-generated upsert
	rec, err := s.q.UpsertOAuthAccount(ctx, db.UpsertOAuthAccountParams{
		RedditUserID:   redditUserID,
		RedditUsername: username,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		ExpiresAt:      expiresAt,
		Scopes:         scopes,
	})
	if err != nil {
		return Account{}, err
	}
	return Account{
		RedditUserID:   rec.RedditUserID,
		RedditUsername: rec.RedditUsername,
		AccessToken:    rec.AccessToken,
		RefreshToken:   rec.RefreshToken,
		ExpiresAt:      rec.ExpiresAt,
		Scopes:         rec.Scopes,
	}, nil
}

func (s *Store) ByUserID(ctx context.Context, redditUserID string) (Account, error) {
	rec, err := s.q.GetOAuthAccountByUserID(ctx, redditUserID)
	if err != nil {
		return Account{}, err
	}
	return Account{
		RedditUserID:   rec.RedditUserID,
		RedditUsername: rec.RedditUsername,
		AccessToken:    rec.AccessToken,
		RefreshToken:   rec.RefreshToken,
		ExpiresAt:      rec.ExpiresAt,
		Scopes:         rec.Scopes,
	}, nil
}

func (s *Store) ByUsername(ctx context.Context, username string) (Account, error) {
	rec, err := s.q.GetOAuthAccountByUsername(ctx, username)
	if err != nil {
		return Account{}, err
	}
	return Account{
		RedditUserID:   rec.RedditUserID,
		RedditUsername: rec.RedditUsername,
		AccessToken:    rec.AccessToken,
		RefreshToken:   rec.RefreshToken,
		ExpiresAt:      rec.ExpiresAt,
		Scopes:         rec.Scopes,
	}, nil
}
