package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/onnwee/reddit-cluster-map/backend/internal/apierr"
	"github.com/onnwee/reddit-cluster-map/backend/internal/crawler"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/middleware"
)

type CrawlRequest struct {
	Subreddit string `json:"subreddit"`
}

// CrawlQueue abstracts the queries used by PostCrawl for testability.
type CrawlQueue interface {
	EnsureSubreddit(ctx context.Context, p db.EnsureSubredditParams) (int32, error)
	CrawlJobExists(ctx context.Context, subredditID int32) (bool, error)
	EnqueueCrawlJob(ctx context.Context, p db.EnqueueCrawlJobParams) error
}

func PostCrawl(q CrawlQueue) http.HandlerFunc {
	sanitizer := &middleware.SanitizeInput{}

	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[PostCrawl] Received request: %v", r)

		// Validate content type
		if err := middleware.ValidateJSON(r); err != nil {
			apierr.WriteErrorWithContext(w, r, apierr.ValidationInvalidFormat(""))
			return
		}

		var req CrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierr.WriteErrorWithContext(w, r, apierr.ValidationInvalidJSON())
			return
		}

		// Sanitize and validate subreddit name
		req.Subreddit = sanitizer.SanitizeString(req.Subreddit, 21)
		if req.Subreddit == "" {
			req.Subreddit = "AskReddit"
		}

		if err := sanitizer.ValidateSubredditName(req.Subreddit); err != nil {
			apierr.WriteErrorWithContext(w, r, apierr.CrawlInvalidSubreddit(""))
			return
		}

		// First get or create the subreddit to get its ID
		subreddit, err := q.EnsureSubreddit(r.Context(), db.EnsureSubredditParams{
			Name:        req.Subreddit,
			Title:       sql.NullString{String: req.Subreddit, Valid: true},
			Description: sql.NullString{String: "", Valid: true},
			Subscribers: sql.NullInt32{Int32: 0, Valid: true},
		})
		if err != nil {
			log.Printf("❌ Failed to upsert subreddit %s: %v", req.Subreddit, err)
			apierr.WriteErrorWithContext(w, r, apierr.SystemDatabase("Failed to create subreddit"))
			return
		}

		// Production queries use the durable request lifecycle. Tests and older
		// adapters retain the historical crawl_jobs behavior until migration.
		if productionQueries, ok := q.(*db.Queries); ok {
			if err := crawler.EnsureJob(r.Context(), productionQueries, subreddit, "api"); err != nil {
				log.Printf("❌ Failed to enqueue %s: %v", req.Subreddit, err)
				apierr.WriteErrorWithContext(w, r, apierr.CrawlQueueFailed(""))
				return
			}
		} else if exists, err := q.CrawlJobExists(r.Context(), subreddit); err == nil {
			if !exists {
				if err := q.EnqueueCrawlJob(r.Context(), db.EnqueueCrawlJobParams{
					SubredditID: subreddit,
					EnqueuedBy:  sql.NullString{String: "api", Valid: true},
				}); err != nil {
					log.Printf("❌ Failed to enqueue %s: %v", req.Subreddit, err)
					apierr.WriteErrorWithContext(w, r, apierr.CrawlQueueFailed(""))
					return
				}
			} else {
				// Promote requested subreddit job by bumping priority.
				if productionQueries, ok := q.(*db.Queries); ok {
					_ = crawler.BumpPriority(r.Context(), productionQueries, subreddit, 1)
				}
			}
		}

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Seeded and crawler started.\n"))
	}
}
