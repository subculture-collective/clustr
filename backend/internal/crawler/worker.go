package crawler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // Import the postgres driver
	"github.com/onnwee/reddit-cluster-map/backend/internal/admin"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// checkAndRequeueStaleSubreddits checks for subreddits that haven't been crawled in 7 days
// and requeues them for crawling.
func checkAndRequeueStaleSubreddits(ctx context.Context, q *db.Queries) error {
	staleSubs, err := q.GetStaleSubreddits(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stale subreddits: %w", err)
	}

	for _, sub := range staleSubs {
		log.Printf("🔄 Requeueing stale subreddit: r/%s", sub)
		// Get subreddit ID
		subreddit, err := q.GetSubreddit(ctx, sub)
		if err != nil {
			log.Printf("⚠️ Failed to get subreddit r/%s: %v", sub, err)
			continue
		}

		if err := EnsureJob(ctx, q, subreddit.ID, "system-stale"); err != nil {
			log.Printf("⚠️ Failed to requeue stale subreddit r/%s: %v", sub, err)
		}
	}

	return nil
}

// Crawler represents a Reddit crawler instance
type Crawler struct {
	queries *db.Queries
	stop    chan struct{}
}

// NewCrawler creates a new crawler instance
func NewCrawler(q *db.Queries) *Crawler {
	return &Crawler{
		queries: q,
		stop:    make(chan struct{}),
	}
}

// Start begins the crawler process
func (c *Crawler) Start(ctx context.Context) {
	// Main crawler loop: wakes up on a short interval, grabs one queued job, processes it.
	log.Println("🚀 Starting crawler...")
	// On start, reset stale in-progress jobs (e.g., container restarts)
	cfg := config.Load()
	_ = ResetIncompleteJobs(ctx, c.queries, time.Duration(cfg.ResetCrawlingAfterMin)*time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	staleTicker := time.NewTicker(6 * time.Hour)
	maintenanceTicker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	defer staleTicker.Stop()
	defer maintenanceTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Crawler stopped by context")
			return
		case <-c.stop:
			log.Println("🛑 Crawler stopped by signal")
			return
		case <-ticker.C:
			// Check if disabled via admin flag
			enabled, _ := admin.GetBool(ctx, c.queries, "crawler_enabled", true)
			if !enabled {
				// skip quietly; avoid noisy logs every tick
				continue
			}
			// Pull the next queued job, if any, and process it end-to-end.
			if err := c.processNextJob(ctx); err != nil {
				log.Printf("⚠️ Error processing job: %v", err)
			}
		case <-maintenanceTicker.C:
			if err := ReconsiderDiscoveryCandidates(ctx, c.queries, cfg.DiscoveryDailyBudget); err != nil {
				log.Printf("⚠️ Failed to promote discovery candidates: %v", err)
			}
			// Requeue jobs that are ready to retry
			if err := RequeueRetryableJobs(ctx, c.queries); err != nil {
				log.Printf("⚠️ Failed to requeue retryable jobs: %v", err)
			}
			// Age starved jobs (jobs waiting more than 1 hour get +10 priority)
			if err := AgeStarvedJobs(ctx, c.queries, 1*time.Hour, 10); err != nil {
				log.Printf("⚠️ Failed to age starved jobs: %v", err)
			}
		case <-staleTicker.C:
			// Periodically requeue subs not crawled in a while to keep data fresh.
			if err := checkAndRequeueStaleSubreddits(ctx, c.queries); err != nil {
				log.Printf("⚠️ Failed to requeue stale subreddits: %v", err)
			}
			// Also enqueue any subreddits not seen in configured TTL in created_at order
			_ = RequeueStaleSubreddits(ctx, c.queries, time.Duration(cfg.StaleDays)*24*time.Hour)
		}
	}
}

// Stop gracefully stops the crawler
func (c *Crawler) Stop() {
	close(c.stop)
}

// processNextJob handles a single crawl job
func (c *Crawler) processNextJob(ctx context.Context) error {
	if !config.Load().CrawlerLifecycleEnabled {
		job, err := ClaimNextJob(ctx, c.queries)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim legacy crawl job: %w", err)
		}
		return handleJob(ctx, c.queries, job)
	}
	attempt, err := ClaimNextCrawlAttempt(ctx, c.queries, "")
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to claim next crawl attempt: %w", err)
	}
	executionCtx, stopHeartbeat, leaseErrors := HeartbeatLease(ctx, c.queries, attempt)
	defer stopHeartbeat()

	// crawl_jobs remains the temporary compatibility Adapter for the existing
	// executor and admin routes. EnsureJob has made it queued for this request.
	job, err := legacyJobForSubreddit(executionCtx, c.queries, attempt.SubredditID)
	result := AttemptResult{Outcome: OutcomeComplete}
	if err == nil {
		result, err = executeJob(executionCtx, c.queries, job)
	}
	select {
	case leaseErr := <-leaseErrors:
		err = fmt.Errorf("crawl attempt lease lost: %w", leaseErr)
		result.Outcome = OutcomeRetryableFailure
		result.ErrorClass = "lease_lost"
	default:
	}
	if err != nil {
		result.ErrorMessage = err.Error()
		var redditError *RedditHTTPError
		switch {
		case result.ErrorClass == "lease_lost":
			// Preserve the heartbeat classification established above.
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			result.Outcome = OutcomeCancelled
			result.ErrorClass = "cancelled"
		case errors.As(err, &redditError) && !redditError.Retryable:
			result.Outcome = OutcomePermanentFailure
			result.ErrorClass = "reddit_permanent"
		default:
			result.Outcome = OutcomeRetryableFailure
			result.ErrorClass = "crawl_error"
		}
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if finishErr := FinishCrawlAttempt(finishCtx, c.queries, attempt, result, config.Load().CrawlFreshness); finishErr != nil {
		return fmt.Errorf("finish crawl attempt: %w", finishErr)
	}
	return err
}

func legacyJobForSubreddit(ctx context.Context, q *db.Queries, subredditID int32) (db.CrawlJob, error) {
	var job db.CrawlJob
	err := q.DB().QueryRowContext(ctx, `SELECT id, subreddit_id, status, retries, last_attempt, duration_ms, enqueued_by, created_at, updated_at FROM crawl_jobs WHERE subreddit_id=$1`, subredditID).
		Scan(&job.ID, &job.SubredditID, &job.Status, &job.Retries, &job.LastAttempt, &job.DurationMs, &job.EnqueuedBy, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}
