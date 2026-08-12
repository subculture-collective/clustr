SELECT
    'node' as data_type,
    id,
    name,
    CAST(val AS TEXT) as val,
    type,
    pos_x,
    pos_y,
    pos_z,
    NULL as source,
    NULL as target
FROM graph_nodes
UNION ALL
SELECT
    'link' as data_type,
    CAST(id AS TEXT),
    NULL as name,
    CAST(NULL AS TEXT) as val,
    NULL as type,
    NULL as pos_x,
    NULL as pos_y,
    NULL as pos_z,
    source,
    target
FROM graph_links
ORDER BY data_type, id;

-- name: GetPrecalculatedGraphDataCappedAll :many
-- Optimized query with improved link filtering
-- Uses EXISTS subqueries for better performance on large datasets
-- Note: statement_timeout is enforced at application level via context timeout
WITH sel_nodes AS (
    SELECT gn.id, gn.name, gn.val, gn.type, gn.pos_x, gn.pos_y, gn.pos_z
    FROM graph_nodes gn
    ORDER BY (
        CASE WHEN gn.val ~ '^[0-9]+$' THEN CAST(gn.val AS BIGINT) ELSE 0 END
    ) DESC NULLS LAST, gn.id
    LIMIT $1
), sel_node_ids AS MATERIALIZED (
    -- Explicitly materialize IDs for efficient hash lookups in EXISTS subqueries
    SELECT id FROM sel_nodes
), sel_links AS (
    SELECT gl.id, gl.source, gl.target
    FROM graph_links gl
    WHERE EXISTS (SELECT 1 FROM sel_node_ids WHERE id = gl.source)
      AND EXISTS (SELECT 1 FROM sel_node_ids WHERE id = gl.target)
    LIMIT $2
)
SELECT
        'node' AS data_type,
        n.id,
        COALESCE(n.name, '') AS name,
        COALESCE(CAST(n.val AS TEXT), '') AS val,
    n.type,
    n.pos_x,
    n.pos_y,
    n.pos_z,
        NULL AS source,
        NULL AS target
FROM sel_nodes n
UNION ALL
SELECT
        'link' AS data_type,
        CAST(l.id AS TEXT),
        '' AS name,
        '' AS val,
        NULL AS type,
    NULL as pos_x,
    NULL as pos_y,
    NULL as pos_z,
        l.source,
        l.target
FROM sel_links l
ORDER BY data_type, id;

-- name: GetPrecalculatedGraphDataCappedFiltered :many
-- Optimized query with improved link filtering
-- Uses EXISTS subqueries for better performance than IN subqueries
-- Note: statement_timeout is enforced at application level via context timeout
WITH sel_nodes AS (
    SELECT gn.id, gn.name, gn.val, gn.type, gn.pos_x, gn.pos_y, gn.pos_z
    FROM graph_nodes gn
    WHERE gn.type IS NOT NULL AND gn.type = ANY($1::text[])
    ORDER BY (
        CASE WHEN gn.val ~ '^[0-9]+$' THEN CAST(gn.val AS BIGINT) ELSE 0 END
    ) DESC NULLS LAST, gn.id
    LIMIT $2
), sel_node_ids AS MATERIALIZED (
    -- Explicitly materialize IDs for efficient hash lookups in EXISTS subqueries
    SELECT id FROM sel_nodes
), sel_links AS (
    SELECT gl.id, gl.source, gl.target
    FROM graph_links gl
    WHERE EXISTS (SELECT 1 FROM sel_node_ids WHERE id = gl.source)
      AND EXISTS (SELECT 1 FROM sel_node_ids WHERE id = gl.target)
    LIMIT $3
)
SELECT
        'node' AS data_type,
        n.id,
        n.name,
        CAST(n.val AS TEXT) AS val,
    n.type,
    n.pos_x,
    n.pos_y,
    n.pos_z,
        NULL AS source,
        NULL AS target
FROM sel_nodes n
UNION ALL
SELECT
        'link' AS data_type,
        CAST(l.id AS TEXT),
        NULL AS name,
        CAST(NULL AS TEXT) AS val,
        NULL AS type,
    NULL as pos_x,
    NULL as pos_y,
    NULL as pos_z,
        l.source,
        l.target
FROM sel_links l
ORDER BY data_type, id;

-- name: GetPrecalculatedGraphDataNoPos :many
SELECT
    'node' as data_type,
    id,
    name,
    CAST(val AS TEXT) as val,
    type,
    NULL as source,
    NULL as target
FROM graph_nodes
UNION ALL
SELECT
    'link' as data_type,
    CAST(id AS TEXT),
    NULL as name,
    CAST(NULL AS TEXT) as val,
    NULL as type,
    source,
    target
FROM graph_links
ORDER BY data_type, id;


-- name: GetAllPosts :many
SELECT id, title, score
FROM posts;

-- name: GetAllComments :many
SELECT id, body, score, post_id
FROM comments;

-- name: BulkInsertGraphNode :exec
INSERT INTO graph_nodes (
    id,
    name,
    val,
    type
) VALUES (
    $1, $2, $3, $4
)
ON CONFLICT (id) DO UPDATE
SET
    name = EXCLUDED.name,
    val = EXCLUDED.val,
    type = EXCLUDED.type,
    updated_at = now();

-- name: CreateGraphLink :one
INSERT INTO graph_links (
    source,
    target
) VALUES (
    $1, $2
) RETURNING *;

-- name: ClearGraphTables :exec
TRUNCATE TABLE graph_nodes, graph_links CASCADE;

INSERT INTO graph_nodes (id, name, val, type)
VALUES ($1, $2, $3, $4);

-- name: BulkInsertGraphLink :exec
INSERT INTO graph_links (source, target)
SELECT $1, $2
WHERE EXISTS (SELECT 1 FROM graph_nodes WHERE id = $1)
    AND EXISTS (SELECT 1 FROM graph_nodes WHERE id = $2)
ON CONFLICT (source, target) DO NOTHING;

-- name: GetAllSubreddits :many
SELECT id, name, subscribers
FROM subreddits;

-- name: GetAllUsers :many
SELECT id, username
FROM users;

-- name: GetSubredditOverlap :one
WITH user_activity AS (
    SELECT DISTINCT p.author_id
    FROM posts p
    WHERE p.subreddit_id = $1
    UNION
    SELECT DISTINCT c.author_id
    FROM comments c
    WHERE c.subreddit_id = $1
),
other_activity AS (
    SELECT DISTINCT p.author_id
    FROM posts p
    WHERE p.subreddit_id = $2
    UNION
    SELECT DISTINCT c.author_id
    FROM comments c
    WHERE c.subreddit_id = $2
)
SELECT COUNT(*)
FROM user_activity ua
JOIN other_activity oa ON ua.author_id = oa.author_id;

-- name: CreateSubredditRelationship :one
INSERT INTO subreddit_relationships (
    source_subreddit_id,
    target_subreddit_id,
    overlap_count
) VALUES (
    $1, $2, $3
) ON CONFLICT (source_subreddit_id, target_subreddit_id)
DO UPDATE SET
    overlap_count = EXCLUDED.overlap_count,
    updated_at = now()
RETURNING *;

-- name: ClearSubredditRelationships :exec
TRUNCATE TABLE subreddit_relationships;

-- name: GetUserSubreddits :many
SELECT DISTINCT s.id, s.name
FROM subreddits s
JOIN posts p ON p.subreddit_id = s.id
WHERE p.author_id = $1
UNION
SELECT DISTINCT s.id, s.name
FROM subreddits s
JOIN comments c ON c.subreddit_id = s.id
WHERE c.author_id = $1;

-- name: GetUserSubredditActivityCount :one
SELECT (
    (SELECT COUNT(*) FROM posts p WHERE p.author_id = $1 AND p.subreddit_id = $2) +
    (SELECT COUNT(*) FROM comments c WHERE c.author_id = $1 AND c.subreddit_id = $2)
) as activity_count;

-- name: CreateUserSubredditActivity :one
INSERT INTO user_subreddit_activity (
    user_id,
    subreddit_id,
    activity_count
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: ClearUserSubredditActivity :exec
TRUNCATE TABLE user_subreddit_activity;

-- name: GetAllSubredditRelationships :many
SELECT source_subreddit_id, target_subreddit_id, overlap_count
FROM subreddit_relationships;

-- name: GetAllUserSubredditActivity :many
SELECT user_id, subreddit_id, activity_count
FROM user_subreddit_activity;

-- name: GetUserTotalActivity :one
SELECT (
    (SELECT COUNT(*) FROM posts p WHERE p.author_id = $1) +
    (SELECT COUNT(*) FROM comments c WHERE c.author_id = $1)
) as total_activity;

-- name: ListUsersWithActivity :many
SELECT
    u.id,
    u.username,
    COALESCE(p.post_count, 0) + COALESCE(c.comment_count, 0) AS total_activity
FROM users u
LEFT JOIN (
    SELECT author_id, CAST(COUNT(*) AS BIGINT) AS post_count
    FROM posts
    GROUP BY author_id
) p ON p.author_id = u.id
LEFT JOIN (
    SELECT author_id, CAST(COUNT(*) AS BIGINT) AS comment_count
    FROM comments
    GROUP BY author_id
) c ON c.author_id = u.id
ORDER BY total_activity DESC, u.id;

-- name: ListGraphNodesByWeight :many
SELECT id, name, val, type, pos_x, pos_y, pos_z
FROM graph_nodes gn
ORDER BY (
    CASE WHEN gn.val ~ '^[0-9]+$' THEN CAST(gn.val AS BIGINT) ELSE 0 END
) DESC NULLS LAST, gn.id
LIMIT $1;

-- name: ListGraphLinksAmong :many
SELECT source, target
FROM graph_links
WHERE source = ANY($1::text[]) AND target = ANY($1::text[]);

-- name: UpdateGraphNodePositions :exec
UPDATE graph_nodes g
SET pos_x = u.x, pos_y = u.y, pos_z = u.z, updated_at = now()
FROM (
    SELECT unnest($1::text[]) AS id,
           unnest($2::double precision[]) AS x,
           unnest($3::double precision[]) AS y,
           unnest($4::double precision[]) AS z
) AS u
WHERE g.id = u.id;

-- ============================================================
-- Community Detection Queries
-- ============================================================

-- name: ClearCommunityTables :exec
TRUNCATE TABLE graph_communities CASCADE;

-- name: CreateCommunity :one
INSERT INTO graph_communities (
    label,
    size,
    modularity
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: CreateCommunityMember :exec
INSERT INTO graph_community_members (
    community_id,
    node_id
) VALUES (
    $1, $2
) ON CONFLICT (community_id, node_id) DO NOTHING;

-- name: CreateCommunityLink :exec
INSERT INTO graph_community_links (
    source_community_id,
    target_community_id,
    weight
) VALUES (
    $1, $2, $3
) ON CONFLICT (source_community_id, target_community_id)
DO UPDATE SET weight = EXCLUDED.weight, updated_at = now();

-- name: GetAllCommunities :many
SELECT * FROM graph_communities
ORDER BY size DESC;

-- name: GetCommunity :one
SELECT * FROM graph_communities
WHERE id = $1;

-- name: GetCommunityMembers :many
SELECT node_id FROM graph_community_members
WHERE community_id = $1;

-- name: GetCommunitySupernodesWithPositions :many
WITH community_stats AS (
    SELECT 
        gc.id,
        gc.label,
        gc.size,
        gc.modularity,
        COALESCE(AVG(gn.pos_x), 0) as avg_x,
        COALESCE(AVG(gn.pos_y), 0) as avg_y,
        COALESCE(AVG(gn.pos_z), 0) as avg_z
    FROM graph_communities gc
    LEFT JOIN graph_community_members gcm ON gc.id = gcm.community_id
    LEFT JOIN graph_nodes gn ON gcm.node_id = gn.id
    GROUP BY gc.id, gc.label, gc.size, gc.modularity
)
SELECT 
    'node' as data_type,
    CAST('community_' || id AS TEXT) as id,
    label as name,
    CAST(size AS TEXT) as val,
    'community' as type,
    avg_x as pos_x,
    avg_y as pos_y,
    avg_z as pos_z,
    NULL as source,
    NULL as target
FROM community_stats
ORDER BY size DESC;

-- name: GetCommunityLinks :many
SELECT
    'link' as data_type,
    CAST(gcl.source_community_id || '_' || gcl.target_community_id AS TEXT) as id,
    NULL as name,
    CAST(gcl.weight AS TEXT) as val,
    NULL as type,
    NULL as pos_x,
    NULL as pos_y,
    NULL as pos_z,
    CAST('community_' || gcl.source_community_id AS TEXT) as source,
    CAST('community_' || gcl.target_community_id AS TEXT) as target
FROM graph_community_links gcl
ORDER BY gcl.weight DESC
LIMIT $1;

-- name: GetCommunitySubgraph :many
WITH member_nodes AS (
    SELECT node_id FROM graph_community_members
    WHERE community_id = $1
)
SELECT
    'node' as data_type,
    gn.id,
    gn.name,
    CAST(gn.val AS TEXT) as val,
    gn.type,
    gn.pos_x,
    gn.pos_y,
    gn.pos_z,
    NULL as source,
    NULL as target
FROM graph_nodes gn
WHERE gn.id IN (SELECT node_id FROM member_nodes)
UNION ALL
SELECT
    'link' as data_type,
    CAST(gl.id AS TEXT),
    NULL as name,
    CAST(NULL AS TEXT) as val,
    NULL as type,
    NULL as pos_x,
    NULL as pos_y,
    NULL as pos_z,
    gl.source,
    gl.target
FROM graph_links gl
WHERE gl.source IN (SELECT node_id FROM member_nodes)
    AND gl.target IN (SELECT node_id FROM member_nodes)
ORDER BY data_type, id
LIMIT $2;

-- ============================================================
-- Edge Bundle Queries
-- ============================================================

-- name: ClearEdgeBundles :exec
TRUNCATE TABLE graph_bundles;

-- name: CreateEdgeBundle :exec
INSERT INTO graph_bundles (
    source_community_id,
    target_community_id,
    weight,
    avg_strength,
    control_x,
    control_y,
    control_z
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) ON CONFLICT (source_community_id, target_community_id)
DO UPDATE SET 
    weight = EXCLUDED.weight,
    avg_strength = EXCLUDED.avg_strength,
    control_x = EXCLUDED.control_x,
    control_y = EXCLUDED.control_y,
    control_z = EXCLUDED.control_z,
    updated_at = now();

-- name: GetEdgeBundles :many
SELECT
    source_community_id,
    target_community_id,
    weight,
    avg_strength,
    control_x,
    control_y,
    control_z
FROM graph_bundles
WHERE weight >= $1
ORDER BY weight DESC;

-- name: SearchGraphNodes :many
-- Fuzzy search for graph nodes by name or ID
-- Uses ILIKE for case-insensitive partial matching
-- Orders results by exact match first, then by relevance (val/weight)
-- Note: Leading wildcards prevent index usage and cause full table scans.
-- For large datasets, consider adding a GIN or GiST index with pg_trgm extension.
SELECT 
    id,
    name,
    CAST(val AS TEXT) as val,
    type,
    pos_x,
    pos_y,
    pos_z
FROM graph_nodes
WHERE 
    name ILIKE '%' || $1 || '%' 
    OR id ILIKE '%' || $1 || '%'
ORDER BY 
    CASE 
        WHEN LOWER(name) = LOWER($1) THEN 0
        WHEN LOWER(id) = LOWER($1) THEN 1
        ELSE 2
    END,
    CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END DESC
LIMIT $2;

-- ============================================================
-- Community Hierarchy Queries
-- ============================================================

-- name: ClearCommunityHierarchy :exec
TRUNCATE TABLE graph_community_hierarchy;

-- name: InsertCommunityHierarchy :exec
INSERT INTO graph_community_hierarchy (
    node_id,
    level,
    community_id,
    parent_community_id,
    centroid_x,
    centroid_y,
    centroid_z
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) ON CONFLICT (node_id, level) DO UPDATE SET
    community_id = EXCLUDED.community_id,
    parent_community_id = EXCLUDED.parent_community_id,
    centroid_x = EXCLUDED.centroid_x,
    centroid_y = EXCLUDED.centroid_y,
    centroid_z = EXCLUDED.centroid_z;

-- name: GetCommunityHierarchy :many
SELECT 
    node_id,
    level,
    community_id,
    parent_community_id,
    centroid_x,
    centroid_y,
    centroid_z
FROM graph_community_hierarchy
ORDER BY level, community_id, node_id;

-- name: GetNodesAtLevel :many
SELECT 
    node_id,
    community_id,
    parent_community_id,
    centroid_x,
    centroid_y,
    centroid_z
FROM graph_community_hierarchy
WHERE level = $1
ORDER BY community_id, node_id;

-- name: GetHierarchyLevels :many
SELECT DISTINCT level
FROM graph_community_hierarchy
ORDER BY level;

-- name: GetCommunitiesAtLevel :many
SELECT 
    community_id,
    COUNT(*) as member_count,
    AVG(centroid_x) as avg_x,
    AVG(centroid_y) as avg_y,
    AVG(centroid_z) as avg_z
FROM graph_community_hierarchy
WHERE level = $1
GROUP BY community_id
ORDER BY member_count DESC;

-- name: GetNodesInBoundingBox :many
-- Retrieves nodes within a 3D bounding box using the spatial index
-- Parameters: x_min, x_max, y_min, y_max, z_min, z_max, limit
-- The spatial index (idx_graph_nodes_spatial_nonnull) makes this query efficient
SELECT 
    id,
    name,
    val,
    type,
    pos_x,
    pos_y,
    pos_z
FROM graph_nodes
WHERE pos_x IS NOT NULL
  AND pos_y IS NOT NULL
  AND pos_z IS NOT NULL
  AND pos_x BETWEEN $1 AND $2
  AND pos_y BETWEEN $3 AND $4
  AND pos_z BETWEEN $5 AND $6
ORDER BY (
    CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END
) DESC NULLS LAST, id
LIMIT $7;

-- name: GetNodesInBoundingBox2D :many
-- Retrieves nodes within a 2D bounding box (ignoring z coordinate)
-- Parameters: x_min, x_max, y_min, y_max, limit
-- Useful for 2D viewport queries where z is not relevant
-- Note: Includes pos_z IS NOT NULL to match the partial GiST index predicate (which requires all position columns to be non-null)
SELECT 
    id,
    name,
    val,
    type,
    pos_x,
    pos_y,
    pos_z
FROM graph_nodes
WHERE pos_x IS NOT NULL
  AND pos_y IS NOT NULL
  AND pos_z IS NOT NULL
  AND pos_x BETWEEN $1 AND $2
  AND pos_y BETWEEN $3 AND $4
ORDER BY (
    CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END
) DESC NULLS LAST, id
LIMIT $5;

-- name: GetLinksForNodesInBoundingBox :many
-- Retrieves links where both source and target nodes are within the bounding box
-- Uses the same spatial filtering approach
-- Parameters: x_min, x_max, y_min, y_max, z_min, z_max, limit
WITH bbox_nodes AS (
    SELECT id
    FROM graph_nodes
    WHERE pos_x IS NOT NULL
      AND pos_y IS NOT NULL
      AND pos_z IS NOT NULL
      AND pos_x BETWEEN $1 AND $2
      AND pos_y BETWEEN $3 AND $4
      AND pos_z BETWEEN $5 AND $6
)
SELECT 
    gl.id,
    gl.source,
    gl.target
FROM graph_links gl
WHERE EXISTS (SELECT 1 FROM bbox_nodes WHERE id = gl.source)
  AND EXISTS (SELECT 1 FROM bbox_nodes WHERE id = gl.target)
LIMIT $7;

-- name: CountNodesInBoundingBox :one
-- Count nodes within a bounding box (useful for pagination)
-- Parameters: x_min, x_max, y_min, y_max, z_min, z_max
SELECT COUNT(*)
FROM graph_nodes
WHERE pos_x IS NOT NULL
  AND pos_y IS NOT NULL
  AND pos_z IS NOT NULL
  AND pos_x BETWEEN $1 AND $2
  AND pos_y BETWEEN $3 AND $4
  AND pos_z BETWEEN $5 AND $6;

-- ============================================================
-- Incremental Precalculation Queries
-- ============================================================

-- name: GetPrecalcState :one
-- Get the current precalculation state
SELECT * FROM precalc_state WHERE id = 1;

-- name: UpdatePrecalcState :exec
-- Update the precalculation state after a run
UPDATE precalc_state
SET 
    last_precalc_at = $1,
    last_full_precalc_at = COALESCE($2, last_full_precalc_at),
    total_nodes = $3,
    total_links = $4,
    precalc_duration_ms = $5,
    updated_at = now()
WHERE id = 1;

-- name: GetChangedSubredditsSince :many
-- Get subreddits that have been created or updated since the given timestamp
SELECT id, name, subscribers
FROM subreddits
WHERE updated_at > $1 OR created_at > $1
ORDER BY id;

-- name: GetChangedUsersSince :many
-- Get users that have been created or updated since the given timestamp
SELECT id, username
FROM users
WHERE updated_at > $1 OR created_at > $1
ORDER BY id;

-- name: GetChangedPostsSince :many
-- Get posts that have been created or updated since the given timestamp
SELECT id, subreddit_id, author_id
FROM posts
WHERE updated_at > $1
ORDER BY id;

-- name: GetChangedCommentsSince :many
-- Get comments that have been created or updated since the given timestamp
SELECT id, subreddit_id, author_id, post_id
FROM comments
WHERE updated_at > $1
ORDER BY id;

-- name: GetAffectedUserIDs :many
-- Get user IDs affected by changed posts/comments
WITH changed_authors AS (
    SELECT DISTINCT author_id FROM posts WHERE posts.updated_at > $1
    UNION
    SELECT DISTINCT author_id FROM comments WHERE comments.updated_at > $1
)
SELECT author_id FROM changed_authors
ORDER BY author_id;

-- name: GetAffectedSubredditIDs :many
-- Get subreddit IDs affected by changed posts/comments
WITH changed_subreddits AS (
    SELECT DISTINCT subreddit_id FROM posts WHERE posts.updated_at > $1
    UNION
    SELECT DISTINCT subreddit_id FROM comments WHERE comments.updated_at > $1
    UNION
    SELECT id as subreddit_id FROM subreddits WHERE subreddits.updated_at > $1 OR subreddits.created_at > $1
)
SELECT subreddit_id FROM changed_subreddits
ORDER BY subreddit_id;

-- name: CountChangedEntities :one
-- Count how many entities have changed since the given timestamp
SELECT 
    (SELECT COUNT(*) FROM subreddits s WHERE s.updated_at > $1 OR s.created_at > $1) as changed_subreddits,
    (SELECT COUNT(*) FROM users u WHERE u.updated_at > $1 OR u.created_at > $1) as changed_users,
    (SELECT COUNT(*) FROM posts p WHERE p.updated_at > $1) as changed_posts,
    (SELECT COUNT(*) FROM comments c WHERE c.updated_at > $1) as changed_comments;

-- name: GetUserActivitySince :many
-- Get user activity that has been updated since the given timestamp
-- Returns users who have posted or commented since the given time
SELECT DISTINCT
    u.id,
    u.username,
    COALESCE(p.post_count, 0) + COALESCE(c.comment_count, 0) AS total_activity
FROM users u
LEFT JOIN (
    SELECT author_id, CAST(COUNT(*) AS BIGINT) AS post_count
    FROM posts
    WHERE posts.updated_at > $1
    GROUP BY author_id
) p ON p.author_id = u.id
LEFT JOIN (
    SELECT author_id, CAST(COUNT(*) AS BIGINT) AS comment_count
    FROM comments
    WHERE comments.updated_at > $1
    GROUP BY author_id
) c ON c.author_id = u.id
WHERE (p.post_count IS NOT NULL AND p.post_count > 0)
   OR (c.comment_count IS NOT NULL AND c.comment_count > 0)
ORDER BY total_activity DESC, u.id;

-- name: DeleteOrphanedGraphNodes :exec
-- Delete graph nodes that no longer have corresponding source entities
-- This is used after incremental updates to clean up stale data
DELETE FROM graph_nodes
WHERE (type = 'user' AND id NOT IN (SELECT 'user_' || id::text FROM users))
   OR (type = 'subreddit' AND id NOT IN (SELECT 'subreddit_' || id::text FROM subreddits));

-- name: DeleteOrphanedGraphLinks :exec
-- Delete graph links where source or target nodes no longer exist
DELETE FROM graph_links
WHERE source NOT IN (SELECT id FROM graph_nodes)
   OR target NOT IN (SELECT id FROM graph_nodes);

-- ============================================================
-- Pagination Queries
-- ============================================================

-- name: GetPaginatedGraphNodes :many
-- Cursor-based pagination for graph nodes ordered by weight (val) descending
-- Cursor format: "weight:id" for tie-breaking
-- Parameters: $1=cursor_weight (BIGINT), $2=cursor_id (TEXT), $3=page_size (INT)
-- If cursor_weight is NULL, starts from beginning
SELECT 
    id,
    name,
    val,
    type,
    pos_x,
    pos_y,
    pos_z
FROM graph_nodes
WHERE 
    -- If no cursor, get from start; otherwise use cursor for pagination
    CASE 
        WHEN $1::BIGINT IS NULL THEN TRUE
        ELSE (
            -- Primary sort: weight descending
            (CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END) < $1::BIGINT
            OR (
                -- Tie-breaker: same weight, but ID is greater (for consistent ordering)
                (CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END) = $1::BIGINT
                AND id > $2::TEXT
            )
        )
    END
ORDER BY (
    CASE WHEN val ~ '^[0-9]+$' THEN CAST(val AS BIGINT) ELSE 0 END
) DESC NULLS LAST, id
LIMIT $3;

-- name: GetLinksForPaginatedNodes :many
-- Get links where both source and target are in the provided node ID list
-- Parameters: $1=node_ids (text array)
SELECT 
    id,
    source,
    target
FROM graph_links
WHERE source = ANY($1::text[]) 
  AND target = ANY($1::text[])
LIMIT $2;

-- ============================================================
-- Graph Versioning Queries
-- ============================================================

-- name: CreateGraphVersion :one
-- Create a new graph version record
INSERT INTO graph_versions (
    node_count,
    link_count,
    status,
    precalc_duration_ms,
    is_full_rebuild
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetGraphVersion :one
-- Get a specific graph version by ID
SELECT * FROM graph_versions WHERE id = $1;

-- name: GetCurrentGraphVersion :one
-- Get the most recent completed graph version
SELECT * FROM graph_versions 
WHERE status = 'completed'
ORDER BY id DESC 
LIMIT 1;

-- name: ListGraphVersions :many
-- List recent graph versions with pagination
SELECT * FROM graph_versions
ORDER BY id DESC
LIMIT $1 OFFSET $2;

-- name: UpdateGraphVersionStatus :exec
-- Update the status of a graph version
UPDATE graph_versions
SET status = $1, precalc_duration_ms = $2
WHERE id = $3;

-- name: DeleteOldGraphVersions :exec
-- Delete graph versions older than the retention count
-- Keeps the most recent N versions
WITH versions_to_keep AS (
    SELECT id FROM graph_versions
    ORDER BY id DESC
    LIMIT $1
)
DELETE FROM graph_versions
WHERE id NOT IN (SELECT id FROM versions_to_keep);

-- name: CountGraphVersions :one
-- Count total number of graph versions
SELECT COUNT(*) FROM graph_versions;

-- name: CreateGraphDiff :exec
-- Record a diff entry for a graph version
INSERT INTO graph_diffs (
    version_id,
    action,
    entity_type,
    entity_id,
    old_val,
    new_val,
    old_pos_x,
    old_pos_y,
    old_pos_z,
    new_pos_x,
    new_pos_y,
    new_pos_z
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
);

-- name: GetGraphDiffsSinceVersion :many
-- Get all diffs since a specific version (exclusive)
-- Returns changes from versions > $1 up to current
SELECT 
    gd.id,
    gd.version_id,
    gd.action,
    gd.entity_type,
    gd.entity_id,
    gd.old_val,
    gd.new_val,
    gd.old_pos_x,
    gd.old_pos_y,
    gd.old_pos_z,
    gd.new_pos_x,
    gd.new_pos_y,
    gd.new_pos_z,
    gd.created_at,
    gv.id as version_number
FROM graph_diffs gd
JOIN graph_versions gv ON gd.version_id = gv.id
WHERE gd.version_id > $1
  AND gv.status = 'completed'
ORDER BY gd.version_id, gd.id;

-- name: GetGraphDiffsForVersion :many
-- Get all diffs for a specific version
SELECT * FROM graph_diffs
WHERE version_id = $1
ORDER BY id;

-- name: CountGraphDiffsForVersion :one
-- Count diffs for a specific version
SELECT COUNT(*) FROM graph_diffs WHERE version_id = $1;

-- name: UpdatePrecalcStateVersion :exec
-- Update the current version ID in precalc_state
UPDATE precalc_state
SET current_version_id = $1
WHERE id = 1;

-- ============================================================
-- Node Inspector Queries
-- ============================================================

-- name: GetNodeDetails :one
-- Get detailed information about a specific node
SELECT 
    id,
    name,
    COALESCE(CAST(val AS TEXT), '') as val,
    type,
    pos_x,
    pos_y,
    pos_z
FROM graph_nodes
WHERE id = $1;

-- name: GetNodeNeighbors :many
-- Get neighbors of a node with connection information
-- Returns top N neighbors ordered by degree (treating all links as equal weight)
WITH node_links AS (
    SELECT gl.target as neighbor_id
    FROM graph_links gl
    WHERE gl.source = $1
    UNION ALL
    SELECT gl.source as neighbor_id
    FROM graph_links gl
    WHERE gl.target = $1
),
neighbor_degrees AS (
    SELECT 
        nl.neighbor_id,
        COUNT(*) as link_count
    FROM node_links nl
    GROUP BY nl.neighbor_id
)
SELECT 
    gn.id,
    gn.name,
    COALESCE(CAST(gn.val AS TEXT), '') as val,
    gn.type,
    nd.link_count::INTEGER as degree
FROM neighbor_degrees nd
JOIN graph_nodes gn ON gn.id = nd.neighbor_id
ORDER BY nd.link_count DESC, gn.id
LIMIT $2;
