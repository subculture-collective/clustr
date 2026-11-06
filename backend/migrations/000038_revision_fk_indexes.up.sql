-- Index the second foreign key column of each revision artifact. Without
-- them, deleting a retained revision cascades from every graph_revision_nodes
-- row into graph_revision_links and graph_revision_community_members by
-- scanning all of that revision's entries, and community route publication
-- performs the same scan per link. Production retention took over 20 minutes.
SET LOCAL lock_timeout = '30s';

CREATE INDEX IF NOT EXISTS graph_revision_links_target_idx
    ON graph_revision_links(revision_id, target);
CREATE INDEX IF NOT EXISTS graph_revision_community_members_node_idx
    ON graph_revision_community_members(revision_id, node_id);
CREATE INDEX IF NOT EXISTS graph_revision_bundles_target_idx
    ON graph_revision_bundles(revision_id, target_community_id);
CREATE INDEX IF NOT EXISTS graph_revision_macro_group_links_target_idx
    ON graph_revision_macro_group_links(revision_id, target_macro_group_id);

-- Each of these duplicates a unique constraint or a leading prefix of one,
-- and is rewritten on every activity rebuild.
DROP INDEX IF EXISTS graph_revision_links_revision_idx;
DROP INDEX IF EXISTS idx_user_subreddit_activity_composite;
DROP INDEX IF EXISTS idx_user_subreddit_activity_user;
DROP INDEX IF EXISTS idx_subreddit_relationships_composite;
DROP INDEX IF EXISTS idx_subreddit_relationships_source;
