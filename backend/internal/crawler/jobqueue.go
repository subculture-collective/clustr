package crawler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// EnsureJob enqueues a job for subredditID if absent, or resets to queued if failed/stale.
func EnsureJob(ctx context.Context, q *db.Queries, subredditID int32, enqueuedBy string) error {
	// The durable request is authoritative after migration 000028.  Keep the
	// legacy row populated while old admin/routes still read crawl_jobs.
	if err := EnsureCrawlRequest(ctx, q, subredditID, enqueuedBy, 0); err != nil {
		return err
	}
	err := q.EnqueueCrawlJob(ctx, db.EnqueueCrawlJobParams{
		SubredditID: subredditID,
		EnqueuedBy:  sql.NullString{String: enqueuedBy, Valid: enqueuedBy != ""},
	})
	if err != nil {
		return err
	}
	// A legacy row may be a historical success.  It is only a compatibility
	// projection now, so make it runnable when the request becomes due.
	_, err = q.DB().ExecContext(ctx, `UPDATE crawl_jobs SET status='queued', visible_at=now(), updated_at=now()
WHERE subreddit_id=$1 AND status <> 'crawling'`, subredditID)
	return err
}

// PromoteNext attempts to pick the oldest queued job by creation time.
func PromoteNext(ctx context.Context, q *db.Queries) (db.CrawlJob, error) {
	const qstr = `SELECT id, subreddit_id, status, retries, last_attempt, duration_ms, enqueued_by, created_at, updated_at, priority
                  FROM crawl_jobs
                  WHERE status='queued'
                  ORDER BY priority DESC, created_at ASC
                  LIMIT 1`
	row := q.DB().QueryRowContext(ctx, qstr)
	var j db.CrawlJob
	// Older generated struct may not have Priority; we ignore scanning it if not present.
	// Scan without priority into existing fields.
	if err := row.Scan(&j.ID, &j.SubredditID, &j.Status, &j.Retries, &j.LastAttempt, &j.DurationMs, &j.EnqueuedBy, &j.CreatedAt, &j.UpdatedAt, new(sql.NullInt32)); err != nil {
		return db.CrawlJob{}, err
	}
	return j, nil
}

// ResetIncompleteJobs moves old 'crawling' jobs (no progress) back to 'queued'.
func ResetIncompleteJobs(ctx context.Context, q *db.Queries, olderThan time.Duration) error {
	// raw SQL since sqlc has no named query for this
	const stmt = `UPDATE crawl_jobs SET status='queued', updated_at=now() WHERE status='crawling' AND updated_at < now() - $1::interval`
	_, err := q.DB().ExecContext(ctx, stmt, olderThan.String())
	return err
}

// RequeueStaleSubreddits enqueues subs with last_seen older than ttl.
func RequeueStaleSubreddits(ctx context.Context, q *db.Queries, ttl time.Duration) error {
	const sel = `SELECT id FROM subreddits WHERE last_seen < now() - $1::interval ORDER BY created_at ASC`
	rows, err := q.DB().QueryContext(ctx, sel, ttl.String())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if err := EnsureJob(ctx, q, id, "system-stale"); err != nil {
			return err
		}
	}
	return rows.Err()
}

// BumpPriority increases a job's priority to promote it to the front.
func BumpPriority(ctx context.Context, q *db.Queries, subredditID int32, delta int) error {
	const stmt = `UPDATE crawl_jobs SET priority = priority + $2, updated_at=now() WHERE subreddit_id = $1`
	_, err := q.DB().ExecContext(ctx, stmt, subredditID, delta)
	return err
}

// ClaimNextJob selects the highest-priority queued job and marks it crawling atomically.
// Updated to respect visibility timeout.
func ClaimNextJob(ctx context.Context, q *db.Queries) (db.CrawlJob, error) {
	rawDB := q.DB()
	sqlDB, ok := rawDB.(*sql.DB)
	if !ok {
		return db.CrawlJob{}, fmt.Errorf("underlying DB does not support transactions")
	}
	tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return db.CrawlJob{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var j db.CrawlJob
	// Select job that is visible (respecting visibility timeout)
	sel := `SELECT id, subreddit_id, status, retries, last_attempt, duration_ms, enqueued_by, created_at, updated_at
            FROM crawl_jobs
            WHERE status='queued' AND (visible_at IS NULL OR visible_at <= now())
            ORDER BY priority DESC, created_at ASC
            FOR UPDATE SKIP LOCKED
            LIMIT 1`
	row := tx.QueryRowContext(ctx, sel)
	if err = row.Scan(&j.ID, &j.SubredditID, &j.Status, &j.Retries, &j.LastAttempt, &j.DurationMs, &j.EnqueuedBy, &j.CreatedAt, &j.UpdatedAt); err != nil {
		// Fallback if visibility_at column is missing (backward compatibility)
		if pqErr, ok := err.(*pq.Error); ok && string(pqErr.Code) == "42703" {
			sel = `SELECT id, subreddit_id, status, retries, last_attempt, duration_ms, enqueued_by, created_at, updated_at
                   FROM crawl_jobs
                   WHERE status='queued'
                   ORDER BY priority DESC, created_at ASC
                   FOR UPDATE SKIP LOCKED
                   LIMIT 1`
			row = tx.QueryRowContext(ctx, sel)
			if err = row.Scan(&j.ID, &j.SubredditID, &j.Status, &j.Retries, &j.LastAttempt, &j.DurationMs, &j.EnqueuedBy, &j.CreatedAt, &j.UpdatedAt); err != nil {
				if err == sql.ErrNoRows {
					_ = tx.Rollback()
					return db.CrawlJob{}, sql.ErrNoRows
				}
				_ = tx.Rollback()
				return db.CrawlJob{}, err
			}
		} else {
			if err == sql.ErrNoRows {
				_ = tx.Rollback()
				return db.CrawlJob{}, sql.ErrNoRows
			}
			_ = tx.Rollback()
			return db.CrawlJob{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE crawl_jobs SET status='crawling', last_attempt=now(), updated_at=now() WHERE id=$1`, j.ID); err != nil {
		_ = tx.Rollback()
		return db.CrawlJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return db.CrawlJob{}, err
	}
	j.Status = "crawling"
	return j, nil
}

// MarkJobFailedWithRetry marks a job as failed and schedules it for retry if under max retries
func MarkJobFailedWithRetry(ctx context.Context, q *db.Queries, jobID int32, retryCount int32) error {
	nextRetry := time.Now().Add(CalculateRetryDelay(retryCount))

	const stmt = `UPDATE crawl_jobs 
              SET status = 'failed',
                  retry_count = retry_count + 1,
                  next_retry_at = $2,
                  updated_at = now()
              WHERE id = $1`
	_, err := q.DB().ExecContext(ctx, stmt, jobID, nextRetry)
	return err
}

// RequeueRetryableJobs finds failed jobs ready to retry and requeues them
func RequeueRetryableJobs(ctx context.Context, q *db.Queries) error {
	const stmt = `UPDATE crawl_jobs
              SET status = 'queued',
                  visible_at = now(),
                  updated_at = now()
              WHERE status = 'failed' 
                AND next_retry_at IS NOT NULL 
                AND next_retry_at <= now()
                AND (retry_count < max_retries OR max_retries IS NULL)`
	_, err := q.DB().ExecContext(ctx, stmt)
	return err
}

// AgeStarvedJobs increases priority for jobs that have been waiting too long
func AgeStarvedJobs(ctx context.Context, q *db.Queries, minAge time.Duration, priorityBoost int32) error {
	const stmt = `UPDATE crawl_jobs
              SET priority = LEAST(priority + $2, 100),
                  updated_at = now()
              WHERE status = 'queued'
                AND created_at < now() - $1::interval
                AND priority < 100`
	_, err := q.DB().ExecContext(ctx, stmt, minAge.String(), priorityBoost)
	return err
}
