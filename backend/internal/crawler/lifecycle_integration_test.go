package crawler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

func TestIntegration_CrawlRequestRecursAfterSuccess(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	name := fmt.Sprintf("lifecycle_%d", time.Now().UnixNano())
	var subredditID int32
	if err := conn.QueryRowContext(ctx, `INSERT INTO subreddits(name) VALUES($1) RETURNING id`, name).Scan(&subredditID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(`DELETE FROM subreddits WHERE id=$1`, subredditID) }()
	queries := db.New(conn)
	if err := EnsureCrawlRequest(ctx, queries, subredditID, "manual", 2_000_000_000); err != nil {
		t.Fatal(err)
	}
	first, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := FinishCrawlAttempt(ctx, queries, first, AttemptResult{Outcome: OutcomeComplete, Posts: 2}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE crawl_requests SET next_due_at=now()-interval '1 second' WHERE id=$1`, first.RequestID); err != nil {
		t.Fatal(err)
	}
	second, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker")
	if err != nil {
		t.Fatalf("second successful crawl was not claimable: %v", err)
	}
	if second.RequestID != first.RequestID || second.ID == first.ID {
		t.Fatalf("unexpected recurring attempt: first=%+v second=%+v", first, second)
	}
	if err := FinishCrawlAttempt(ctx, queries, second, AttemptResult{Outcome: OutcomePartial, ItemFailures: 1}, time.Hour); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_CrawlRetriesResetAfterCompletion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	name := fmt.Sprintf("lifecycle_retry_reset_%d", time.Now().UnixNano())
	var subredditID int32
	if err := conn.QueryRowContext(ctx, `INSERT INTO subreddits(name) VALUES($1) RETURNING id`, name).Scan(&subredditID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(`DELETE FROM subreddits WHERE id=$1`, subredditID) }()
	queries := db.New(conn)
	if err := EnsureCrawlRequest(ctx, queries, subredditID, "manual", 2_000_000_000); err != nil {
		t.Fatal(err)
	}

	first, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker")
	if err != nil {
		t.Fatal(err)
	}
	if first.RetryNumber != 0 {
		t.Fatalf("first retry number = %d, want 0", first.RetryNumber)
	}
	if err := FinishCrawlAttempt(ctx, queries, first, AttemptResult{Outcome: OutcomeRetryableFailure}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE crawl_requests SET next_due_at=now()-interval '1 second' WHERE id=$1`, first.RequestID); err != nil {
		t.Fatal(err)
	}
	second, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker")
	if err != nil {
		t.Fatal(err)
	}
	if second.RetryNumber != 1 {
		t.Fatalf("retry after failure = %d, want 1", second.RetryNumber)
	}
	if err := FinishCrawlAttempt(ctx, queries, second, AttemptResult{Outcome: OutcomeComplete}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE crawl_requests SET next_due_at=now()-interval '1 second' WHERE id=$1`, first.RequestID); err != nil {
		t.Fatal(err)
	}
	third, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker")
	if err != nil {
		t.Fatalf("next recurrence was not claimable after completion: %v", err)
	}
	if third.RetryNumber != 0 {
		t.Fatalf("retry generation did not reset: got %d, want 0", third.RetryNumber)
	}
	if err := FinishCrawlAttempt(ctx, queries, third, AttemptResult{Outcome: OutcomeComplete}, time.Hour); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_HeartbeatCancelsExecutionAfterLeaseLoss(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("CRAWL_HEARTBEAT_INTERVAL", "20ms")
	config.ResetForTest()
	defer config.ResetForTest()

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	name := fmt.Sprintf("lifecycle_lease_loss_%d", time.Now().UnixNano())
	var subredditID int32
	if err := conn.QueryRowContext(ctx, `INSERT INTO subreddits(name) VALUES($1) RETURNING id`, name).Scan(&subredditID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(`DELETE FROM subreddits WHERE id=$1`, subredditID) }()
	queries := db.New(conn)
	if err := EnsureCrawlRequest(ctx, queries, subredditID, "manual", 2_000_000_000); err != nil {
		t.Fatal(err)
	}
	attempt, err := ClaimNextCrawlAttempt(ctx, queries, "lease-owner")
	if err != nil {
		t.Fatal(err)
	}
	executionCtx, stop, leaseErrors := HeartbeatLease(ctx, queries, attempt)
	defer stop()
	if _, err := conn.ExecContext(ctx, `UPDATE crawl_attempts SET worker_id='replacement-owner' WHERE id=$1`, attempt.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-leaseErrors:
		if err == nil {
			t.Fatal("heartbeat reported a nil lease-loss error")
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not report lease ownership loss")
	}
	select {
	case <-executionCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("crawl execution context was not cancelled after lease loss")
	}
}

func TestIntegration_BacklogActivationLimitDefersNextBacklogClaim(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	var baseline int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM crawl_attempts a JOIN crawl_requests r ON r.id=a.request_id WHERE r.activation_cohort='backlog' AND a.started_at >= now()-interval '1 hour'`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRAWL_BACKLOG_ACTIVATION_LIMIT", strconv.Itoa(baseline+1))
	t.Setenv("CRAWL_BACKLOG_ACTIVATION_WINDOW", "1h")
	config.ResetForTest()
	t.Cleanup(config.ResetForTest)

	name := fmt.Sprintf("lifecycle_backlog_limit_%d", time.Now().UnixNano())
	var subredditID int32
	if err := conn.QueryRowContext(ctx, `INSERT INTO subreddits(name) VALUES($1) RETURNING id`, name).Scan(&subredditID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(`DELETE FROM subreddits WHERE id=$1`, subredditID) }()
	queries := db.New(conn)
	if err := EnsureCrawlRequest(ctx, queries, subredditID, "legacy", 2_000_000_000); err != nil {
		t.Fatal(err)
	}
	var cohort string
	if err := conn.QueryRowContext(ctx, `SELECT activation_cohort FROM crawl_requests WHERE subreddit_id=$1`, subredditID).Scan(&cohort); err != nil {
		t.Fatal(err)
	}
	if cohort != "backlog" {
		t.Fatalf("legacy request cohort=%q, want backlog", cohort)
	}

	first, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker")
	if err != nil {
		t.Fatal(err)
	}
	if first.SubredditID != subredditID {
		t.Fatalf("claimed subreddit=%d, want %d", first.SubredditID, subredditID)
	}
	if err := FinishCrawlAttempt(ctx, queries, first, AttemptResult{Outcome: OutcomeComplete}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE crawl_requests SET next_due_at=now()-interval '1 second' WHERE id=$1`, first.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := ClaimNextCrawlAttempt(ctx, queries, "integration-worker"); err != sql.ErrNoRows {
		t.Fatalf("backlog limit should defer the next claim, got %v", err)
	}
}
