-- Workspace link materialization keeps the 200,000 strongest overlap and
-- activity links. Without an index in that order PostgreSQL joined and sorted
-- every row of both tables on each hourly run (about 30 s on production).
-- These match the materialization's ORDER BY so it stops after the top rows.
CREATE INDEX IF NOT EXISTS user_subreddit_activity_weight_idx
    ON user_subreddit_activity(activity_count DESC, user_id, subreddit_id);
CREATE INDEX IF NOT EXISTS subreddit_relationships_weight_idx
    ON subreddit_relationships(overlap_count DESC, source_subreddit_id, target_subreddit_id)
    WHERE source_subreddit_id < target_subreddit_id;
