CREATE INDEX IF NOT EXISTS idx_subreddit_relationships_source ON subreddit_relationships(source_subreddit_id);
CREATE INDEX IF NOT EXISTS idx_subreddit_relationships_composite ON subreddit_relationships(source_subreddit_id, target_subreddit_id);
CREATE INDEX IF NOT EXISTS idx_user_subreddit_activity_user ON user_subreddit_activity(user_id);
CREATE INDEX IF NOT EXISTS idx_user_subreddit_activity_composite ON user_subreddit_activity(user_id, subreddit_id);
CREATE INDEX IF NOT EXISTS graph_revision_links_revision_idx ON graph_revision_links(revision_id, source, target);
DROP INDEX IF EXISTS graph_revision_macro_group_links_target_idx;
DROP INDEX IF EXISTS graph_revision_bundles_target_idx;
DROP INDEX IF EXISTS graph_revision_community_members_node_idx;
DROP INDEX IF EXISTS graph_revision_links_target_idx;
