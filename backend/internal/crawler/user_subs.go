package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// FetchUserSubreddits fetches the list of subreddits a user has posted or commented in.
func FetchUserSubreddits(username string, limit int) ([]string, error) {
	return FetchUserSubredditsContext(context.Background(), username, limit)
}

func FetchUserSubredditsContext(ctx context.Context, username string, limit int) ([]string, error) {
	endpoints := []string{
		fmt.Sprintf("https://oauth.reddit.com/user/%s/submitted.json?limit=%d", username, limit),
		fmt.Sprintf("https://oauth.reddit.com/user/%s/comments.json?limit=%d", username, limit),
	}

	subreddits := make(map[string]bool)

	for _, url := range endpoints {
		resp, err := authenticatedGetWithContext(ctx, url)
		if err != nil {
			log.Printf("⚠️ Failed to fetch %s: %v", url, err)
			continue
		}

		if resp.StatusCode != 200 {
			log.Printf("⚠️ Non-200 status for %s: %d", url, resp.StatusCode)
			continue
		}

		var parsed struct {
			Data struct {
				Children []struct {
					Data struct {
						Subreddit string `json:"subreddit"`
					} `json:"data"`
				} `json:"children"`
			} `json:"data"`
		}

		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
			resp.Body.Close()
			log.Printf("⚠️ Failed to decode JSON from %s: %v", url, err)
			continue
		}
		resp.Body.Close()

		for _, child := range parsed.Data.Children {
			if sub := child.Data.Subreddit; sub != "" {
				subreddits[sub] = true
			}
		}
	}

	var results []string
	for sub := range subreddits {
		results = append(results, sub)
	}
	return results, nil
}

// FetchRecentUserSubreddits fetches the list of subreddits a user has recently posted or commented in.
func FetchRecentUserSubreddits(username string, limit int) ([]string, error) {
	return FetchRecentUserSubredditsContext(context.Background(), username, limit)
}

func FetchRecentUserSubredditsContext(ctx context.Context, username string, limit int) ([]string, error) {
	uname := strings.TrimSpace(username)
	// Attempt 1: OAuth user listing
	u1 := fmt.Sprintf("https://oauth.reddit.com/user/%s/.json?limit=%d&raw_json=1", url.PathEscape(uname), limit)
	if subs, ok, err := tryUserListingOAuthContext(ctx, u1); err == nil && ok {
		return subs, nil
	} else if err != nil && !ok {
		// hard error (e.g., network). Return error to allow upstream handling.
		return nil, err
	}

	// Attempt 2: OAuth search (author:username) for links/comments
	q := url.Values{}
	q.Set("q", fmt.Sprintf("author:%s", uname))
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("sort", "new")
	q.Set("type", "link")
	u2 := "https://oauth.reddit.com/search.json?" + q.Encode()
	if subs, ok, err := trySearchOAuthContext(ctx, u2); err == nil && ok {
		return subs, nil
	} else if err != nil && !ok {
		return nil, err
	}

	// Attempt 3: Public old.reddit.com listing
	u3 := fmt.Sprintf("https://old.reddit.com/user/%s/.json?limit=%d&raw_json=1", url.PathEscape(uname), limit)
	if subs, ok, err := tryUserListingPublicContext(ctx, u3); err == nil && ok {
		return subs, nil
	} else if err != nil && !ok {
		return nil, err
	}

	// If we reached here, treat as no accessible data (e.g., shadowbanned/user privacy) without failing pipeline.
	log.Printf("ℹ️ No accessible listing for u/%s (possibly private/shadowbanned). Skipping.", uname)
	return []string{}, nil
}

// parseUserListing extracts unique subreddits from a standard Reddit listing response.
func parseUserListing(resp *http.Response) ([]string, error) {
	var parsed struct {
		Data struct {
			Children []struct {
				Data struct {
					Subreddit string `json:"subreddit"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}
	seen := map[string]bool{}
	var subs []string
	for _, item := range parsed.Data.Children {
		sub := item.Data.Subreddit
		if sub != "" && !seen[sub] {
			subs = append(subs, sub)
			seen[sub] = true
		}
	}
	return subs, nil
}

// try helpers return (subs, ok, err). ok=false when caller should try next strategy; err!=nil with ok=false means hard error.
func tryUserListingOAuth(u string) ([]string, bool, error) {
	return tryUserListingOAuthContext(context.Background(), u)
}
func tryUserListingOAuthContext(ctx context.Context, u string) ([]string, bool, error) {
	resp, err := authenticatedGetWithContext(ctx, u)
	if err != nil {
		var status *RedditHTTPError
		if errors.As(err, &status) && (status.StatusCode == http.StatusUnauthorized || status.StatusCode == http.StatusForbidden || status.StatusCode == http.StatusNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed authenticated GET: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		subs, err := parseUserListing(resp)
		return subs, true, err
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		// NOTE: This log is intentionally muted to reduce noise from frequent 401/403/404 on app-only OAuth.
		// log.Printf("ℹ️ OAuth listing status %d; will try alternate strategies", resp.StatusCode)
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
}

func trySearchOAuth(u string) ([]string, bool, error) {
	return trySearchOAuthContext(context.Background(), u)
}
func trySearchOAuthContext(ctx context.Context, u string) ([]string, bool, error) {
	resp, err := authenticatedGetWithContext(ctx, u)
	if err != nil {
		var status *RedditHTTPError
		if errors.As(err, &status) && (status.StatusCode == http.StatusUnauthorized || status.StatusCode == http.StatusForbidden || status.StatusCode == http.StatusNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed OAuth search GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("search non-200: %d", resp.StatusCode)
	}
	var parsed struct {
		Data struct {
			Children []struct {
				Data struct {
					Subreddit string `json:"subreddit"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
		return nil, false, fmt.Errorf("search decode failed: %w", err)
	}
	seen := map[string]bool{}
	var subs []string
	for _, ch := range parsed.Data.Children {
		sub := ch.Data.Subreddit
		if sub != "" && !seen[sub] {
			subs = append(subs, sub)
			seen[sub] = true
		}
	}
	return subs, true, nil
}

func tryUserListingPublic(u string) ([]string, bool, error) {
	return tryUserListingPublicContext(context.Background(), u)
}
func tryUserListingPublicContext(ctx context.Context, u string) ([]string, bool, error) {
	resp, err := unauthenticatedGetContext(ctx, u)
	if err != nil {
		var status *RedditHTTPError
		if errors.As(err, &status) && (status.StatusCode == http.StatusUnauthorized || status.StatusCode == http.StatusForbidden || status.StatusCode == http.StatusNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed public GET: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		subs, err := parseUserListing(resp)
		return subs, true, err
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		// treat as no data; ok=false so caller tries next (if any)
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("public unexpected status: %d", resp.StatusCode)
	}
}
