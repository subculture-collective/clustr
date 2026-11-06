package graph

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Full rebuild statements. Both derived tables are rebuilt from source so a
// full run, or a run after a source TRUNCATE, starts from a clean baseline.
const (
	rebuildUserActivitySQL = `
INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count)
SELECT author_id,subreddit_id,count(*)::integer
FROM (
  SELECT author_id,subreddit_id FROM posts
  UNION ALL
  SELECT author_id,subreddit_id FROM comments
) activity
WHERE author_id IS NOT NULL AND subreddit_id IS NOT NULL
GROUP BY author_id,subreddit_id`

	rebuildSubredditRelationshipsSQL = `
WITH canonical AS (
  SELECT a.subreddit_id source_id,b.subreddit_id target_id,count(*)::integer overlap_count
  FROM user_subreddit_activity a
  JOIN user_subreddit_activity b
    ON b.user_id=a.user_id AND a.subreddit_id<b.subreddit_id
  GROUP BY a.subreddit_id,b.subreddit_id
)
INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count)
SELECT source_id,target_id,overlap_count FROM canonical
UNION ALL
SELECT target_id,source_id,overlap_count FROM canonical`
)

// Incremental refresh. overlap_count(a,b) is the number of users active in
// both subreddits, so only users whose (user, subreddit) set changed can move
// it. For each dirty user the old set comes from user_subreddit_activity and
// the new set from source; the pair counts differ by new pairs minus old pairs.
var incrementalActivitySteps = []struct{ name, sql string }{
	{"dirty_users", `CREATE TEMP TABLE activity_refresh_users ON COMMIT DROP AS
SELECT DISTINCT user_id FROM activity_refresh_log WHERE user_id IS NOT NULL`},
	{"dirty_users_index", `CREATE UNIQUE INDEX ON activity_refresh_users(user_id)`},
	{"old_activity", `CREATE TEMP TABLE activity_refresh_old ON COMMIT DROP AS
SELECT a.user_id,a.subreddit_id,a.activity_count
FROM user_subreddit_activity a JOIN activity_refresh_users d ON d.user_id=a.user_id`},
	{"new_activity", `CREATE TEMP TABLE activity_refresh_new ON COMMIT DROP AS
SELECT author_id user_id,subreddit_id,count(*)::integer activity_count
FROM (
  SELECT p.author_id,p.subreddit_id FROM posts p JOIN activity_refresh_users d ON d.user_id=p.author_id
  UNION ALL
  SELECT c.author_id,c.subreddit_id FROM comments c JOIN activity_refresh_users d ON d.user_id=c.author_id
) activity
WHERE subreddit_id IS NOT NULL
GROUP BY author_id,subreddit_id`},
	{"pair_delta", `CREATE TEMP TABLE activity_refresh_pairs ON COMMIT DROP AS
SELECT source_id,target_id,sum(delta)::integer delta FROM (
  SELECT a.subreddit_id source_id,b.subreddit_id target_id,-1 delta
  FROM activity_refresh_old a JOIN activity_refresh_old b ON b.user_id=a.user_id AND a.subreddit_id<b.subreddit_id
  UNION ALL
  SELECT a.subreddit_id,b.subreddit_id,1
  FROM activity_refresh_new a JOIN activity_refresh_new b ON b.user_id=a.user_id AND a.subreddit_id<b.subreddit_id
) pairs
GROUP BY source_id,target_id
HAVING sum(delta)<>0`},
	{"activity_delete", `DELETE FROM user_subreddit_activity a
USING activity_refresh_old o
WHERE a.user_id=o.user_id AND a.subreddit_id=o.subreddit_id
  AND NOT EXISTS (SELECT 1 FROM activity_refresh_new n WHERE n.user_id=o.user_id AND n.subreddit_id=o.subreddit_id)`},
	{"activity_upsert", `INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count)
SELECT user_id,subreddit_id,activity_count FROM activity_refresh_new
ON CONFLICT(user_id,subreddit_id) DO UPDATE
SET activity_count=EXCLUDED.activity_count,updated_at=now()
WHERE user_subreddit_activity.activity_count<>EXCLUDED.activity_count`},
	{"relationships_upsert", `INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count)
SELECT source_id,target_id,delta FROM activity_refresh_pairs
UNION ALL
SELECT target_id,source_id,delta FROM activity_refresh_pairs
ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE
SET overlap_count=subreddit_relationships.overlap_count+EXCLUDED.overlap_count,updated_at=now()`},
	{"relationships_prune", `DELETE FROM subreddit_relationships r
USING (
  SELECT source_id,target_id FROM activity_refresh_pairs
  UNION ALL
  SELECT target_id,source_id FROM activity_refresh_pairs
) p
WHERE r.source_subreddit_id=p.source_id AND r.target_subreddit_id=p.target_id AND r.overlap_count<=0`},
}

// refreshActivityAndRelationships keeps user_subreddit_activity and
// subreddit_relationships equal to a full rebuild from posts and comments.
// Incremental runs recompute only authors logged by the source triggers in
// activity_dirty_users; full runs, and any logged source TRUNCATE, rebuild
// both tables. The log is consumed in the same transaction that applies it,
// so a failed run leaves it in place for the next attempt.
func refreshActivityAndRelationships(ctx context.Context, database *sql.DB, full bool) error {
	start := time.Now()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin activity refresh: %w", err)
	}
	defer tx.Rollback()
	summary, err := refreshActivityInTx(ctx, tx, full)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit activity refresh: %w", err)
	}
	log.Printf("✅ %s in %s", summary, time.Since(start).Truncate(time.Millisecond))
	return nil
}

func refreshActivityInTx(ctx context.Context, tx *sql.Tx, full bool) (string, error) {
	defer func() {
		// Temporary tables are also dropped at commit; dropping them here lets
		// one transaction run more than one refresh.
		_, _ = tx.ExecContext(context.Background(), `DROP TABLE IF EXISTS activity_refresh_log,activity_refresh_users,activity_refresh_old,activity_refresh_new,activity_refresh_pairs`)
	}()
	// Rows logged after this DELETE stay for the next run. Recomputing a user
	// from source is idempotent, so a change seen by both runs is harmless.
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE activity_refresh_log ON COMMIT DROP AS
WITH consumed AS (DELETE FROM activity_dirty_users RETURNING user_id)
SELECT user_id FROM consumed`); err != nil {
		return "", fmt.Errorf("consume activity log: %w", err)
	}
	var logged, rebuildRequested int64
	if err := tx.QueryRowContext(ctx, `SELECT count(DISTINCT user_id),count(*) FILTER (WHERE user_id IS NULL) FROM activity_refresh_log`).Scan(&logged, &rebuildRequested); err != nil {
		return "", fmt.Errorf("inspect activity log: %w", err)
	}

	if full || rebuildRequested > 0 {
		for _, step := range []struct{ name, sql string }{
			{"truncate", `TRUNCATE TABLE user_subreddit_activity, subreddit_relationships RESTART IDENTITY`},
			{"user_activity", rebuildUserActivitySQL},
			{"subreddit_relationships", rebuildSubredditRelationshipsSQL},
		} {
			if _, err := tx.ExecContext(ctx, step.sql); err != nil {
				return "", fmt.Errorf("full activity rebuild %s: %w", step.name, err)
			}
		}
		return fmt.Sprintf("Rebuilt user activity and subreddit relationships (requested_by_log=%v)", rebuildRequested > 0), nil
	}
	if logged == 0 {
		return "User activity unchanged: no authors logged since the last refresh", nil
	}
	var pairChanges int64
	for _, step := range incrementalActivitySteps {
		result, err := tx.ExecContext(ctx, step.sql)
		if err != nil {
			return "", fmt.Errorf("incremental activity refresh %s: %w", step.name, err)
		}
		if step.name == "pair_delta" {
			pairChanges, _ = result.RowsAffected()
		}
	}
	return fmt.Sprintf("Refreshed user activity for %d authors (%d subreddit pair changes)", logged, pairChanges), nil
}
