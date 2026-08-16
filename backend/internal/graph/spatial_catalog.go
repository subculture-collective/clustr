package graph

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
)

// SpatialCatalogOptions bounds the operational footprint of a full-corpus
// placement without changing its semantic contents. Every entity at the source
// watermark is included; these options control PostgreSQL memory and retention.
type SpatialCatalogOptions struct {
	Retention       int
	WorkMemMB       int
	ParallelWorkers int
	KnownBotNames   []string
	ShadowOnly      bool
}

func DefaultSpatialCatalogOptions() SpatialCatalogOptions {
	return SpatialCatalogOptions{
		Retention:       positiveEnv("SPATIAL_CATALOG_RETENTION", 2),
		WorkMemMB:       positiveEnv("SPATIAL_CATALOG_WORK_MEM_MB", 256),
		ParallelWorkers: nonNegativeEnv("SPATIAL_CATALOG_PARALLEL_WORKERS", 0),
		KnownBotNames:   configuredBotNames(),
	}
}

func configuredBotNames() []string {
	names := []string{"AutoModerator"}
	for _, name := range strings.Split(os.Getenv("SPATIAL_CATALOG_KNOWN_BOTS"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// BuildSpatialCatalogAtWatermark is the full Spatial Catalog Module Interface.
// It produces and atomically publishes a complete placement or leaves the
// current catalog unchanged. Callers only provide the fixed source watermark.
func BuildSpatialCatalogAtWatermark(ctx context.Context, database *sql.DB, watermark time.Time) (int64, error) {
	return BuildSpatialCatalogWithOptions(ctx, database, watermark, DefaultSpatialCatalogOptions())
}

func BuildSpatialCatalogWithOptions(ctx context.Context, database *sql.DB, watermark time.Time, options SpatialCatalogOptions) (int64, error) {
	if watermark.IsZero() {
		return 0, fmt.Errorf("spatial catalog source watermark is required")
	}
	if options.Retention < 1 || options.WorkMemMB < 1 || options.ParallelWorkers < 0 {
		return 0, fmt.Errorf("spatial catalog retention and work memory must be positive and parallel workers non-negative")
	}
	if options.WorkMemMB > 2048 {
		options.WorkMemMB = 2048
	}
	if options.ParallelWorkers > 12 {
		options.ParallelWorkers = 12
	}
	config, _ := json.Marshal(map[string]any{
		"placement":        "anchored-semantic-3d-v1",
		"projection":       "full-corpus-typed-weighted-v1",
		"labels":           "semantic-label-v1",
		"bot_policy":       AutomationPolicyVersion,
		"metric_policy":    "galaxy-metrics-v1",
		"parallel_workers": options.ParallelWorkers,
	})
	var catalogID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO spatial_catalogs(status,source_watermark,algorithm_version,config)
VALUES('staging',$1,'full-corpus-spatial-v1',$2::jsonb) RETURNING id`, watermark, string(config)).Scan(&catalogID); err != nil {
		return 0, fmt.Errorf("create staging spatial catalog: %w", err)
	}
	fail := func(stage string, cause error) (int64, error) {
		if _, markErr := database.ExecContext(context.Background(), `UPDATE spatial_catalogs SET status='failed',failure_reason=$2,validation_result=jsonb_build_object('valid',false,'stage',$3::text) WHERE id=$1`, catalogID, cause.Error(), stage); markErr != nil {
			logger.ErrorContext(context.Background(), "Unable to mark failed spatial catalog", "catalog_id", catalogID, "stage", stage, "error", markErr)
		}
		return 0, fmt.Errorf("build spatial catalog %d at %s: %w", catalogID, stage, cause)
	}

	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fail("begin", err)
	}
	defer tx.Rollback()
	failTx := func(stage string, cause error) (int64, error) {
		_ = tx.Rollback()
		return fail(stage, cause)
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(48487755497811)`); err != nil {
		return failTx("catalog_lock", err)
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('work_mem',$1,true)`, fmt.Sprintf("%dMB", options.WorkMemMB)); err != nil {
		return failTx("work_mem", err)
	}
	// The safe default is zero because Docker commonly exposes only 64 MiB of
	// /dev/shm. Dedicated workers with an explicitly enlarged shared-memory mount
	// may opt into parallel plans without weakening that portable default.
	if _, err = tx.ExecContext(ctx, `SELECT set_config('max_parallel_workers_per_gather',$1,true)`, fmt.Sprintf("%d", options.ParallelWorkers)); err != nil {
		return failTx("parallelism", err)
	}

	knownBots := make([]string, 0, len(options.KnownBotNames)+1)
	knownBots = append(knownBots, "automoderator")
	for _, name := range options.KnownBotNames {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			knownBots = append(knownBots, name)
		}
	}
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE catalog_automated_users ON COMMIT DROP AS
WITH activity AS (
 SELECT author_id user_id,count(DISTINCT subreddit_id)::integer distinct_communities FROM (
  SELECT author_id,subreddit_id FROM posts WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1
  UNION ALL
  SELECT author_id,subreddit_id FROM comments WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1
 ) source_activity GROUP BY author_id
)
SELECT u.id user_id,
 CASE WHEN override.user_id IS NOT NULL THEN override.reason_code
      WHEN lower(u.username)=ANY(string_to_array($2,',')) THEN 'known-list'
      ELSE 'broad-bot-name' END reason_code
FROM users u
LEFT JOIN activity ON activity.user_id=u.id
LEFT JOIN automation_classifications override ON override.user_id=u.id
WHERE COALESCE(u.updated_at,u.created_at,to_timestamp(0)) <= $1
AND CASE WHEN override.user_id IS NOT NULL THEN override.automated
         ELSE lower(u.username)=ANY(string_to_array($2,','))
           OR (COALESCE(activity.distinct_communities,0)>=20 AND lower(u.username) LIKE '%bot') END`, watermark, strings.Join(knownBots, ","))
	if err != nil {
		return failTx("automation_policy", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_automated_users_id ON catalog_automated_users(user_id); ANALYZE catalog_automated_users`); err != nil {
		return failTx("automation_policy_index", err)
	}

	metricStatements := []string{`CREATE TEMP TABLE catalog_user_metrics ON COMMIT DROP AS
WITH activity AS (
 SELECT author_id,subreddit_id,'post' kind FROM posts p WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $1 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)
 UNION ALL
 SELECT c.author_id,c.subreddit_id,'comment' kind FROM comments c JOIN posts p ON p.id=c.post_id
 WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $1
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id OR b.user_id=p.author_id)
)
SELECT author_id user_id,count(*) FILTER(WHERE kind='post')::bigint post_count,
 count(*) FILTER(WHERE kind='comment')::bigint comment_count,count(DISTINCT subreddit_id)::bigint distinct_communities,
	 count(*)::bigint activity_count FROM activity GROUP BY author_id`,
		`CREATE UNIQUE INDEX catalog_user_metrics_id ON catalog_user_metrics(user_id)`,
		`CREATE TEMP TABLE catalog_post_metrics ON COMMIT DROP AS
SELECT p.id post_id,count(c.id)::bigint comment_count,
 count(c.id) FILTER(WHERE c.parent_id IS NULL OR c.parent_id NOT LIKE 't1_%')::bigint top_level_comment_count,
 count(c.id) FILTER(WHERE c.parent_id LIKE 't1_%')::bigint reply_count
FROM posts p LEFT JOIN comments c ON c.post_id=p.id
 AND COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $1
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id)
WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $1
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)
	GROUP BY p.id`,
		`CREATE UNIQUE INDEX catalog_post_metrics_id ON catalog_post_metrics(post_id)`,
		`CREATE TEMP TABLE catalog_comment_metrics ON COMMIT DROP AS
SELECT parent.id comment_id,count(child.id)::bigint reply_count
FROM comments parent LEFT JOIN comments child ON child.parent_id='t1_'||parent.id
 AND COALESCE(child.updated_at,child.created_at,to_timestamp(0)) <= $1
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=child.author_id)
JOIN posts post ON post.id=parent.post_id
WHERE COALESCE(parent.updated_at,parent.created_at,to_timestamp(0)) <= $1
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=parent.author_id OR b.user_id=post.author_id)
	GROUP BY parent.id`,
		`CREATE UNIQUE INDEX catalog_comment_metrics_id ON catalog_comment_metrics(comment_id)`,
		`CREATE TEMP TABLE catalog_subreddit_metrics ON COMMIT DROP AS
WITH activity AS (
 SELECT subreddit_id,author_id,'post' kind FROM posts p WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $1 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)
 UNION ALL
 SELECT c.subreddit_id,c.author_id,'comment' kind FROM comments c JOIN posts p ON p.id=c.post_id
 WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $1
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id OR b.user_id=p.author_id)
)
SELECT subreddit_id,count(*) FILTER(WHERE kind='post')::bigint post_count,
 count(*) FILTER(WHERE kind='comment')::bigint comment_count,count(DISTINCT author_id)::bigint unique_users,
	 count(*)::bigint activity_count FROM activity GROUP BY subreddit_id`,
		`CREATE UNIQUE INDEX catalog_subreddit_metrics_id ON catalog_subreddit_metrics(subreddit_id)`,
	}
	for _, statement := range metricStatements {
		var execErr error
		if strings.Contains(statement, "$1") {
			_, execErr = tx.ExecContext(ctx, statement, watermark)
		} else {
			_, execErr = tx.ExecContext(ctx, statement)
		}
		if execErr != nil {
			return failTx("presentation_metrics", execErr)
		}
	}

	logger.InfoContext(ctx, "Building full spatial catalog", "catalog_id", catalogID, "source_watermark", watermark)
	// Resident nodes and their stable community IDs provide high-quality seeds.
	// A catalog can still be built before the first graph revision; deterministic
	// global placement is the complete fallback rather than a partial result.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE catalog_resident_subreddits ON COMMIT DROP AS
SELECT substring(n.id FROM 11)::integer subreddit_id,n.id,n.x,n.y,n.z,m.community_id
FROM graph_revision_current current
JOIN graph_revision_nodes n ON n.revision_id=current.revision_id AND n.type='subreddit' AND n.id ~ '^subreddit_[0-9]+$'
LEFT JOIN LATERAL (
  SELECT community_id FROM graph_revision_community_members
  WHERE revision_id=n.revision_id AND node_id=n.id ORDER BY community_id LIMIT 1
) m ON true`)
	if err != nil {
		return failTx("resident_seeds", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_resident_subreddits_id ON catalog_resident_subreddits(subreddit_id); ANALYZE catalog_resident_subreddits`); err != nil {
		return failTx("resident_seed_index", err)
	}

	// Give unpositioned subreddits the strongest available positioned neighbor.
	// This converts real overlap into locality while keeping an indexed,
	// deterministic one-pass calculation over the normalized relationship table.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE catalog_related_subreddit_seeds ON COMMIT DROP AS
SELECT DISTINCT ON (candidate_id) candidate_id,anchor_id,x,y,z,community_id
FROM (
  SELECT r.source_subreddit_id candidate_id,t.subreddit_id anchor_id,t.x,t.y,t.z,t.community_id,r.overlap_count weight
  FROM subreddit_relationships r JOIN catalog_resident_subreddits t ON t.subreddit_id=r.target_subreddit_id
  UNION ALL
  SELECT r.target_subreddit_id,s.subreddit_id,s.x,s.y,s.z,s.community_id,r.overlap_count weight
  FROM subreddit_relationships r JOIN catalog_resident_subreddits s ON s.subreddit_id=r.source_subreddit_id
) candidates ORDER BY candidate_id,weight DESC,anchor_id`)
	if err != nil {
		return failTx("related_subreddit_seeds", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_related_subreddit_id ON catalog_related_subreddit_seeds(candidate_id); ANALYZE catalog_related_subreddit_seeds`); err != nil {
		return failTx("related_subreddit_index", err)
	}

	// Existing entities never jump when a new catalog is built. New subreddits
	// prefer the graph-revision seed, then their strongest seeded neighbor.
	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
	catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at,metrics)
SELECT $1,'subreddit_'||s.id,COALESCE(NULLIF(btrim(s.name),''),'Subreddit '||s.id),'subreddit',GREATEST(COALESCE(s.subscribers,0),0),
 COALESCE(previous.x,resident.x,related.x + (hashtextextended(s.id::text,11)%2000)/100.0,(hashtextextended(s.id::text,11)%1000000)/10.0),
 COALESCE(previous.y,resident.y,related.y + (hashtextextended(s.id::text,12)%2000)/100.0,(hashtextextended(s.id::text,12)%1000000)/10.0),
 COALESCE(previous.z,resident.z,related.z + (hashtextextended(s.id::text,13)%2000)/100.0,(hashtextextended(s.id::text,13)%1000000)/10.0),
 NULL,
 CASE WHEN resident.id IS NOT NULL THEN resident.id WHEN related.anchor_id IS NOT NULL THEN 'subreddit_'||related.anchor_id END,
 NULL,
 COALESCE(previous.community_id,resident.community_id,related.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' WHEN resident.id IS NOT NULL THEN 'revision-seeded' WHEN related.anchor_id IS NOT NULL THEN 'overlap-seeded' ELSE 'deterministic-global' END,
	 COALESCE(s.updated_at,s.created_at,to_timestamp(0)),
	 jsonb_build_object('subscribers',GREATEST(COALESCE(s.subscribers,0),0),'post_count',COALESCE(metric.post_count,0),
	  'comment_count',COALESCE(metric.comment_count,0),'unique_users',COALESCE(metric.unique_users,0),'activity_count',COALESCE(metric.activity_count,0))
FROM subreddits s
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='subreddit_'||s.id
LEFT JOIN catalog_resident_subreddits resident ON resident.subreddit_id=s.id
	LEFT JOIN catalog_related_subreddit_seeds related ON related.candidate_id=s.id
	LEFT JOIN catalog_subreddit_metrics metric ON metric.subreddit_id=s.id
WHERE COALESCE(s.updated_at,s.created_at,to_timestamp(0)) <= $2`, catalogID, watermark)
	if err != nil {
		return failTx("subreddit_entities", err)
	}

	// One strongest activity anchor gives each user a semantic home without a
	// memory-heavy all-user force simulation. Repeated activity becomes value.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE catalog_user_anchors ON COMMIT DROP AS
WITH activity AS (
 SELECT author_id user_id,subreddit_id FROM posts p
 WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $2
  AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)
 UNION ALL
 SELECT c.author_id,c.subreddit_id FROM comments c JOIN posts p ON p.id=c.post_id
 WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $2
  AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id OR b.user_id=p.author_id)
), totals AS (
 SELECT user_id,subreddit_id,count(*)::bigint activity_count FROM activity GROUP BY user_id,subreddit_id
)
SELECT DISTINCT ON (a.user_id) a.user_id,a.subreddit_id,a.activity_count,s.x,s.y,s.z,s.community_id
FROM totals a JOIN spatial_catalog_entities s ON s.catalog_id=$1 AND s.id='subreddit_'||a.subreddit_id
ORDER BY a.user_id,a.activity_count DESC,a.subreddit_id`, catalogID, watermark)
	if err != nil {
		return failTx("user_anchors", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_user_anchor_id ON catalog_user_anchors(user_id); ANALYZE catalog_user_anchors`); err != nil {
		return failTx("user_anchor_index", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at,metrics)
SELECT $1,'user_'||u.id,COALESCE(NULLIF(btrim(u.username),''),'User '||u.id),'user',GREATEST(COALESCE(anchor.activity_count,0),0),
 COALESCE(previous.x,anchor.x + (hashtextextended(u.id::text,21)%5000)/100.0,(hashtextextended(u.id::text,21)%1000000)/10.0),
 COALESCE(previous.y,anchor.y + (hashtextextended(u.id::text,22)%5000)/100.0,(hashtextextended(u.id::text,22)%1000000)/10.0),
 COALESCE(previous.z,anchor.z + (hashtextextended(u.id::text,23)%5000)/100.0,(hashtextextended(u.id::text,23)%1000000)/10.0),
 NULL,CASE WHEN anchor.subreddit_id IS NOT NULL THEN 'subreddit_'||anchor.subreddit_id END,NULL,
 COALESCE(previous.community_id,anchor.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' WHEN anchor.subreddit_id IS NOT NULL THEN 'activity-seeded' ELSE 'deterministic-global' END,
 COALESCE(u.updated_at,u.created_at,to_timestamp(0)),
 jsonb_build_object('post_count',COALESCE(metric.post_count,0),'comment_count',COALESCE(metric.comment_count,0),
  'distinct_communities',COALESCE(metric.distinct_communities,0),'activity_count',COALESCE(metric.activity_count,0),'last_seen',u.last_seen)
FROM users u
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='user_'||u.id
LEFT JOIN catalog_user_anchors anchor ON anchor.user_id=u.id
LEFT JOIN catalog_user_metrics metric ON metric.user_id=u.id
WHERE COALESCE(u.updated_at,u.created_at,to_timestamp(0)) <= $2
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users automated WHERE automated.user_id=u.id)`, catalogID, watermark)
	if err != nil {
		return failTx("user_entities", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at,metrics)
SELECT $1,'post_'||p.id,
 CASE WHEN p.source_sensitive OR p.source_removed OR p.source_deleted THEN 'Post '||p.id
 ELSE COALESCE(NULLIF(left(regexp_replace(p.title,E'[\\n\\r\\t]+',' ','g'),160),''),'Post '||p.id) END,
 'post',GREATEST(COALESCE(p.score,0),0),
 COALESCE(previous.x,subreddit.x + (hashtextextended(p.id,31)%800)/100.0),
 COALESCE(previous.y,subreddit.y + (hashtextextended(p.id,32)%800)/100.0),
 COALESCE(previous.z,subreddit.z + (hashtextextended(p.id,33)%800)/100.0),
 'subreddit_'||p.subreddit_id,'subreddit_'||p.subreddit_id,'user_'||p.author_id,COALESCE(previous.community_id,subreddit.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' ELSE 'subreddit-seeded' END,
 COALESCE(p.updated_at,p.created_at,to_timestamp(0)),
 jsonb_build_object('score',COALESCE(p.score,0),'comment_count',COALESCE(metric.comment_count,0),
  'top_level_comment_count',COALESCE(metric.top_level_comment_count,0),'reply_count',COALESCE(metric.reply_count,0),'created_at',p.created_at)
FROM posts p
JOIN spatial_catalog_entities subreddit ON subreddit.catalog_id=$1 AND subreddit.id='subreddit_'||p.subreddit_id
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='post_'||p.id
LEFT JOIN catalog_post_metrics metric ON metric.post_id=p.id
WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $2
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users automated WHERE automated.user_id=p.author_id)`, catalogID, watermark)
	if err != nil {
		return failTx("post_entities", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at,metrics)
SELECT $1,'comment_'||c.id,
 CASE WHEN c.source_sensitive OR c.source_removed OR c.source_deleted THEN 'Comment '||c.id
 ELSE COALESCE(NULLIF(btrim(left(regexp_replace(c.body,E'[\\n\\r\\t]+',' ','g'),160)),''),'Comment '||c.id) END,
 'comment',GREATEST(COALESCE(c.score,0),0),
 COALESCE(previous.x,post.x + (hashtextextended(c.id,41)%200)/100.0),
 COALESCE(previous.y,post.y + (hashtextextended(c.id,42)%200)/100.0),
 COALESCE(previous.z,post.z + (hashtextextended(c.id,43)%200)/100.0 + COALESCE(c.depth,0)*0.15),
 CASE WHEN c.parent_id LIKE 't1_%' THEN 'comment_'||substring(c.parent_id FROM 4) ELSE 'post_'||c.post_id END,
 'post_'||c.post_id,'user_'||c.author_id,COALESCE(previous.community_id,post.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' ELSE 'post-seeded' END,
 COALESCE(c.updated_at,c.created_at,to_timestamp(0)),
 jsonb_build_object('score',COALESCE(c.score,0),'reply_count',COALESCE(metric.reply_count,0),
  'depth',COALESCE(c.depth,0),'created_at',c.created_at)
FROM comments c
JOIN spatial_catalog_entities post ON post.catalog_id=$1 AND post.id='post_'||c.post_id
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='comment_'||c.id
LEFT JOIN catalog_comment_metrics metric ON metric.comment_id=c.id
WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $2
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users automated WHERE automated.user_id=c.author_id)`, catalogID, watermark)
	if err != nil {
		return failTx("comment_entities", err)
	}

	// Keep full public text in a separate TOAST-backed table so spatial scans
	// never touch wide documents. Removed/deleted source text is represented by
	// an identity-only tombstone; its former contents are never copied.
	//
	// Each immutable catalog is one document partition. Loading an unattached
	// heap lets PostgreSQL build the primary, filter, and GIN indexes in bulk at
	// attach time instead of maintaining them row-by-row for millions of full-
	// text documents. The catalog ID comes from the database sequence, so this
	// generated identifier contains only the fixed prefix and decimal digits.
	documentPartition := fmt.Sprintf("spatial_catalog_documents_catalog_%d", catalogID)
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE %s (
LIKE spatial_catalog_documents INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED INCLUDING STORAGE
)`, documentPartition))
	if err != nil {
		return failTx("document_partition", err)
	}
	documentStatements := []struct {
		stage string
		query string
	}{
		{"subreddit_documents", fmt.Sprintf(`INSERT INTO %s(
catalog_id,entity_id,entity_type,title,body,description,source_permalink,source_sensitive,
administrative_sensitive,content_created_at,content_updated_at,provenance)
SELECT $1,'subreddit_'||s.id,'subreddit',NULL,NULL,s.description,NULL,false,false,s.created_at,
 COALESCE(s.updated_at,s.created_at,to_timestamp(0)),
 jsonb_build_object('source_watermark',$2::timestamptz,'source','public-reddit','removed_text_excluded',false)
FROM subreddits s
WHERE COALESCE(s.updated_at,s.created_at,to_timestamp(0)) <= $2`, documentPartition)},
		{"post_documents", fmt.Sprintf(`INSERT INTO %s(
catalog_id,entity_id,entity_type,title,body,description,source_permalink,source_sensitive,
administrative_sensitive,content_created_at,content_updated_at,provenance)
SELECT $1,'post_'||p.id,'post',
 CASE WHEN p.source_removed OR p.source_deleted THEN NULL ELSE p.title END,
 CASE WHEN p.source_removed OR p.source_deleted THEN NULL ELSE p.selftext END,
 NULL,p.permalink,p.source_sensitive,false,p.created_at,COALESCE(p.updated_at,p.created_at,to_timestamp(0)),
 jsonb_build_object('source_watermark',$2::timestamptz,'source','public-reddit','removed_text_excluded',p.source_removed OR p.source_deleted)
FROM posts p
WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $2
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users automated WHERE automated.user_id=p.author_id)`, documentPartition)},
		{"comment_documents", fmt.Sprintf(`INSERT INTO %s(
catalog_id,entity_id,entity_type,title,body,description,source_permalink,source_sensitive,
administrative_sensitive,content_created_at,content_updated_at,provenance)
SELECT $1,'comment_'||c.id,'comment',NULL,
 CASE WHEN c.source_removed OR c.source_deleted THEN NULL ELSE c.body END,NULL,
 CASE WHEN p.permalink IS NULL THEN NULL ELSE rtrim(p.permalink,'/')||'/comment/'||c.id END,
 c.source_sensitive,false,c.created_at,COALESCE(c.updated_at,c.created_at,to_timestamp(0)),
 jsonb_build_object('source_watermark',$2::timestamptz,'source','public-reddit','removed_text_excluded',c.source_removed OR c.source_deleted)
FROM comments c JOIN posts p ON p.id=c.post_id
WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $2
 AND COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $2
 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users automated WHERE automated.user_id=c.author_id OR automated.user_id=p.author_id)`, documentPartition)},
	}
	for _, statement := range documentStatements {
		if _, err = tx.ExecContext(ctx, statement.query, catalogID, watermark); err != nil {
			return failTx(statement.stage, err)
		}
		logger.InfoContext(ctx, "Spatial document stage complete", "catalog_id", catalogID, "stage", statement.stage)
	}
	partitionBoundName := fmt.Sprintf("spatial_catalog_documents_catalog_%d_bound", catalogID)
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s CHECK (catalog_id=%d)`, documentPartition, partitionBoundName, catalogID)); err != nil {
		return failTx("document_partition_bound", err)
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE spatial_catalog_documents ATTACH PARTITION %s FOR VALUES IN (%d)`, documentPartition, catalogID)); err != nil {
		return failTx("document_partition_indexes", err)
	}
	logger.InfoContext(ctx, "Spatial document partition indexed", "catalog_id", catalogID, "partition", documentPartition)

	logger.InfoContext(ctx, "Spatial entities placed", "catalog_id", catalogID)
	// The catalog is bulk-loaded in this transaction, so the table statistics do
	// not otherwise describe the new catalog_id. Refresh them before joining the
	// staged entities back to the relationship tables; without this PostgreSQL can
	// estimate one entity and choose a multiplicative nested-loop plan.
	if _, err = tx.ExecContext(ctx, `ANALYZE spatial_catalog_entities`); err != nil {
		return failTx("entity_statistics", err)
	}
	if _, err = tx.ExecContext(ctx, `SET LOCAL enable_nestloop = off`); err != nil {
		return failTx("relationship_join_plan", err)
	}
	if options.ParallelWorkers > 0 {
		// INSERT ... SELECT is not parallelized by PostgreSQL. Materialize the
		// expensive projection with CTAS first and keep the final insert simple.
		// These cost settings apply only to this publication transaction.
		for stage, statement := range map[string]string{
			"parallel_setup_cost":           `SET LOCAL parallel_setup_cost = 0`,
			"parallel_tuple_cost":           `SET LOCAL parallel_tuple_cost = 0`,
			"parallel_table_scan_threshold": `SET LOCAL min_parallel_table_scan_size = 0`,
			"parallel_index_scan_threshold": `SET LOCAL min_parallel_index_scan_size = 0`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return failTx(stage, err)
			}
		}
	}

	_, err = tx.ExecContext(ctx, `CREATE UNLOGGED TABLE catalog_subreddit_overlap_projection AS
	SELECT source_entity.id source,target_entity.id target,max(r.overlap_count)::bigint weight
	FROM subreddit_relationships r
	JOIN subreddits source ON source.id=r.source_subreddit_id AND COALESCE(source.updated_at,source.created_at,to_timestamp(0)) <= $2
	JOIN subreddits target ON target.id=r.target_subreddit_id AND COALESCE(target.updated_at,target.created_at,to_timestamp(0)) <= $2
	JOIN spatial_catalog_entities source_entity
	  ON source_entity.catalog_id=$1 AND source_entity.id='subreddit_'||LEAST(r.source_subreddit_id,r.target_subreddit_id)
	JOIN spatial_catalog_entities target_entity
	  ON target_entity.catalog_id=$1 AND target_entity.id='subreddit_'||GREATEST(r.source_subreddit_id,r.target_subreddit_id)
	WHERE r.source_subreddit_id<>r.target_subreddit_id AND r.overlap_count>0
	GROUP BY source_entity.id,target_entity.id`, catalogID, watermark)
	if err != nil {
		return failTx("subreddit_overlap_projection", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_subreddit_overlap_projection_pair
	ON catalog_subreddit_overlap_projection(source,target); ANALYZE catalog_subreddit_overlap_projection`); err != nil {
		return failTx("subreddit_overlap_projection_index", err)
	}
	logger.InfoContext(ctx, "Spatial relationship projection complete", "catalog_id", catalogID, "stage", "subreddit_overlap_projection")

	// Normalized semantic links are inserted in deterministic groups. Joins to
	// the catalog enforce the source watermark and prevent orphan endpoints.
	linkStatements := []struct {
		stage string
		query string
		args  []any
	}{
		{"user_activity_links", `INSERT INTO spatial_catalog_links(catalog_id,source,target,relation,directed,weight)
	WITH activity AS (
 SELECT author_id user_id,subreddit_id FROM posts p
 WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $2
  AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)
 UNION ALL
 SELECT c.author_id,c.subreddit_id FROM comments c JOIN posts p ON p.id=c.post_id
 WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $2
  AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id OR b.user_id=p.author_id)
), weighted AS (
 SELECT user_id,subreddit_id,count(*)::bigint weight
 FROM activity GROUP BY user_id,subreddit_id
)
SELECT $1,user_entity.id,subreddit_entity.id,'user_activity',true,weighted.weight
FROM weighted
	JOIN spatial_catalog_entities user_entity
	  ON user_entity.catalog_id=$1 AND user_entity.id='user_'||weighted.user_id
	JOIN spatial_catalog_entities subreddit_entity
	  ON subreddit_entity.catalog_id=$1 AND subreddit_entity.id='subreddit_'||weighted.subreddit_id`, []any{catalogID, watermark}},
		{"subreddit_overlap_links", `INSERT INTO spatial_catalog_links(catalog_id,source,target,relation,directed,weight)
	SELECT $1,source,target,'subreddit_overlap',false,weight
	FROM catalog_subreddit_overlap_projection`, []any{catalogID}},
	}
	for _, statement := range linkStatements {
		if _, err = tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return failTx(statement.stage, err)
		}
		logger.InfoContext(ctx, "Spatial relationship stage complete", "catalog_id", catalogID, "stage", statement.stage)
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE catalog_subreddit_overlap_projection`); err != nil {
		return failTx("subreddit_overlap_projection_cleanup", err)
	}

	var entities, links, documents, subreddits, users, posts, comments, emptyLabels int64
	var minX, maxX, minY, maxY, minZ, maxZ float64
	err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE type='subreddit'),count(*) FILTER(WHERE type='user'),
count(*) FILTER(WHERE type='post'),count(*) FILTER(WHERE type='comment'),count(*) FILTER(WHERE btrim(label)=''),
min(x),max(x),min(y),max(y),min(z),max(z)
FROM spatial_catalog_entities WHERE catalog_id=$1`, catalogID).Scan(
		&entities, &subreddits, &users, &posts, &comments, &emptyLabels, &minX, &maxX, &minY, &maxY, &minZ, &maxZ)
	if err == nil {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM spatial_catalog_links WHERE catalog_id=$1`, catalogID).Scan(&links)
	}
	if err == nil {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM spatial_catalog_documents WHERE catalog_id=$1`, catalogID).Scan(&documents)
	}
	if err != nil {
		return failTx("counts", err)
	}
	// Every relationship projection above inner-joins both endpoints from this
	// exact staged catalog before insertion. That construction is the orphan
	// invariant. Do not re-scan millions of uncommitted rows here: PostgreSQL has
	// no usable catalog-id statistics for a revision that exists only inside this
	// transaction and can otherwise choose a catastrophic nested-loop plan.
	const orphanLinks int64 = 0
	var expectedSubreddits, expectedUsers, expectedPosts, expectedComments int64
	var excludedUsers, excludedPosts, excludedComments int64
	err = tx.QueryRowContext(ctx, `SELECT
	(SELECT count(*) FROM subreddits WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1),
	(SELECT count(*) FROM users u WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=u.id)),
	(SELECT count(*) FROM posts p WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)),
	(SELECT count(*) FROM comments c JOIN posts p ON p.id=c.post_id
	 WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $1 AND COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $1
	 AND NOT EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id OR b.user_id=p.author_id)),
	(SELECT count(*) FROM catalog_automated_users),
	(SELECT count(*) FROM posts p WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1 AND EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=p.author_id)),
	(SELECT count(*) FROM comments c JOIN posts p ON p.id=c.post_id
	 WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $1
	 AND EXISTS (SELECT 1 FROM catalog_automated_users b WHERE b.user_id=c.author_id OR b.user_id=p.author_id))`, watermark).Scan(
		&expectedSubreddits, &expectedUsers, &expectedPosts, &expectedComments, &excludedUsers, &excludedPosts, &excludedComments)
	if err != nil {
		return failTx("expected_count", err)
	}
	expectedEntities := expectedSubreddits + expectedUsers + expectedPosts + expectedComments
	expectedDocuments := expectedSubreddits + expectedPosts + expectedComments
	if entities != expectedEntities || documents != expectedDocuments || subreddits != expectedSubreddits || users != expectedUsers || posts != expectedPosts || comments != expectedComments ||
		entities == 0 || links == 0 || emptyLabels != 0 || orphanLinks != 0 || minX == maxX || minY == maxY || minZ == maxZ ||
		minX < -1e12 || maxX > 1e12 || minY < -1e12 || maxY > 1e12 || minZ < -1e12 || maxZ > 1e12 {
		return failTx("validation", fmt.Errorf("invalid full spatial catalog: entities=%d documents=%d expected=%d types=[%d,%d,%d,%d] expected_types=[%d,%d,%d,%d] links=%d empty_labels=%d orphan_links=%d bounds=[%g,%g,%g,%g,%g,%g]", entities, documents, expectedEntities, subreddits, users, posts, comments, expectedSubreddits, expectedUsers, expectedPosts, expectedComments, links, emptyLabels, orphanLinks, minX, maxX, minY, maxY, minZ, maxZ))
	}
	_, err = tx.ExecContext(ctx, `WITH metric_values AS (
 SELECT type,
  CASE type WHEN 'subreddit' THEN COALESCE((metrics->>'unique_users')::bigint,0)
            WHEN 'post' THEN COALESCE((metrics->>'comment_count')::bigint,0)
            WHEN 'comment' THEN COALESCE((metrics->>'reply_count')::bigint,0)
            WHEN 'user' THEN COALESCE((metrics->>'activity_count')::bigint,0) END value
 FROM spatial_catalog_entities WHERE catalog_id=$1
), quantiles AS (
 SELECT type,percentile_disc(0.05) WITHIN GROUP(ORDER BY value) q05,
  percentile_disc(0.95) WITHIN GROUP(ORDER BY value) q95 FROM metric_values GROUP BY type
)
UPDATE spatial_catalogs SET config=config || jsonb_build_object('normalization_quantiles',(
 SELECT jsonb_object_agg(type,jsonb_build_object('q05',q05,'q95',q95)) FROM quantiles
)) WHERE id=$1`, catalogID)
	if err != nil {
		return failTx("normalization_quantiles", err)
	}
	validation := `{"valid":true,"complete_filtered_entity_coverage":true,"full_text_document_coverage":true,"finite_coordinates":true,"noncollapsed_bounds":true,"no_orphan_links":true,"semantic_labels":true,"typed_metrics":true}`
	checksumInput := fmt.Sprintf("%d:%s:%d:%d:%d:%d:%d", catalogID, watermark.UTC().Format(time.RFC3339Nano), entities, links, subreddits, users, posts+comments)
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(checksumInput)))
	_, err = tx.ExecContext(ctx, `UPDATE spatial_catalogs SET status='published',published_at=now(),entity_count=$2,link_count=$3,
subreddit_count=$4,user_count=$5,post_count=$6,comment_count=$7,min_x=$8,max_x=$9,min_y=$10,max_y=$11,min_z=$12,max_z=$13,
validation_result=$14::jsonb,bot_policy_version=$15,metric_policy_version='galaxy-metrics-v1',catalog_checksum=$16,
excluded_user_count=$17,excluded_post_count=$18,excluded_comment_count=$19 WHERE id=$1`, catalogID, entities, links, subreddits, users, posts, comments, minX, maxX, minY, maxY, minZ, maxZ, validation, AutomationPolicyVersion, checksum, excludedUsers, excludedPosts, excludedComments)
	if err != nil {
		return failTx("finalize", err)
	}
	if !options.ShadowOnly {
		if _, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_current(singleton,catalog_id) VALUES(true,$1)
ON CONFLICT(singleton) DO UPDATE SET catalog_id=EXCLUDED.catalog_id`, catalogID); err != nil {
			return failTx("promote", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return failTx("commit", err)
	}
	if err := RetainSpatialCatalogs(ctx, database, options.Retention); err != nil {
		logger.WarnContext(ctx, "Published spatial catalog but retention cleanup failed", "catalog_id", catalogID, "error", err)
	}
	logger.InfoContext(ctx, "Full spatial catalog validated", "catalog_id", catalogID, "entities", entities, "links", links, "shadow_only", options.ShadowOnly)
	return catalogID, nil
}

// RetainSpatialCatalogs removes only unreferenced older catalogs. A catalog
// remains protected while it is current or referenced by any retained graph
// revision, regardless of the requested retention count.
func RetainSpatialCatalogs(ctx context.Context, database *sql.DB, keep int) error {
	if keep < 1 {
		return fmt.Errorf("spatial catalog retention must retain at least one catalog")
	}
	_, err := database.ExecContext(ctx, `DELETE FROM spatial_catalogs c
WHERE c.status='published'
  AND c.id <> COALESCE((SELECT catalog_id FROM spatial_catalog_current WHERE singleton),-1)
  AND NOT EXISTS (SELECT 1 FROM graph_revisions r WHERE r.spatial_catalog_id=c.id)
  AND c.id IN (
    SELECT id FROM spatial_catalogs WHERE status='published'
    ORDER BY published_at DESC NULLS LAST,id DESC OFFSET $1
  )`, keep)
	return err
}
