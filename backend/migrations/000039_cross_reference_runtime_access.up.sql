-- Migration 32 may be applied by a database migration owner that differs from
-- the crawler runtime role. Without these privileges every crawl that meets a
-- crosspost fails with "permission denied for table subreddit_cross_references".
-- The crawler inserts and updates cross references; the affinity builder reads them.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reddit_cluster_user') THEN
    GRANT SELECT,INSERT,UPDATE ON TABLE subreddit_cross_references TO reddit_cluster_user;
  END IF;
END
$$;
