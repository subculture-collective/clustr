package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
)

// RevisionOptions bounds one immutable explorer artifact.  The caps are
// applied before publication and ordering is stable, so a CPU-only worker has
// a predictable memory/output envelope.
type RevisionOptions struct {
	NodeCap      int
	LinkCap      int
	Retention    int
	SubredditCap int
	UserCap      int
	PostCap      int
	CommentCap   int
	WorkMemMB    int
}

func DefaultRevisionOptions() RevisionOptions {
	return RevisionOptions{
		NodeCap:      positiveEnv("GRAPH_REVISION_NODE_CAP", 100000),
		LinkCap:      positiveEnv("GRAPH_REVISION_LINK_CAP", 200000),
		Retention:    positiveEnv("GRAPH_REVISION_RETENTION", 24),
		SubredditCap: nonNegativeEnv("GRAPH_REVISION_SUBREDDIT_CAP", 70000),
		UserCap:      nonNegativeEnv("GRAPH_REVISION_USER_CAP", 30000),
		PostCap:      nonNegativeEnv("GRAPH_REVISION_POST_CAP", 0),
		CommentCap:   nonNegativeEnv("GRAPH_REVISION_COMMENT_CAP", 0),
		WorkMemMB:    positiveEnv("GRAPH_PUBLICATION_WORK_MEM_MB", 256),
	}
}

func positiveEnv(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func nonNegativeEnv(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

// PublishRevision turns the legacy build workspace into an immutable spatial
// world. The staging revision is durable for diagnosis, while every artifact
// and the current pointer are committed atomically.
func PublishRevision(ctx context.Context, database *sql.DB) (int64, error) {
	return publishRevisionWithOptions(ctx, database, DefaultRevisionOptions(), nil)
}

func PublishRevisionWithOptions(ctx context.Context, database *sql.DB, options RevisionOptions) (int64, error) {
	return publishRevisionWithOptions(ctx, database, options, nil)
}

// PublishRevisionAtWatermark records the source cutoff captured before the
// calculation began. Source rows arriving during the build may be processed,
// but they remain newer than this watermark and are therefore reconsidered by
// the next incremental run instead of being skipped.
func PublishRevisionAtWatermark(ctx context.Context, database *sql.DB, sourceWatermark time.Time) (int64, error) {
	if sourceWatermark.IsZero() {
		return 0, fmt.Errorf("source watermark is required")
	}
	return publishRevisionWithOptions(ctx, database, DefaultRevisionOptions(), &sourceWatermark)
}

func publishRevisionWithOptions(ctx context.Context, database *sql.DB, options RevisionOptions, sourceWatermark *time.Time) (int64, error) {
	if options.NodeCap < 1 || options.LinkCap < 1 || options.Retention < 1 ||
		options.SubredditCap < 0 || options.UserCap < 0 || options.PostCap < 0 || options.CommentCap < 0 {
		return 0, fmt.Errorf("revision caps and retention must be positive")
	}
	configJSON, _ := json.Marshal(map[string]any{
		"projection": "typed-weighted-v1",
		"hierarchy":  "stable-overlap-v1",
		"layout":     "stable-community-3d-v1",
		"caps": map[string]int{
			"nodes": options.NodeCap, "links": options.LinkCap,
			"subreddits": options.SubredditCap, "users": options.UserCap,
			"posts": options.PostCap, "comments": options.CommentCap,
		},
	})
	var id int64
	err := database.QueryRowContext(ctx, `INSERT INTO graph_revisions(
status,source_watermark,algorithm_version,config,layout_algorithm,layout_seed,layout_dimensions)
SELECT 'staging',COALESCE($2::timestamptz,GREATEST(
  COALESCE((SELECT max(updated_at) FROM subreddits),to_timestamp(0)),
  COALESCE((SELECT max(updated_at) FROM users),to_timestamp(0)),
  COALESCE((SELECT max(updated_at) FROM posts),to_timestamp(0)),
  COALESCE((SELECT max(updated_at) FROM comments),to_timestamp(0))
)),'projection-v1',$1::jsonb,'stable-community-3d-v1',0,3 RETURNING id`, string(configJSON), sourceWatermark).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create staging revision: %w", err)
	}
	fail := func(stage string, cause error) (int64, error) {
		_, _ = database.ExecContext(context.Background(), `UPDATE graph_revisions SET status='failed',failure_reason=$2,validation_result=jsonb_build_object('stage',$3,'valid',false) WHERE id=$1`, id, cause.Error(), stage)
		return 0, fmt.Errorf("publish revision %d at %s: %w", id, stage, cause)
	}

	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fail("begin", err)
	}
	defer tx.Rollback()
	failTx := func(stage string, cause error) (int64, error) {
		// Revision artifact inserts hold a key-share lock on the parent revision.
		// Release the transaction before marking the durable staging row failed,
		// otherwise the diagnostic update can block behind our own transaction.
		_ = tx.Rollback()
		return fail(stage, cause)
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(48487755497810)`); err != nil {
		return failTx("publication_lock", err)
	}
	workMemMB := options.WorkMemMB
	if workMemMB < 1 {
		workMemMB = 256
	}
	if workMemMB > 2048 {
		workMemMB = 2048
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('work_mem',$1,true)`, fmt.Sprintf("%dMB", workMemMB)); err != nil {
		return failTx("publication_work_mem", err)
	}
	if _, err = tx.ExecContext(ctx, `SET LOCAL enable_nestloop = off`); err != nil {
		return failTx("publication_join_plan", err)
	}

	// Match both sides' best weighted membership overlap. Mutual-best matching
	// prevents one old landmark from being assigned to both sides of a split.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE revision_community_map ON COMMIT DROP AS
WITH previous AS (SELECT revision_id FROM graph_revision_current WHERE singleton),
overlap AS (
 SELECT m.community_id AS legacy_id,pm.community_id AS stable_id,count(*) AS shared
 FROM graph_community_members m
 JOIN previous p ON true
 JOIN graph_revision_community_members pm ON pm.revision_id=p.revision_id AND pm.node_id=m.node_id
 GROUP BY m.community_id,pm.community_id
), ranked AS (
 SELECT *,row_number() OVER(PARTITION BY legacy_id ORDER BY shared DESC,stable_id) rn_new,
          row_number() OVER(PARTITION BY stable_id ORDER BY shared DESC,legacy_id) rn_old
 FROM overlap
), mutual AS (SELECT legacy_id,stable_id FROM ranked WHERE rn_new=1 AND rn_old=1),
anchors AS (SELECT community_id,min(node_id) anchor FROM graph_community_members GROUP BY community_id)
SELECT c.id legacy_id,COALESCE(mutual.stable_id,'c:new:'||substr(md5(COALESCE(anchors.anchor,c.id::text)),1,16)) stable_id,
       old.x old_x,old.y old_y,old.z old_z
FROM graph_communities c LEFT JOIN mutual ON mutual.legacy_id=c.id
LEFT JOIN anchors ON anchors.community_id=c.id
LEFT JOIN previous p ON true
LEFT JOIN graph_revision_communities old ON old.revision_id=p.revision_id AND old.community_id=mutual.stable_id`)
	if err != nil {
		return failTx("community_matching", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_communities(revision_id,community_id,parent_id,level,label,size,x,y,z)
SELECT $1,m.stable_id,NULL,0,c.label,c.size,
 COALESCE(m.old_x,((hashtext(m.stable_id)::bigint % 20000)-10000)/10.0),
 COALESCE(m.old_y,((hashtext(m.stable_id||':y')::bigint % 20000)-10000)/10.0),
 COALESCE(m.old_z,((hashtext(m.stable_id||':z')::bigint % 20000)-10000)/10.0)
FROM graph_communities c JOIN revision_community_map m ON m.legacy_id=c.id`, id)
	if err != nil {
		return failTx("community_landmarks", err)
	}

	// Each typed branch is a bounded, indexable top-N lookup.  Do not use a
	// window over every workspace node here: a detailed crawl can contain
	// millions of comments that are explicitly excluded from this revision.
	_, err = tx.ExecContext(ctx, `WITH ranked_nodes AS (
 SELECT * FROM (
   (SELECT n.* FROM graph_nodes n WHERE n.type='subreddit'
    ORDER BY CASE WHEN n.val ~ '^[0-9]+$' THEN n.val::bigint ELSE 0 END DESC,n.id LIMIT $3)
   UNION ALL
   (SELECT n.* FROM graph_nodes n WHERE n.type='user'
    ORDER BY CASE WHEN n.val ~ '^[0-9]+$' THEN n.val::bigint ELSE 0 END DESC,n.id LIMIT $4)
   UNION ALL
   (SELECT n.* FROM graph_nodes n WHERE n.type='post'
    ORDER BY CASE WHEN n.val ~ '^[0-9]+$' THEN n.val::bigint ELSE 0 END DESC,n.id LIMIT $5)
   UNION ALL
   (SELECT n.* FROM graph_nodes n WHERE n.type='comment'
    ORDER BY CASE WHEN n.val ~ '^[0-9]+$' THEN n.val::bigint ELSE 0 END DESC,n.id LIMIT $6)
 ) typed
 WHERE ($3::bigint+$4::bigint+$5::bigint+$6::bigint)>0
 UNION ALL
 SELECT fallback.* FROM (
   SELECT n.* FROM graph_nodes n
   ORDER BY CASE WHEN n.val ~ '^[0-9]+$' THEN n.val::bigint ELSE 0 END DESC,n.id
   LIMIT $2
 ) fallback
 WHERE ($3::bigint+$4::bigint+$5::bigint+$6::bigint)=0
), capped_nodes AS (
 SELECT * FROM ranked_nodes
 ORDER BY CASE type WHEN 'subreddit' THEN 0 WHEN 'user' THEN 1 WHEN 'post' THEN 2 WHEN 'comment' THEN 3 ELSE 4 END,
          CASE WHEN val ~ '^[0-9]+$' THEN val::bigint ELSE 0 END DESC,id
 LIMIT $2
)
INSERT INTO graph_revision_nodes(revision_id,id,name,value,type,x,y,z,position_provenance)
SELECT $1,n.id,n.name,CASE WHEN n.val ~ '^[0-9]+$' THEN n.val::bigint ELSE 0 END,COALESCE(n.type,'unknown'),
 COALESCE(old.x,landmark.x + ((hashtext(n.id)::bigint % 2000)-1000)/100.0,n.pos_x,((hashtext(n.id)::bigint % 20000)-10000)/10.0),
 COALESCE(old.y,landmark.y + ((hashtext(n.id||':y')::bigint % 2000)-1000)/100.0,n.pos_y,((hashtext(n.id||':y')::bigint % 20000)-10000)/10.0),
 COALESCE(old.z,landmark.z + ((hashtext(n.id||':z')::bigint % 2000)-1000)/100.0,
          CASE WHEN abs(COALESCE(n.pos_z,0)) > 0.000001 THEN n.pos_z END,
          ((hashtext(n.id||':z')::bigint % 20000)-10000)/10.0),
 CASE WHEN old.id IS NOT NULL THEN 'warm-start' WHEN landmark.community_id IS NOT NULL THEN 'community-seeded' ELSE 'deterministic-global' END
FROM capped_nodes n
LEFT JOIN graph_revision_current current ON current.singleton
LEFT JOIN graph_revision_nodes old ON old.revision_id=current.revision_id AND old.id=n.id
LEFT JOIN LATERAL (SELECT map.stable_id FROM graph_community_members gm JOIN revision_community_map map ON map.legacy_id=gm.community_id WHERE gm.node_id=n.id ORDER BY map.stable_id LIMIT 1) member ON true
LEFT JOIN graph_revision_communities landmark ON landmark.revision_id=$1 AND landmark.community_id=member.stable_id`,
		id, options.NodeCap, options.SubredditCap, options.UserCap, options.PostCap, options.CommentCap)
	if err != nil {
		return failTx("spatial_layout", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_community_members(revision_id,community_id,node_id)
SELECT $1,map.stable_id,m.node_id FROM graph_community_members m
JOIN revision_community_map map ON map.legacy_id=m.community_id
JOIN graph_revision_nodes n ON n.revision_id=$1 AND n.id=m.node_id`, id)
	if err != nil {
		return failTx("community_members", err)
	}
	// A landmark without a resident member cannot be approached or inspected in
	// this revision, so it must not appear as an empty overview destination.
	_, err = tx.ExecContext(ctx, `DELETE FROM graph_revision_communities c
WHERE c.revision_id=$1 AND NOT EXISTS (
  SELECT 1 FROM graph_revision_community_members m
  WHERE m.revision_id=c.revision_id AND m.community_id=c.community_id
)`, id)
	if err != nil {
		return failTx("empty_communities", err)
	}

	// Give PostgreSQL real cardinality statistics for the resident entity set.
	// Parsing string graph IDs inside an inline CTE led the production-sized
	// projection to choose a multi-minute random-I/O join plan.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE revision_selected_entities ON COMMIT DROP AS
SELECT type AS entity_type,
       CASE type
         WHEN 'user' THEN substring(id FROM 6)::integer
         WHEN 'subreddit' THEN substring(id FROM 11)::integer
       END AS entity_id
FROM graph_revision_nodes
WHERE revision_id=$1
  AND ((type='user' AND id ~ '^user_[0-9]+$')
    OR (type='subreddit' AND id ~ '^subreddit_[0-9]+$'))`, id)
	if err != nil {
		return failTx("selected_entities", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX revision_selected_entities_lookup
ON revision_selected_entities(entity_type,entity_id)`); err != nil {
		return failTx("selected_entities_index", err)
	}
	if _, err = tx.ExecContext(ctx, `ANALYZE revision_selected_entities`); err != nil {
		return failTx("selected_entities_analyze", err)
	}

	// Source projection is set-based. Repeated facts increase weight; undirected
	// overlap is canonicalized once instead of materializing reverse duplicates.
	// Publication consumes the already-calculated workspace tables rather than
	// rejoining every post/comment on each immutable snapshot.
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_links(revision_id,source,target,relation,directed,weight)
WITH activity AS (
	SELECT 'user_'||a.user_id source,'subreddit_'||a.subreddit_id target,
	       a.activity_count::bigint weight
	FROM user_subreddit_activity a
	JOIN revision_selected_entities u ON u.entity_type='user' AND u.entity_id=a.user_id
	JOIN revision_selected_entities s ON s.entity_type='subreddit' AND s.entity_id=a.subreddit_id
	ORDER BY a.activity_count DESC,a.user_id,a.subreddit_id
	LIMIT $2
), overlap AS (
	SELECT 'subreddit_'||LEAST(r.source_subreddit_id,r.target_subreddit_id) source,
	       'subreddit_'||GREATEST(r.source_subreddit_id,r.target_subreddit_id) target,
	       GREATEST(r.overlap_count,COALESCE(reciprocal.overlap_count,r.overlap_count))::bigint weight
	FROM subreddit_relationships r
	LEFT JOIN subreddit_relationships reciprocal
	  ON reciprocal.source_subreddit_id=r.target_subreddit_id
	 AND reciprocal.target_subreddit_id=r.source_subreddit_id
	JOIN revision_selected_entities s ON s.entity_type='subreddit' AND s.entity_id=r.source_subreddit_id
	JOIN revision_selected_entities t ON t.entity_type='subreddit' AND t.entity_id=r.target_subreddit_id
	WHERE r.source_subreddit_id<>r.target_subreddit_id
	  AND (r.source_subreddit_id<r.target_subreddit_id OR reciprocal.id IS NULL)
	ORDER BY GREATEST(r.overlap_count,COALESCE(reciprocal.overlap_count,r.overlap_count)) DESC,
	         LEAST(r.source_subreddit_id,r.target_subreddit_id),
	         GREATEST(r.source_subreddit_id,r.target_subreddit_id)
	LIMIT $2
), projected AS (
	SELECT source,target,'user_activity' relation,true directed,weight FROM activity
	UNION ALL SELECT source,target,'subreddit_overlap',false,weight FROM overlap
	UNION ALL SELECT 'user_'||author_id,'post_'||id,'authorship',true,1 FROM posts WHERE $3::bigint>0
	UNION ALL SELECT 'user_'||author_id,'comment_'||id,'authorship',true,1 FROM comments WHERE $4::bigint>0
	UNION ALL SELECT 'comment_'||regexp_replace(parent_id,'^t1_',''),'comment_'||id,'reply',true,1 FROM comments WHERE $4::bigint>0 AND parent_id LIKE 't1_%'
	UNION ALL
	SELECT l.source,l.target,s.type||'_to_'||t.type,true,count(*)::bigint
	FROM graph_links l JOIN graph_revision_nodes s ON s.revision_id=$1 AND s.id=l.source
	JOIN graph_revision_nodes t ON t.revision_id=$1 AND t.id=l.target
	WHERE ($3::bigint+$4::bigint)>0
	  AND NOT (s.type='subreddit' AND t.type='subreddit')
	  AND NOT (s.type='user' AND t.type IN ('subreddit','post','comment'))
	  AND NOT (s.type='comment' AND t.type='comment')
	GROUP BY l.source,l.target,s.type,t.type
), valid AS (
	SELECT p.* FROM projected p JOIN graph_revision_nodes s ON s.revision_id=$1 AND s.id=p.source
	JOIN graph_revision_nodes t ON t.revision_id=$1 AND t.id=p.target WHERE p.source<>p.target
)
SELECT $1,source,target,relation,directed,weight FROM valid
ORDER BY weight DESC,source,target,relation,directed
LIMIT $2`, id, options.LinkCap, options.PostCap, options.CommentCap)
	if err != nil {
		return failTx("typed_projection", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_community_links(revision_id,source_community_id,target_community_id,weight)
SELECT $1,LEAST(sm.community_id,tm.community_id),GREATEST(sm.community_id,tm.community_id),sum(l.weight)
FROM graph_revision_links l
JOIN LATERAL (SELECT community_id FROM graph_revision_community_members WHERE revision_id=$1 AND node_id=l.source ORDER BY community_id LIMIT 1) sm ON true
JOIN LATERAL (SELECT community_id FROM graph_revision_community_members WHERE revision_id=$1 AND node_id=l.target ORDER BY community_id LIMIT 1) tm ON true
WHERE l.revision_id=$1 AND sm.community_id<>tm.community_id GROUP BY 2,3`, id)
	if err != nil {
		return failTx("community_routes", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_bundles(revision_id,source_community_id,target_community_id,weight,control_x,control_y,control_z)
SELECT $1,l.source_community_id,l.target_community_id,l.weight,(s.x+t.x)/2,(s.y+t.y)/2,(s.z+t.z)/2
FROM graph_revision_community_links l JOIN graph_revision_communities s ON s.revision_id=$1 AND s.community_id=l.source_community_id
JOIN graph_revision_communities t ON t.revision_id=$1 AND t.community_id=l.target_community_id WHERE l.revision_id=$1`, id)
	if err != nil {
		return failTx("bundles", err)
	}

	var nodes, links, communities int64
	var minX, maxX, minY, maxY, minZ, maxZ float64
	err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(min(x),0),COALESCE(max(x),0),COALESCE(min(y),0),COALESCE(max(y),0),COALESCE(min(z),0),COALESCE(max(z),0) FROM graph_revision_nodes WHERE revision_id=$1`, id).Scan(&nodes, &minX, &maxX, &minY, &maxY, &minZ, &maxZ)
	if err == nil {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM graph_revision_links WHERE revision_id=$1`, id).Scan(&links)
	}
	if err == nil {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM graph_revision_communities WHERE revision_id=$1`, id).Scan(&communities)
	}
	if err != nil {
		return failTx("counts", err)
	}
	if nodes == 0 || links == 0 || communities == 0 || minX == maxX || minY == maxY || minZ == maxZ || minX < -1e12 || maxX > 1e12 || minY < -1e12 || maxY > 1e12 || minZ < -1e12 || maxZ > 1e12 {
		return failTx("validation", fmt.Errorf("invalid or empty spatial world: nodes=%d links=%d communities=%d bounds=[%g,%g,%g,%g,%g,%g]", nodes, links, communities, minX, maxX, minY, maxY, minZ, maxZ))
	}

	validation := `{"valid":true,"finite_coordinates":true,"noncollapsed_bounds":true,"no_orphan_links":true,"landmark_continuity":"warm-started"}`
	_, err = tx.ExecContext(ctx, `UPDATE graph_revisions SET status='published',published_at=$2,node_count=$3,link_count=$4,community_count=$5,min_x=$6,max_x=$7,min_y=$8,max_y=$9,min_z=$10,max_z=$11,validation_result=$12::jsonb WHERE id=$1`, id, time.Now().UTC(), nodes, links, communities, minX, maxX, minY, maxY, minZ, maxZ, validation)
	if err != nil {
		return failTx("finalize", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_current(singleton,revision_id) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET revision_id=EXCLUDED.revision_id`, id)
	if err != nil {
		return failTx("promote", err)
	}
	if err = tx.Commit(); err != nil {
		return failTx("commit", err)
	}
	if err := RetainPublishedRevisions(ctx, database, options.Retention); err != nil {
		// Publication already committed and is valid. Retention is intentionally
		// best-effort so a cleanup failure cannot take the current world offline.
		logger.WarnContext(ctx, "Published graph revision but retention cleanup failed", "revision_id", id, "error", err)
	}
	return id, nil
}

// RetainPublishedRevisions removes only older published artifacts.  The current
// pointer is explicitly excluded even if its published_at is anomalous.
func RetainPublishedRevisions(ctx context.Context, database *sql.DB, keep int) error {
	if keep < 1 {
		return fmt.Errorf("revision retention must retain at least one revision")
	}
	_, err := database.ExecContext(ctx, `DELETE FROM graph_revisions r
WHERE r.status='published'
  AND r.id <> COALESCE((SELECT revision_id FROM graph_revision_current WHERE singleton), -1)
  AND r.id IN (
    SELECT id FROM graph_revisions WHERE status='published'
    ORDER BY published_at DESC NULLS LAST,id DESC OFFSET $1
  )`, keep)
	return err
}
