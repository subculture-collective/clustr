package crawler

// The lifecycle Module is intentionally independent from sqlc.  It is used by
// new workers and can coexist with crawl_jobs until every caller has migrated.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

const (
	AttemptLease   = 2 * time.Minute
	LeaseHeartbeat = 30 * time.Second
	MaxAttempts    = 3
	activationLock = int64(0x434c435241574c) // "CLCRAWL"
)

type AttemptOutcome string

const (
	OutcomeComplete         AttemptOutcome = "complete"
	OutcomePartial          AttemptOutcome = "partial"
	OutcomeRetryableFailure AttemptOutcome = "retryable_failure"
	OutcomePermanentFailure AttemptOutcome = "permanent_failure"
	OutcomeCancelled        AttemptOutcome = "cancelled"
)

type CrawlAttempt struct {
	ID          int64
	RequestID   int64
	SubredditID int32
	RetryNumber int
	WorkerID    string
}

type AttemptResult struct {
	Outcome      AttemptOutcome
	Posts        int
	Comments     int
	ItemFailures int
	ErrorClass   string
	ErrorMessage string
}

func crawlerWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "crawler"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

// EnsureCrawlRequest is idempotent and brings a request forward when an
// explicit/manual discovery asks for it. A successful request is therefore
// never a permanent queue tombstone.
func EnsureCrawlRequest(ctx context.Context, q *db.Queries, subredditID int32, provenance string, priority int) error {
	if provenance == "" {
		provenance = "manual"
	}
	const stmt = `INSERT INTO crawl_requests
  (subreddit_id, enabled, priority, next_due_at, discovery_provenance, activation_cohort)
  VALUES ($1, true, $2, now(), $3, $4)
  ON CONFLICT (subreddit_id) DO UPDATE SET
    enabled = true,
    priority = GREATEST(crawl_requests.priority, EXCLUDED.priority),
    next_due_at = LEAST(crawl_requests.next_due_at, now()),
    discovery_provenance = CASE WHEN crawl_requests.discovery_provenance = 'legacy' THEN EXCLUDED.discovery_provenance ELSE crawl_requests.discovery_provenance END,
	activation_cohort = CASE WHEN EXCLUDED.activation_cohort = 'active' THEN 'active' ELSE crawl_requests.activation_cohort END,
    updated_at = now()`
	_, err := q.DB().ExecContext(ctx, stmt, subredditID, priority, provenance, activationCohort(provenance))
	return err
}

// activationCohort keeps new explicit work responsive while imported, stale,
// and discovery-derived work remains subject to the safe backlog throttle.
func activationCohort(provenance string) string {
	p := strings.ToLower(strings.TrimSpace(provenance))
	if p == "api" || p == "manual" || strings.HasPrefix(p, "scheduler:") {
		return "active"
	}
	return "backlog"
}

// ClaimNextCrawlAttempt atomically reclaims expired leases and leases one due
// request.  It returns sql.ErrNoRows when there is no due work.
func ClaimNextCrawlAttempt(ctx context.Context, q *db.Queries, workerID string) (CrawlAttempt, error) {
	if workerID == "" {
		workerID = crawlerWorkerID()
	}
	sqlDB, ok := q.DB().(*sql.DB)
	if !ok {
		return CrawlAttempt{}, errors.New("crawl lifecycle requires *sql.DB transactions")
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return CrawlAttempt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// A transaction-scoped lock makes the count-based activation limit exact
	// across multiple crawler replicas.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, activationLock); err != nil {
		return CrawlAttempt{}, err
	}

	// An abandoned attempt is terminal; its request is immediately eligible for
	// a new lease (unless it exhausted the retry policy below).
	if _, err = tx.ExecContext(ctx, `UPDATE crawl_attempts
SET state = 'retryable_failure', finished_at = now(), error_class = COALESCE(error_class, 'lease_expired'), error_message = COALESCE(error_message, 'worker lease expired')
WHERE state = 'leased' AND lease_expires_at < now()`); err != nil {
		return CrawlAttempt{}, err
	}

	const claim = `WITH due AS (
  SELECT r.id, r.subreddit_id,
	         -- A complete or partial attempt starts a fresh retry generation.
	         -- Historical failures must not permanently exhaust future recurring
	         -- crawls for the same request.
	         COALESCE((SELECT MAX(a.retry_number) + 1 FROM crawl_attempts a
	                   WHERE a.request_id = r.id
	                     AND a.state = 'retryable_failure'
	                     AND a.started_at >= COALESCE(r.last_completed_at, '-infinity'::timestamptz)), 0) AS retry_number
  FROM crawl_requests r
  WHERE r.enabled AND r.next_due_at <= now()
    AND NOT EXISTS (SELECT 1 FROM crawl_attempts active WHERE active.request_id = r.id AND active.state = 'leased' AND active.lease_expires_at >= now())
	AND (r.activation_cohort = 'active' OR (
	  SELECT count(*) FROM crawl_attempts recent
	  JOIN crawl_requests recent_request ON recent_request.id = recent.request_id
	  WHERE recent_request.activation_cohort = 'backlog'
	    AND recent.started_at >= now() - $4::interval
	) < $5)
	AND COALESCE((SELECT MAX(a.retry_number) + 1 FROM crawl_attempts a
	              WHERE a.request_id = r.id
	                AND a.state = 'retryable_failure'
	                AND a.started_at >= COALESCE(r.last_completed_at, '-infinity'::timestamptz)), 0) <= $2
  ORDER BY r.priority DESC, r.next_due_at ASC, r.id ASC
  FOR UPDATE SKIP LOCKED LIMIT 1
), created AS (
  INSERT INTO crawl_attempts (request_id, state, worker_id, lease_expires_at, retry_number)
  SELECT id, 'leased', $1, now() + $3::interval, retry_number FROM due
  WHERE retry_number <= $2
  RETURNING id, request_id, retry_number
)
SELECT created.id, created.request_id, due.subreddit_id, created.retry_number
FROM created JOIN due ON due.id = created.request_id`
	var attempt CrawlAttempt
	maxRetryNumber := config.Load().CrawlMaxAttempts - 1
	cfg := config.Load()
	err = tx.QueryRowContext(ctx, claim, workerID, maxRetryNumber, cfg.CrawlLeaseDuration.String(), cfg.BacklogActivationWindow.String(), cfg.BacklogActivationLimit).Scan(&attempt.ID, &attempt.RequestID, &attempt.SubredditID, &attempt.RetryNumber)
	if err != nil {
		return CrawlAttempt{}, err
	}
	attempt.WorkerID = workerID
	if _, err = tx.ExecContext(ctx, `UPDATE crawl_requests SET updated_at = now() WHERE id = $1`, attempt.RequestID); err != nil {
		return CrawlAttempt{}, err
	}
	if err = tx.Commit(); err != nil {
		return CrawlAttempt{}, err
	}
	return attempt, nil
}

func RenewCrawlAttemptLease(ctx context.Context, q *db.Queries, attempt CrawlAttempt) error {
	result, err := q.DB().ExecContext(ctx, `UPDATE crawl_attempts SET lease_expires_at = now() + $3::interval
WHERE id = $1 AND state = 'leased' AND worker_id = $2`, attempt.ID, attempt.WorkerID, config.Load().CrawlLeaseDuration.String())
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("crawl attempt lease is no longer owned")
	}
	return nil
}

// HeartbeatLease renews a lease until stop is called. Losing ownership cancels
// the returned execution context so a stale worker cannot continue writing.
func HeartbeatLease(ctx context.Context, q *db.Queries, attempt CrawlAttempt) (executionCtx context.Context, stop func(), leaseErrors <-chan error) {
	executionCtx, cancel := context.WithCancel(ctx)
	errorsOut := make(chan error, 1)
	var stopOnce sync.Once
	go func() {
		ticker := time.NewTicker(config.Load().CrawlHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-executionCtx.Done():
				return
			case <-ticker.C:
				if err := RenewCrawlAttemptLease(executionCtx, q, attempt); err != nil {
					errorsOut <- err
					cancel()
					return
				}
			}
		}
	}()
	return executionCtx, func() { stopOnce.Do(cancel) }, errorsOut
}

func FinishCrawlAttempt(ctx context.Context, q *db.Queries, attempt CrawlAttempt, result AttemptResult, freshness time.Duration) error {
	if freshness <= 0 {
		freshness = config.Load().CrawlFreshness
	}
	if result.Outcome == "" {
		result.Outcome = OutcomePermanentFailure
	}
	nextDue := time.Now().Add(freshness)
	lastRetryNumber := config.Load().CrawlMaxAttempts - 1
	if result.Outcome == OutcomeRetryableFailure && attempt.RetryNumber < lastRetryNumber {
		nextDue = time.Now().Add(CalculateRetryDelay(int32(attempt.RetryNumber)))
	}
	if result.Outcome == OutcomeRetryableFailure && attempt.RetryNumber >= lastRetryNumber {
		result.Outcome = OutcomePermanentFailure
		result.ErrorClass = "retry_exhausted"
		nextDue = time.Now().Add(freshness)
	}
	if result.Outcome == OutcomePermanentFailure || result.Outcome == OutcomeCancelled {
		nextDue = time.Now().Add(freshness)
	}
	txDB, ok := q.DB().(*sql.DB)
	if !ok {
		return errors.New("crawl lifecycle requires *sql.DB transactions")
	}
	tx, err := txDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := tx.ExecContext(ctx, `UPDATE crawl_attempts SET state=$1, finished_at=now(), posts_count=$2, comments_count=$3, item_failures=$4, error_class=NULLIF($5,''), error_message=NULLIF($6,'')
WHERE id=$7 AND state='leased' AND worker_id=$8`, result.Outcome, result.Posts, result.Comments, result.ItemFailures, result.ErrorClass, result.ErrorMessage, attempt.ID, attempt.WorkerID)
	if err != nil {
		return err
	}
	if n, _ := updated.RowsAffected(); n != 1 {
		return errors.New("crawl attempt is not an active owned lease")
	}
	_, err = tx.ExecContext(ctx, `UPDATE crawl_requests SET next_due_at=$1, last_completed_at=CASE WHEN $2 IN ('complete','partial') THEN now() ELSE last_completed_at END, updated_at=now() WHERE id=$3`, nextDue, result.Outcome, attempt.RequestID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// CalculateRetryDelay is kept deterministic in its bounds for callers/tests;
// random jitter prevents synchronized retries in production.
func CalculateRetryDelay(retryCount int32) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	delay := time.Minute * time.Duration(1<<uint(retryCount))
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	withJitter := delay + time.Duration(rand.Float64()*0.2*float64(delay))
	if withJitter > 24*time.Hour {
		return 24 * time.Hour
	}
	return withJitter
}
