package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
)

// SpatialCatalogOptions bounds the operational footprint of a full-corpus
// placement without changing its semantic contents. Every entity at the source
// watermark is included; these options control PostgreSQL memory and retention.
type SpatialCatalogOptions struct {
	Retention int
	WorkMemMB int
}

func DefaultSpatialCatalogOptions() SpatialCatalogOptions {
	return SpatialCatalogOptions{
		Retention: positiveEnv("SPATIAL_CATALOG_RETENTION", 2),
		WorkMemMB: positiveEnv("SPATIAL_CATALOG_WORK_MEM_MB", 256),
	}
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
	if options.Retention < 1 || options.WorkMemMB < 1 {
		return 0, fmt.Errorf("spatial catalog retention and work memory must be positive")
	}
	if options.WorkMemMB > 2048 {
		options.WorkMemMB = 2048
	}
	config, _ := json.Marshal(map[string]any{
		"placement":  "anchored-semantic-3d-v1",
		"projection": "full-corpus-typed-weighted-v1",
		"labels":     "semantic-label-v1",
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
	// Container defaults commonly expose only 64 MiB of /dev/shm. Parallel
	// aggregate workers multiply dynamic shared-memory demand and can make the
	// final validation fail after all rows are staged. One worker is predictable
	// and keeps publication independent of Docker's shm-size setting.
	if _, err = tx.ExecContext(ctx, `SELECT set_config('max_parallel_workers_per_gather','0',true)`); err != nil {
		return failTx("parallelism", err)
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
  WHERE r.updated_at <= $1
  UNION ALL
  SELECT r.target_subreddit_id,s.subreddit_id,s.x,s.y,s.z,s.community_id,r.overlap_count weight
  FROM subreddit_relationships r JOIN catalog_resident_subreddits s ON s.subreddit_id=r.source_subreddit_id
  WHERE r.updated_at <= $1
) candidates ORDER BY candidate_id,weight DESC,anchor_id`, watermark)
	if err != nil {
		return failTx("related_subreddit_seeds", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_related_subreddit_id ON catalog_related_subreddit_seeds(candidate_id); ANALYZE catalog_related_subreddit_seeds`); err != nil {
		return failTx("related_subreddit_index", err)
	}

	// Existing entities never jump when a new catalog is built. New subreddits
	// prefer the graph-revision seed, then their strongest seeded neighbor.
	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at)
SELECT $1,'subreddit_'||s.id,COALESCE(NULLIF(btrim(s.name),''),'Subreddit '||s.id),'subreddit',GREATEST(COALESCE(s.subscribers,0),0),
 COALESCE(previous.x,resident.x,related.x + (hashtextextended(s.id::text,11)%2000)/100.0,(hashtextextended(s.id::text,11)%1000000)/10.0),
 COALESCE(previous.y,resident.y,related.y + (hashtextextended(s.id::text,12)%2000)/100.0,(hashtextextended(s.id::text,12)%1000000)/10.0),
 COALESCE(previous.z,resident.z,related.z + (hashtextextended(s.id::text,13)%2000)/100.0,(hashtextextended(s.id::text,13)%1000000)/10.0),
 NULL,
 CASE WHEN resident.id IS NOT NULL THEN resident.id WHEN related.anchor_id IS NOT NULL THEN 'subreddit_'||related.anchor_id END,
 NULL,
 COALESCE(previous.community_id,resident.community_id,related.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' WHEN resident.id IS NOT NULL THEN 'revision-seeded' WHEN related.anchor_id IS NOT NULL THEN 'overlap-seeded' ELSE 'deterministic-global' END,
 COALESCE(s.updated_at,s.created_at,to_timestamp(0))
FROM subreddits s
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='subreddit_'||s.id
LEFT JOIN catalog_resident_subreddits resident ON resident.subreddit_id=s.id
LEFT JOIN catalog_related_subreddit_seeds related ON related.candidate_id=s.id
WHERE COALESCE(s.updated_at,s.created_at,to_timestamp(0)) <= $2`, catalogID, watermark)
	if err != nil {
		return failTx("subreddit_entities", err)
	}

	// One strongest activity anchor gives each user a semantic home without a
	// memory-heavy all-user force simulation. Repeated activity becomes value.
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE catalog_user_anchors ON COMMIT DROP AS
SELECT DISTINCT ON (a.user_id) a.user_id,a.subreddit_id,a.activity_count,s.x,s.y,s.z,s.community_id
FROM user_subreddit_activity a
JOIN spatial_catalog_entities s ON s.catalog_id=$1 AND s.id='subreddit_'||a.subreddit_id
WHERE a.updated_at <= $2
ORDER BY a.user_id,a.activity_count DESC,a.subreddit_id`, catalogID, watermark)
	if err != nil {
		return failTx("user_anchors", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX catalog_user_anchor_id ON catalog_user_anchors(user_id); ANALYZE catalog_user_anchors`); err != nil {
		return failTx("user_anchor_index", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at)
SELECT $1,'user_'||u.id,COALESCE(NULLIF(btrim(u.username),''),'User '||u.id),'user',GREATEST(COALESCE(anchor.activity_count,0),0),
 COALESCE(previous.x,anchor.x + (hashtextextended(u.id::text,21)%5000)/100.0,(hashtextextended(u.id::text,21)%1000000)/10.0),
 COALESCE(previous.y,anchor.y + (hashtextextended(u.id::text,22)%5000)/100.0,(hashtextextended(u.id::text,22)%1000000)/10.0),
 COALESCE(previous.z,anchor.z + (hashtextextended(u.id::text,23)%5000)/100.0,(hashtextextended(u.id::text,23)%1000000)/10.0),
 NULL,CASE WHEN anchor.subreddit_id IS NOT NULL THEN 'subreddit_'||anchor.subreddit_id END,NULL,
 COALESCE(previous.community_id,anchor.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' WHEN anchor.subreddit_id IS NOT NULL THEN 'activity-seeded' ELSE 'deterministic-global' END,
 COALESCE(u.updated_at,u.created_at,to_timestamp(0))
FROM users u
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='user_'||u.id
LEFT JOIN catalog_user_anchors anchor ON anchor.user_id=u.id
WHERE COALESCE(u.updated_at,u.created_at,to_timestamp(0)) <= $2`, catalogID, watermark)
	if err != nil {
		return failTx("user_entities", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at)
SELECT $1,'post_'||p.id,COALESCE(NULLIF(left(regexp_replace(p.title,E'[\\n\\r\\t]+',' ','g'),160),''),'Post '||p.id),'post',GREATEST(COALESCE(p.score,0),0),
 COALESCE(previous.x,subreddit.x + (hashtextextended(p.id,31)%800)/100.0),
 COALESCE(previous.y,subreddit.y + (hashtextextended(p.id,32)%800)/100.0),
 COALESCE(previous.z,subreddit.z + (hashtextextended(p.id,33)%800)/100.0),
 'subreddit_'||p.subreddit_id,'subreddit_'||p.subreddit_id,'user_'||p.author_id,COALESCE(previous.community_id,subreddit.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' ELSE 'subreddit-seeded' END,
 COALESCE(p.updated_at,p.created_at,to_timestamp(0))
FROM posts p
JOIN spatial_catalog_entities subreddit ON subreddit.catalog_id=$1 AND subreddit.id='subreddit_'||p.subreddit_id
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='post_'||p.id
WHERE COALESCE(p.updated_at,p.created_at,to_timestamp(0)) <= $2`, catalogID, watermark)
	if err != nil {
		return failTx("post_entities", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_entities(
catalog_id,id,label,type,value,x,y,z,parent_id,anchor_id,author_id,community_id,position_provenance,source_updated_at)
SELECT $1,'comment_'||c.id,
 COALESCE(NULLIF(btrim(left(regexp_replace(c.body,E'[\\n\\r\\t]+',' ','g'),160)),''),'Comment '||c.id),
 'comment',GREATEST(COALESCE(c.score,0),0),
 COALESCE(previous.x,post.x + (hashtextextended(c.id,41)%200)/100.0),
 COALESCE(previous.y,post.y + (hashtextextended(c.id,42)%200)/100.0),
 COALESCE(previous.z,post.z + (hashtextextended(c.id,43)%200)/100.0 + COALESCE(c.depth,0)*0.15),
 CASE WHEN c.parent_id LIKE 't1_%' THEN 'comment_'||substring(c.parent_id FROM 4) ELSE 'post_'||c.post_id END,
 'post_'||c.post_id,'user_'||c.author_id,COALESCE(previous.community_id,post.community_id),
 CASE WHEN previous.id IS NOT NULL THEN 'warm-start' ELSE 'post-seeded' END,
 COALESCE(c.updated_at,c.created_at,to_timestamp(0))
FROM comments c
JOIN spatial_catalog_entities post ON post.catalog_id=$1 AND post.id='post_'||c.post_id
LEFT JOIN spatial_catalog_current current ON current.singleton
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='comment_'||c.id
WHERE COALESCE(c.updated_at,c.created_at,to_timestamp(0)) <= $2`, catalogID, watermark)
	if err != nil {
		return failTx("comment_entities", err)
	}

	logger.InfoContext(ctx, "Spatial entities placed", "catalog_id", catalogID)
	// Normalized semantic links are inserted in deterministic groups. Joins to
	// the catalog enforce the source watermark and prevent orphan endpoints.
	linkStatements := []struct {
		stage string
		query string
	}{
		{"user_activity_links", `INSERT INTO spatial_catalog_links(catalog_id,source,target,relation,directed,weight)
SELECT $1,'user_'||a.user_id,'subreddit_'||a.subreddit_id,'user_activity',true,a.activity_count
FROM user_subreddit_activity a
WHERE a.updated_at <= $2 AND a.activity_count>0`},
		{"subreddit_overlap_links", `INSERT INTO spatial_catalog_links(catalog_id,source,target,relation,directed,weight)
SELECT $1,'subreddit_'||LEAST(r.source_subreddit_id,r.target_subreddit_id),
 'subreddit_'||GREATEST(r.source_subreddit_id,r.target_subreddit_id),'subreddit_overlap',false,max(r.overlap_count)::bigint
FROM subreddit_relationships r
WHERE r.updated_at <= $2 AND r.source_subreddit_id<>r.target_subreddit_id AND r.overlap_count>0
GROUP BY LEAST(r.source_subreddit_id,r.target_subreddit_id),GREATEST(r.source_subreddit_id,r.target_subreddit_id)`},
	}
	for _, statement := range linkStatements {
		if _, err = tx.ExecContext(ctx, statement.query, catalogID, watermark); err != nil {
			return failTx(statement.stage, err)
		}
		logger.InfoContext(ctx, "Spatial relationship stage complete", "catalog_id", catalogID, "stage", statement.stage)
	}

	var entities, links, subreddits, users, posts, comments, emptyLabels, orphanLinks int64
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
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM spatial_catalog_links l
LEFT JOIN spatial_catalog_entities s ON s.catalog_id=l.catalog_id AND s.id=l.source
LEFT JOIN spatial_catalog_entities t ON t.catalog_id=l.catalog_id AND t.id=l.target
WHERE l.catalog_id=$1 AND (s.id IS NULL OR t.id IS NULL)`, catalogID).Scan(&orphanLinks)
	}
	if err != nil {
		return failTx("counts", err)
	}
	var expectedSubreddits, expectedUsers, expectedPosts, expectedComments int64
	err = tx.QueryRowContext(ctx, `SELECT
	(SELECT count(*) FROM subreddits WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1),
	(SELECT count(*) FROM users WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1),
	(SELECT count(*) FROM posts WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1),
	(SELECT count(*) FROM comments WHERE COALESCE(updated_at,created_at,to_timestamp(0)) <= $1)`, watermark).Scan(
		&expectedSubreddits, &expectedUsers, &expectedPosts, &expectedComments)
	if err != nil {
		return failTx("expected_count", err)
	}
	expectedEntities := expectedSubreddits + expectedUsers + expectedPosts + expectedComments
	if entities != expectedEntities || subreddits != expectedSubreddits || users != expectedUsers || posts != expectedPosts || comments != expectedComments ||
		entities == 0 || links == 0 || emptyLabels != 0 || orphanLinks != 0 || minX == maxX || minY == maxY || minZ == maxZ ||
		minX < -1e12 || maxX > 1e12 || minY < -1e12 || maxY > 1e12 || minZ < -1e12 || maxZ > 1e12 {
		return failTx("validation", fmt.Errorf("invalid full spatial catalog: entities=%d expected=%d types=[%d,%d,%d,%d] expected_types=[%d,%d,%d,%d] links=%d empty_labels=%d orphan_links=%d bounds=[%g,%g,%g,%g,%g,%g]", entities, expectedEntities, subreddits, users, posts, comments, expectedSubreddits, expectedUsers, expectedPosts, expectedComments, links, emptyLabels, orphanLinks, minX, maxX, minY, maxY, minZ, maxZ))
	}
	validation := `{"valid":true,"complete_source_entity_coverage":true,"finite_coordinates":true,"noncollapsed_bounds":true,"no_orphan_links":true,"semantic_labels":true}`
	_, err = tx.ExecContext(ctx, `UPDATE spatial_catalogs SET status='published',published_at=now(),entity_count=$2,link_count=$3,
subreddit_count=$4,user_count=$5,post_count=$6,comment_count=$7,min_x=$8,max_x=$9,min_y=$10,max_y=$11,min_z=$12,max_z=$13,
validation_result=$14::jsonb WHERE id=$1`, catalogID, entities, links, subreddits, users, posts, comments, minX, maxX, minY, maxY, minZ, maxZ, validation)
	if err != nil {
		return failTx("finalize", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_current(singleton,catalog_id) VALUES(true,$1)
ON CONFLICT(singleton) DO UPDATE SET catalog_id=EXCLUDED.catalog_id`, catalogID); err != nil {
		return failTx("promote", err)
	}
	if err = tx.Commit(); err != nil {
		return failTx("commit", err)
	}
	if err := RetainSpatialCatalogs(ctx, database, options.Retention); err != nil {
		logger.WarnContext(ctx, "Published spatial catalog but retention cleanup failed", "catalog_id", catalogID, "error", err)
	}
	logger.InfoContext(ctx, "Full spatial catalog published", "catalog_id", catalogID, "entities", entities, "links", links)
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
