CREATE TABLE subreddits (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    title TEXT,
    description TEXT,
    subscribers INT,
    created_at TIMESTAMPTZ DEFAULT now(),
    last_seen TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    last_seen TIMESTAMPTZ DEFAULT now(),
    first_seen TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE posts (
    id TEXT PRIMARY KEY,
    subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
    author_id INTEGER NOT NULL REFERENCES users(id),
    title TEXT,
    selftext TEXT,
    permalink TEXT,
    created_at TIMESTAMPTZ,
    score INT,
    flair TEXT,
    url TEXT,
    is_self BOOLEAN,
    last_seen TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_posts_subreddit_id ON posts(subreddit_id);
CREATE INDEX idx_posts_author_id ON posts(author_id);

CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_id INTEGER NOT NULL REFERENCES users(id),
    subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
    parent_id TEXT,
    body TEXT,
    created_at TIMESTAMPTZ,
    score INT,
    last_seen TIMESTAMPTZ DEFAULT now(),
    depth INT DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_comments_parent_id ON comments(parent_id);
CREATE INDEX idx_comments_post_id ON comments(post_id);
CREATE INDEX idx_comments_author_id ON comments(author_id);
CREATE INDEX idx_comments_subreddit_id ON comments(subreddit_id);

CREATE TABLE crawl_jobs (
  id SERIAL PRIMARY KEY,
  subreddit_id INTEGER NOT NULL REFERENCES subreddits(id) UNIQUE,
  status TEXT NOT NULL DEFAULT 'queued', -- queued, crawling, success, failed
    priority INT DEFAULT 0,
  retries INT DEFAULT 0,
  last_attempt TIMESTAMPTZ DEFAULT now(),
  duration_ms INT,
  enqueued_by TEXT DEFAULT 'system',
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now(),
  visible_at TIMESTAMPTZ DEFAULT now(),
  next_retry_at TIMESTAMPTZ
);

CREATE INDEX idx_crawl_jobs_status ON crawl_jobs(status);
CREATE INDEX idx_crawl_jobs_subreddit_id ON crawl_jobs(subreddit_id);
CREATE INDEX idx_crawl_jobs_priority ON crawl_jobs(priority);
CREATE INDEX idx_crawl_jobs_visible_at ON crawl_jobs(visible_at) WHERE status = 'queued';
CREATE INDEX idx_crawl_jobs_next_retry ON crawl_jobs(next_retry_at) WHERE status = 'failed' AND next_retry_at IS NOT NULL;

COMMENT ON COLUMN crawl_jobs.visible_at IS 'Timestamp when the job becomes visible/available for processing';
COMMENT ON COLUMN crawl_jobs.next_retry_at IS 'Timestamp for next retry attempt (if failed)';

CREATE TABLE graph_nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    val TEXT,
    type TEXT,
    -- Optional precomputed positions for layout
    pos_x DOUBLE PRECISION,
    pos_y DOUBLE PRECISION,
    pos_z DOUBLE PRECISION,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Avoid wide btree index on full name to prevent oversized index entries; keep a small prefix index instead
-- CREATE INDEX idx_graph_nodes_name ON graph_nodes(name);
CREATE INDEX idx_graph_nodes_name_hash ON graph_nodes (substring(name, 1, 10));
CREATE INDEX idx_graph_nodes_type ON graph_nodes(type) WHERE type IS NOT NULL;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE INDEX idx_graph_nodes_spatial_nonnull ON graph_nodes USING gist (pos_x, pos_y, pos_z)
WHERE pos_x IS NOT NULL AND pos_y IS NOT NULL AND pos_z IS NOT NULL;
-- Expression index matching the query pattern in GetPrecalculatedGraphDataCappedAll/Filtered
-- This mirrors the ORDER BY clause: ORDER BY (CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END) DESC
-- Performance note: regex check is necessary to avoid CAST errors on non-numeric strings
CREATE INDEX idx_graph_nodes_val_numeric ON graph_nodes(
    (CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END) DESC NULLS LAST, id
);
-- Partial index for common node types with value ordering
CREATE INDEX idx_graph_nodes_type_val ON graph_nodes(type, (
    CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END
) DESC NULLS LAST)
WHERE type IN ('subreddit', 'user', 'post', 'comment');

CREATE TABLE graph_links (
    id SERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    target TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source) REFERENCES graph_nodes(id),
    FOREIGN KEY (target) REFERENCES graph_nodes(id),
    UNIQUE(source, target)
);

CREATE INDEX idx_graph_links_source ON graph_links(source);
CREATE INDEX idx_graph_links_target ON graph_links(target);
CREATE INDEX idx_graph_links_target_source ON graph_links(target, source);
CREATE INDEX idx_graph_links_source_target ON graph_links(source, target);

CREATE TABLE graph_communities (
    id SERIAL PRIMARY KEY,
    label TEXT NOT NULL,
    size INTEGER NOT NULL DEFAULT 0,
    modularity DOUBLE PRECISION,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE graph_community_members (
    community_id INTEGER NOT NULL REFERENCES graph_communities(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    PRIMARY KEY (community_id, node_id)
);

CREATE TABLE graph_community_links (
    source_community_id INTEGER NOT NULL REFERENCES graph_communities(id) ON DELETE CASCADE,
    target_community_id INTEGER NOT NULL REFERENCES graph_communities(id) ON DELETE CASCADE,
    weight INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source_community_id, target_community_id)
);

CREATE INDEX idx_community_members_node ON graph_community_members(node_id);
CREATE INDEX idx_community_links_source ON graph_community_links(source_community_id);
CREATE INDEX idx_community_links_target ON graph_community_links(target_community_id);

CREATE TABLE graph_community_hierarchy (
    node_id TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    level INTEGER NOT NULL CHECK (level >= 0),
    community_id INTEGER NOT NULL,
    parent_community_id INTEGER,
    centroid_x DOUBLE PRECISION,
    centroid_y DOUBLE PRECISION,
    centroid_z DOUBLE PRECISION,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (node_id, level)
);
CREATE INDEX idx_hierarchy_level ON graph_community_hierarchy(level);
CREATE INDEX idx_hierarchy_community ON graph_community_hierarchy(level, community_id);
CREATE INDEX idx_hierarchy_parent ON graph_community_hierarchy(level, parent_community_id);

CREATE TABLE graph_bundles (
    source_community_id INTEGER NOT NULL REFERENCES graph_communities(id) ON DELETE CASCADE,
    target_community_id INTEGER NOT NULL REFERENCES graph_communities(id) ON DELETE CASCADE,
    weight INTEGER NOT NULL DEFAULT 0,
    avg_strength DOUBLE PRECISION,
    control_x DOUBLE PRECISION,
    control_y DOUBLE PRECISION,
    control_z DOUBLE PRECISION,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source_community_id, target_community_id)
);
CREATE INDEX idx_bundles_source ON graph_bundles(source_community_id);
CREATE INDEX idx_bundles_target ON graph_bundles(target_community_id);
CREATE INDEX idx_bundles_weight ON graph_bundles(weight DESC);

CREATE TABLE subreddit_relationships (
    id SERIAL PRIMARY KEY,
    source_subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
    target_subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
    overlap_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_subreddit_id, target_subreddit_id)
);

CREATE INDEX idx_subreddit_relationships_source ON subreddit_relationships(source_subreddit_id);
CREATE INDEX idx_subreddit_relationships_target ON subreddit_relationships(target_subreddit_id);
CREATE INDEX idx_subreddit_relationships_composite ON subreddit_relationships(source_subreddit_id, target_subreddit_id);

CREATE TABLE user_subreddit_activity (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
    activity_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, subreddit_id)
);

CREATE INDEX idx_user_subreddit_activity_user ON user_subreddit_activity(user_id);
CREATE INDEX idx_user_subreddit_activity_subreddit ON user_subreddit_activity(subreddit_id);
CREATE INDEX idx_user_subreddit_activity_composite ON user_subreddit_activity(user_id, subreddit_id);

CREATE TABLE graph_data (
    id SERIAL PRIMARY KEY,
    data_type TEXT NOT NULL,
    node_id TEXT,
    node_name TEXT,
    node_value TEXT,
    node_type TEXT,
    source TEXT,
    target TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_graph_data_data_type ON graph_data(data_type);
CREATE INDEX idx_graph_data_node_id ON graph_data(node_id);
CREATE INDEX idx_graph_data_source ON graph_data(source);
CREATE INDEX idx_graph_data_target ON graph_data(target);

CREATE TABLE scheduled_jobs (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    subreddit_id INTEGER REFERENCES subreddits(id),
    cron_expression TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ NOT NULL,
    priority INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    created_by TEXT,
    CONSTRAINT valid_cron_format CHECK (cron_expression ~ '^(@(annually|yearly|monthly|weekly|daily|hourly|reboot))|(@every (\d+(ns|us|ms|s|m|h))+)|((((\d+,)+\d+|(\d+(\/|-)\d+)|\d+|\*) ?){5,7})$')
);

CREATE INDEX idx_scheduled_jobs_next_run ON scheduled_jobs(next_run_at) WHERE enabled = true;
CREATE INDEX idx_scheduled_jobs_subreddit ON scheduled_jobs(subreddit_id) WHERE enabled = true;

COMMENT ON TABLE scheduled_jobs IS 'Scheduled recurring crawl jobs with cron-like scheduling';
COMMENT ON COLUMN scheduled_jobs.cron_expression IS 'Cron expression (standard 5-7 field format) or @every duration';
COMMENT ON COLUMN scheduled_jobs.next_run_at IS 'Next scheduled execution time';
COMMENT ON COLUMN scheduled_jobs.last_run_at IS 'Last execution time';

CREATE TABLE admin_audit_log (
    id SERIAL PRIMARY KEY,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    user_id TEXT NOT NULL,
    details JSONB,
    ip_address TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_admin_audit_log_created_at ON admin_audit_log(created_at DESC);
CREATE INDEX idx_admin_audit_log_user_id ON admin_audit_log(user_id);
CREATE INDEX idx_admin_audit_log_resource ON admin_audit_log(resource_type, resource_id);

-- Admin service settings key-value table
CREATE TABLE IF NOT EXISTS service_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Precalculation state tracking
-- Precalculation state tracking (including version history)
-- Graph versions track each precalculation run for incremental updates
CREATE TABLE IF NOT EXISTS graph_versions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    node_count INTEGER DEFAULT 0 NOT NULL,
    link_count INTEGER DEFAULT 0 NOT NULL,
    status VARCHAR(20) DEFAULT 'completed' NOT NULL,
    precalc_duration_ms INTEGER DEFAULT 0,
    is_full_rebuild BOOLEAN DEFAULT false NOT NULL,
    CONSTRAINT valid_status CHECK (status IN ('pending', 'completed', 'failed'))
);

-- Graph diffs track changes between versions for incremental client updates
CREATE TABLE IF NOT EXISTS graph_diffs (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES graph_versions(id) ON DELETE CASCADE,
    action VARCHAR(10) NOT NULL,
    entity_type VARCHAR(10) NOT NULL,
    entity_id TEXT NOT NULL,
    old_val TEXT,
    new_val TEXT,
    old_pos_x DOUBLE PRECISION,
    old_pos_y DOUBLE PRECISION,
    old_pos_z DOUBLE PRECISION,
    new_pos_x DOUBLE PRECISION,
    new_pos_y DOUBLE PRECISION,
    new_pos_z DOUBLE PRECISION,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    CONSTRAINT valid_action CHECK (action IN ('add', 'remove', 'update')),
    CONSTRAINT valid_entity_type CHECK (entity_type IN ('node', 'link'))
);

-- Indexes for efficient version and diff queries
CREATE INDEX IF NOT EXISTS idx_graph_versions_created_at ON graph_versions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_graph_versions_status ON graph_versions(status);
CREATE INDEX IF NOT EXISTS idx_graph_diffs_version_id ON graph_diffs(version_id);
CREATE INDEX IF NOT EXISTS idx_graph_diffs_entity ON graph_diffs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_graph_diffs_action ON graph_diffs(action);

CREATE TABLE IF NOT EXISTS precalc_state (
    id INTEGER PRIMARY KEY DEFAULT 1,
    last_precalc_at TIMESTAMPTZ,
    last_full_precalc_at TIMESTAMPTZ,
    total_nodes INTEGER DEFAULT 0,
    total_links INTEGER DEFAULT 0,
    precalc_duration_ms INTEGER DEFAULT 0,
    current_version_id BIGINT REFERENCES graph_versions(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT single_row_constraint CHECK (id = 1)
);

-- Ensure singleton precalc_state row exists (id must be 1)
INSERT INTO precalc_state (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- Indexes for change detection
CREATE INDEX IF NOT EXISTS idx_subreddits_updated_at ON subreddits(updated_at);
CREATE INDEX IF NOT EXISTS idx_users_updated_at ON users(updated_at);
CREATE INDEX IF NOT EXISTS idx_posts_updated_at ON posts(updated_at);
CREATE INDEX IF NOT EXISTS idx_comments_updated_at ON comments(updated_at);

-- Triggers to auto-update updated_at columns
CREATE OR REPLACE FUNCTION update_subreddits_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_subreddits_updated_at
    BEFORE UPDATE ON subreddits
    FOR EACH ROW
    EXECUTE FUNCTION update_subreddits_updated_at();

CREATE OR REPLACE FUNCTION update_users_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_users_updated_at();

CREATE OR REPLACE FUNCTION update_posts_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_posts_updated_at
    BEFORE UPDATE ON posts
    FOR EACH ROW
    EXECUTE FUNCTION update_posts_updated_at();

CREATE OR REPLACE FUNCTION update_comments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_comments_updated_at
    BEFORE UPDATE ON comments
    FOR EACH ROW
    EXECUTE FUNCTION update_comments_updated_at();
