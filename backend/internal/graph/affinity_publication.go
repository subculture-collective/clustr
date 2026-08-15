package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PromoteAffinityShadow atomically publishes a validated affinity build with
// an immutable full-text catalog. It is intentionally never called by a build.
func PromoteAffinityShadow(ctx context.Context, database *sql.DB, buildID, catalogID int64) (int64, error) {
	if buildID < 1 || catalogID < 1 {
		return 0, errors.New("validated affinity build and catalog IDs are required")
	}
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(48487755497812)`); err != nil {
		return 0, err
	}
	var watermark time.Time
	var macroCount int
	err = tx.QueryRowContext(ctx, `SELECT b.source_watermark,(SELECT count(*) FROM affinity_build_macro_groups WHERE build_id=b.id) FROM affinity_builds b WHERE b.id=$1 AND b.status='validated' FOR UPDATE`, buildID).Scan(&watermark, &macroCount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("affinity build is not validated")
	}
	if err != nil {
		return 0, err
	}
	if macroCount < 8 || macroCount > 12 {
		return 0, fmt.Errorf("affinity build has %d macro-groups, expected 8-12", macroCount)
	}
	var minMacroLandmarks int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(min(member_count),0) FROM (SELECT macro_group_id,count(*) member_count FROM affinity_build_communities WHERE build_id=$1 GROUP BY macro_group_id) groups`, buildID).Scan(&minMacroLandmarks); err != nil {
		return 0, err
	}
	if minMacroLandmarks < 5 {
		return 0, fmt.Errorf("a macro-group has only %d fine landmarks; five are required for overview coverage", minMacroLandmarks)
	}
	var topCount, fallbackCount int
	if err = tx.QueryRowContext(ctx, `WITH ranked AS (
SELECT c.naming_method FROM affinity_build_communities c
LEFT JOIN LATERAL (SELECT count(*) size FROM affinity_build_community_members m WHERE m.build_id=c.build_id AND m.community_id=c.community_id) members ON true
WHERE c.build_id=$1 ORDER BY .30*ln(1+members.size)+.30*c.distinctiveness+.25*c.affinity_confidence+.15*CASE WHEN c.is_bridge THEN 1 ELSE 0 END DESC,c.community_id LIMIT 300)
SELECT count(*),count(*) FILTER(WHERE naming_method='deterministic-fallback') FROM ranked`, buildID).Scan(&topCount, &fallbackCount); err != nil {
		return 0, err
	}
	if topCount > 0 && float64(fallbackCount)/float64(topCount) >= .05 {
		return 0, fmt.Errorf("top landmark deterministic-label fallback rate %.2f%% is not below 5%%", 100*float64(fallbackCount)/float64(topCount))
	}
	var catalogWatermark time.Time
	if err = tx.QueryRowContext(ctx, `SELECT source_watermark FROM spatial_catalogs WHERE id=$1 AND status='published' FOR SHARE`, catalogID).Scan(&catalogWatermark); err != nil {
		return 0, fmt.Errorf("catalog unavailable: %w", err)
	}
	if !catalogWatermark.Equal(watermark) {
		return 0, fmt.Errorf("affinity watermark %s does not match catalog watermark %s", watermark, catalogWatermark)
	}
	config, _ := json.Marshal(map[string]any{"projection": "normalized-affinity-v2", "hierarchy": "weighted-multiplex-louvain-v2", "layout": "stable-community-3d-v2", "affinity_build_id": buildID, "catalog_id": catalogID, "manual_promotion": true})
	var revisionID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO graph_revisions(status,source_watermark,algorithm_version,config,layout_algorithm,layout_seed,layout_dimensions,spatial_catalog_id,affinity_build_id,scene_contract) VALUES('staging',$1,'affinity-world-v2',$2::jsonb,'stable-community-3d-v2',74036448965714,3,$3,$4,'spatial-scene-v2') RETURNING id`, watermark, string(config), catalogID, buildID).Scan(&revisionID); err != nil {
		return 0, err
	}
	// Stable identities retain their exact prior positions. New landmarks use a
	// deterministic hash seed and therefore remain byte-stable across retries.
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_communities(revision_id,community_id,parent_id,level,label,size,x,y,z,display_name,evidence_label,macro_group_id,primary_color,secondary_color,distinctiveness,affinity_confidence,bridge_affinity,is_bridge,naming_method,naming_version,naming_confidence,representative_members,evidence_metrics)
SELECT $1,c.community_id,c.macro_group_id,0,c.evidence_label,(SELECT count(*) FROM affinity_build_community_members m WHERE m.build_id=c.build_id AND m.community_id=c.community_id),
 COALESCE(old.x,((hashtext(c.community_id)::bigint%20000)-10000)/10.0),COALESCE(old.y,((hashtext(c.community_id||':y')::bigint%20000)-10000)/10.0),COALESCE(old.z,((hashtext(c.community_id||':z')::bigint%20000)-10000)/10.0),
 c.display_name,c.evidence_label,c.macro_group_id,c.primary_color,c.secondary_color,c.distinctiveness,c.affinity_confidence,COALESCE((c.evidence_metrics->>'bridge_affinity')::float8,0),c.is_bridge,c.naming_method,c.naming_version,c.naming_confidence,c.representative_members,c.evidence_metrics
FROM affinity_build_communities c LEFT JOIN graph_revision_current current ON current.singleton LEFT JOIN graph_revision_communities old ON old.revision_id=current.revision_id AND old.community_id=c.community_id WHERE c.build_id=$2`, revisionID, buildID)
	if err != nil {
		return 0, fmt.Errorf("publish fine communities: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_macro_groups(revision_id,macro_group_id,display_name,evidence_label,palette_role,primary_color,member_count,representative_clusters,evidence_metrics)
SELECT $1,m.macro_group_id,m.display_name,m.evidence_label,m.palette_role,m.primary_color,
(SELECT count(*) FROM affinity_build_communities c WHERE c.build_id=m.build_id AND c.macro_group_id=m.macro_group_id),
COALESCE((SELECT jsonb_agg(representative.community_id ORDER BY representative.rank) FROM (
 SELECT c.community_id,row_number() OVER(ORDER BY c.distinctiveness DESC,c.affinity_confidence DESC,c.community_id) rank
 FROM affinity_build_communities c WHERE c.build_id=m.build_id AND c.macro_group_id=m.macro_group_id LIMIT 12
) representative),'[]'::jsonb),m.evidence_metrics FROM affinity_build_macro_groups m WHERE m.build_id=$2`, revisionID, buildID)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_identity_events(revision_id,layer,event_type,old_ids,new_ids,evidence)
SELECT $1,layer,event_type,old_ids,new_ids,evidence FROM affinity_build_identity_events WHERE build_id=$2`, revisionID, buildID); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_communities(revision_id,community_id,parent_id,level,label,size,x,y,z,display_name,evidence_label,macro_group_id,primary_color,naming_method,naming_version,evidence_metrics)
SELECT $1,m.macro_group_id,NULL,1,m.evidence_label,count(c.community_id),avg(c.x),avg(c.y),avg(c.z),m.display_name,m.evidence_label,m.macro_group_id,m.primary_color,'deterministic-fallback','macro-v1',m.evidence_metrics
FROM graph_revision_macro_groups m JOIN graph_revision_communities c ON c.revision_id=m.revision_id AND c.macro_group_id=m.macro_group_id AND c.level=0 WHERE m.revision_id=$1 GROUP BY m.macro_group_id,m.evidence_label,m.display_name,m.primary_color,m.evidence_metrics`, revisionID)
	if err != nil {
		return 0, err
	}
	// The resident revision remains bounded; the catalog remains complete.
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_nodes(revision_id,id,name,value,type,x,y,z,position_provenance)
SELECT $1,e.id,e.label,e.value,e.type,e.x,e.y,e.z,e.position_provenance FROM spatial_catalog_entities e WHERE e.catalog_id=$2 AND e.type='subreddit' ORDER BY e.value DESC,e.id LIMIT 100000`, revisionID, catalogID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_community_members(revision_id,community_id,node_id)
SELECT $1,m.community_id,'subreddit_'||m.subreddit_id FROM affinity_build_community_members m JOIN graph_revision_nodes n ON n.revision_id=$1 AND n.id='subreddit_'||m.subreddit_id WHERE m.build_id=$2`, revisionID, buildID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_community_members(revision_id,community_id,node_id)
SELECT $1,c.macro_group_id,m.node_id FROM graph_revision_communities c JOIN graph_revision_community_members m ON m.revision_id=c.revision_id AND m.community_id=c.community_id WHERE c.revision_id=$1 AND c.level=0`, revisionID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_links(revision_id,source,target,relation,directed,weight,affinity,observed_evidence,signal_composition)
SELECT $1,'subreddit_'||e.source_subreddit_id,'subreddit_'||e.target_subreddit_id,'normalized_affinity',false,GREATEST(1,round(e.affinity*1000000)::bigint),e.affinity,e.observed_evidence,e.signal_composition
FROM affinity_edges e JOIN graph_revision_nodes a ON a.revision_id=$1 AND a.id='subreddit_'||e.source_subreddit_id JOIN graph_revision_nodes b ON b.revision_id=$1 AND b.id='subreddit_'||e.target_subreddit_id WHERE e.build_id=$2`, revisionID, buildID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_community_links(revision_id,source_community_id,target_community_id,weight,affinity,observed_evidence,signal_composition)
SELECT $1,LEAST(a.community_id,b.community_id),GREATEST(a.community_id,b.community_id),GREATEST(1,round(sum(e.affinity)*1000000)::bigint),LEAST(1,avg(e.affinity)),sum(e.observed_evidence),jsonb_build_object('edge_count',count(*))
FROM affinity_edges e JOIN affinity_build_community_members a ON a.build_id=e.build_id AND a.subreddit_id=e.source_subreddit_id JOIN affinity_build_community_members b ON b.build_id=e.build_id AND b.subreddit_id=e.target_subreddit_id WHERE e.build_id=$2 AND a.community_id<>b.community_id GROUP BY LEAST(a.community_id,b.community_id),GREATEST(a.community_id,b.community_id)`, revisionID, buildID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_macro_group_links(revision_id,source_macro_group_id,target_macro_group_id,affinity,observed_evidence)
SELECT $1,LEAST(a.macro_group_id,b.macro_group_id),GREATEST(a.macro_group_id,b.macro_group_id),LEAST(1,avg(l.affinity)),sum(l.observed_evidence)
FROM graph_revision_community_links l JOIN graph_revision_communities a ON a.revision_id=l.revision_id AND a.community_id=l.source_community_id JOIN graph_revision_communities b ON b.revision_id=l.revision_id AND b.community_id=l.target_community_id WHERE l.revision_id=$1 AND a.macro_group_id<>b.macro_group_id GROUP BY LEAST(a.macro_group_id,b.macro_group_id),GREATEST(a.macro_group_id,b.macro_group_id)`, revisionID)
	if err != nil {
		return 0, err
	}
	// Reserve five overview positions per macro-group, then fill by the approved
	// population/distinctiveness/confidence/bridge blend.
	_, err = tx.ExecContext(ctx, `WITH scored AS (SELECT community_id,macro_group_id,.30*ln(1+size)+.30*distinctiveness+.25*affinity_confidence+.15*CASE WHEN is_bridge THEN 1 ELSE 0 END score FROM graph_revision_communities WHERE revision_id=$1 AND level=0), covered AS (SELECT *,row_number() OVER(PARTITION BY macro_group_id ORDER BY score DESC,community_id) macro_rank FROM scored), ranked AS (SELECT community_id,row_number() OVER(ORDER BY CASE WHEN macro_rank<=5 THEN 0 ELSE 1 END,CASE WHEN macro_rank<=5 THEN macro_rank ELSE 999 END,score DESC,community_id) overview_rank FROM covered) UPDATE graph_revision_communities c SET evidence_metrics=c.evidence_metrics||jsonb_build_object('overview_rank',ranked.overview_rank) FROM ranked WHERE c.revision_id=$1 AND c.community_id=ranked.community_id`, revisionID)
	if err != nil {
		return 0, err
	}
	var nodeCount, linkCount, communityCount int64
	var minX, maxX, minY, maxY, minZ, maxZ float64
	if err = tx.QueryRowContext(ctx, `SELECT count(*),min(x),max(x),min(y),max(y),min(z),max(z) FROM graph_revision_nodes WHERE revision_id=$1`, revisionID).Scan(&nodeCount, &minX, &maxX, &minY, &maxY, &minZ, &maxZ); err != nil {
		return 0, err
	}
	_ = tx.QueryRowContext(ctx, `SELECT count(*) FROM graph_revision_links WHERE revision_id=$1`, revisionID).Scan(&linkCount)
	_ = tx.QueryRowContext(ctx, `SELECT count(*) FROM graph_revision_communities WHERE revision_id=$1`, revisionID).Scan(&communityCount)
	validation, _ := json.Marshal(map[string]any{"valid": true, "manual_promotion": true, "affinity_build_id": buildID, "catalog_id": catalogID, "macro_group_count": macroCount, "coordinate_continuity": "matched-identities-exact", "rollback": "prior revision and catalog retained"})
	_, err = tx.ExecContext(ctx, `UPDATE graph_revisions SET status='published',published_at=now(),node_count=$2,link_count=$3,community_count=$4,min_x=$5,max_x=$6,min_y=$7,max_y=$8,min_z=$9,max_z=$10,validation_result=$11::jsonb WHERE id=$1`, revisionID, nodeCount, linkCount, communityCount, minX, maxX, minY, maxY, minZ, maxZ, string(validation))
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_current(singleton,revision_id) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET revision_id=EXCLUDED.revision_id`, revisionID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_current(singleton,catalog_id) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET catalog_id=EXCLUDED.catalog_id`, catalogID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE affinity_builds SET status='published' WHERE id=$1`, buildID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return revisionID, nil
}

// RollbackPublishedArtifacts atomically restores a retained published
// revision/catalog pair. Additive schema and suppression state intentionally
// remain in place; feature flags are disabled by the operator independently.
func RollbackPublishedArtifacts(ctx context.Context, database *sql.DB, revisionID, catalogID int64) error {
	if revisionID < 1 || catalogID < 1 {
		return errors.New("published revision and catalog IDs are required")
	}
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(48487755497812)`); err != nil {
		return err
	}
	var referencedCatalog int64
	if err = tx.QueryRowContext(ctx, `SELECT spatial_catalog_id FROM graph_revisions WHERE id=$1 AND status='published' FOR SHARE`, revisionID).Scan(&referencedCatalog); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("rollback revision is not a retained published revision")
		}
		return err
	}
	if referencedCatalog != catalogID {
		return fmt.Errorf("revision %d references catalog %d, not requested catalog %d", revisionID, referencedCatalog, catalogID)
	}
	var catalogPublished bool
	if err = tx.QueryRowContext(ctx, `SELECT status='published' FROM spatial_catalogs WHERE id=$1 FOR SHARE`, catalogID).Scan(&catalogPublished); err != nil || !catalogPublished {
		if err == nil || errors.Is(err, sql.ErrNoRows) {
			return errors.New("rollback catalog is not retained and published")
		}
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO graph_revision_current(singleton,revision_id) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET revision_id=EXCLUDED.revision_id`, revisionID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO spatial_catalog_current(singleton,catalog_id) VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET catalog_id=EXCLUDED.catalog_id`, catalogID); err != nil {
		return err
	}
	return tx.Commit()
}
