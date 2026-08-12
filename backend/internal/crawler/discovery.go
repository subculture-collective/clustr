package crawler

import (
	"context"
	"database/sql"
	"log"
	"sort"
	"strings"
	"time"

	appconfig "github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

func FetchAndQueueUserSubreddits(ctx context.Context, q *db.Queries, username string, config FetchUserSubredditsConfig) {
	if !config.Enabled {
		return
	}

	subs, err := FetchRecentUserSubredditsContext(ctx, username, config.Limit)
	if err != nil {
		log.Printf("⚠️ Failed to fetch subs for u/%s: %v", username, err)
		return
	}

	// Stable ordering makes a discovery budget repeatable and auditable.
	sort.Strings(subs)
	count := 0
	total := len(subs)

	for _, sub := range subs {
		// Get or create subreddit
		subredditID, err := q.EnsureSubreddit(ctx, db.EnsureSubredditParams{
			Name:        sub,
			Title:       sql.NullString{String: sub, Valid: true},
			Description: sql.NullString{String: "", Valid: true},
			Subscribers: sql.NullInt32{Int32: 0, Valid: true},
		})
		if err != nil {
			log.Printf("⚠️ Failed to upsert subreddit r/%s: %v", sub, err)
			continue
		}

		if err := recordDiscoveryCandidate(ctx, q, subredditID, "author_history", username, 1); err != nil {
			log.Printf("⚠️ Failed to record candidate r/%s: %v", sub, err)
			continue
		}
		// Candidate recording is separate from promotion. This per-crawl budget
		// bounds discovery fan-out; the daily budget is enforced below.
		count++
		limit := appconfig.Load().DiscoveryCandidateBudget
		if config.MaxEnqueue > 0 && config.MaxEnqueue < limit {
			limit = config.MaxEnqueue
		}
		if count >= limit {
			break
		}
	}

	log.Printf("📬 Enqueued %d/%d new subs from u/%s", count, total, username)
}

func recordDiscoveryCandidate(ctx context.Context, q *db.Queries, subredditID int32, source, entity string, score float64) error {
	_, err := q.DB().ExecContext(ctx, `INSERT INTO discovery_candidates (subreddit_id, source, source_entity, score)
VALUES ($1, $2, $3, $4)
ON CONFLICT (subreddit_id, source, source_entity) DO UPDATE SET score=discovery_candidates.score + EXCLUDED.score, last_seen_at=now()`, subredditID, source, strings.ToLower(strings.TrimSpace(entity)), score)
	return err
}

func markDiscoveryPromoted(ctx context.Context, q *db.Queries, subredditID int32, source, entity string) error {
	_, err := q.DB().ExecContext(ctx, `UPDATE discovery_candidates SET disposition='promoted', last_seen_at=now()
WHERE subreddit_id=$1 AND source=$2 AND source_entity=$3`, subredditID, source, strings.ToLower(strings.TrimSpace(entity)))
	return err
}

// ReconsiderDiscoveryCandidates is an explicit, durable selection step.  It
// prioritizes explicit mentions, then score, and is intentionally bounded.
func ReconsiderDiscoveryCandidates(ctx context.Context, q *db.Queries, dailyBudget int) error {
	if dailyBudget <= 0 {
		return nil
	}
	rows, err := q.DB().QueryContext(ctx, `WITH remaining AS (
 SELECT GREATEST($1::integer-count(*),0)::integer budget FROM discovery_candidates
 WHERE disposition='promoted' AND last_seen_at >= date_trunc('day',now())
)
SELECT subreddit_id FROM discovery_candidates,remaining
WHERE disposition='pending' ORDER BY CASE source WHEN 'mention' THEN 0 ELSE 1 END, score DESC, subreddit_id ASC LIMIT (SELECT budget FROM remaining)`, dailyBudget)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if err := EnsureJob(ctx, q, id, "discovery-budget"); err != nil {
			return err
		}
		if _, err := q.DB().ExecContext(ctx, `UPDATE discovery_candidates SET disposition='promoted', last_seen_at=$2 WHERE subreddit_id=$1 AND disposition='pending'`, id, time.Now()); err != nil {
			return err
		}
	}
	return rows.Err()
}

// FetchAndQueueUserSubredditsForAuthors processes a list of authors and queues subs for each.
func FetchAndQueueUserSubredditsForAuthors(ctx context.Context, q *db.Queries, authors []string, config FetchUserSubredditsConfig) {
	sort.Strings(authors)
	if max := appconfig.Load().DiscoveryAuthorBudget; len(authors) > max {
		authors = authors[:max]
	}
	total := len(authors)
	processed := 0

	for _, author := range authors {
		FetchAndQueueUserSubreddits(ctx, q, author, config)
		processed++
		log.Printf("👥 Processed %d/%d users", processed, total)
	}
}
