package affinity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/clusterlabel"
	"github.com/onnwee/reddit-cluster-map/backend/internal/presentation"
)

const BuildAlgorithmVersion = "weighted-multiplex-louvain-v2"
const PublicationSeed uint64 = 0x434c55535452

type stagedPair struct {
	a, b         int64
	score        Result
	rankA, rankB int
}

// BuildShadow stages affinity edges, fine communities, macro-groups, labels,
// and palette facts. It never changes graph_revision_current.
func BuildShadow(ctx context.Context, database *sql.DB, watermark time.Time) (int64, error) {
	if watermark.IsZero() {
		return 0, errors.New("affinity source watermark is required")
	}
	defaults, defaultChecksum := HistoricalDefaults()
	_ = defaults
	configured := DefaultWeights()
	configJSON, _ := json.Marshal(map[string]any{"weights": configured, "edge_floor": map[string]any{"shared_users": 2, "cross_reference": 1}, "top_edges_per_subreddit": 24, "edge_cap": 1000000, "seed": PublicationSeed, "historical_defaults_version": HistoricalDefaultVersion, "historical_defaults_checksum": defaultChecksum, "historical_default_representative_prior": HistoricalDefaultRepresentativePrior})
	var buildID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO affinity_builds(status,source_watermark,seed,algorithm_version,config) VALUES('staging',$1,$2,$3,$4) RETURNING id`, watermark, int64(PublicationSeed), BuildAlgorithmVersion, string(configJSON)).Scan(&buildID); err != nil {
		return 0, fmt.Errorf("create affinity build: %w", err)
	}
	fail := func(stage string, cause error) (int64, error) {
		_, _ = database.ExecContext(context.Background(), `UPDATE affinity_builds SET status='failed',failure_reason=$2,validation_result=jsonb_build_object('valid',false,'stage',$3),completed_at=now() WHERE id=$1`, buildID, cause.Error(), stage)
		return buildID, fmt.Errorf("affinity shadow %d at %s: %w", buildID, stage, cause)
	}
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fail("begin", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(48487755497811)`); err != nil {
		return fail("lock", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE affinity_candidates ON COMMIT DROP AS
SELECT source_subreddit_id source_id,target_subreddit_id target_id FROM subreddit_relationships
WHERE source_subreddit_id<target_subreddit_id AND overlap_count>=2 AND updated_at<=$1
UNION
SELECT LEAST(source_subreddit_id,target_subreddit_id),GREATEST(source_subreddit_id,target_subreddit_id)
FROM subreddit_cross_references WHERE target_subreddit_id IS NOT NULL AND source_subreddit_id<>target_subreddit_id AND observed_at<=$1`, watermark); err != nil {
		return fail("candidates", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX affinity_candidates_pair ON affinity_candidates(source_id,target_id); ANALYZE affinity_candidates`); err != nil {
		return fail("candidate_index", err)
	}
	rows, err := tx.QueryContext(ctx, `WITH
active_marginal AS (SELECT subreddit_id,count(*)::float8 n FROM user_subreddit_activity WHERE updated_at<=$1 GROUP BY subreddit_id),
author_activity AS (SELECT DISTINCT subreddit_id,author_id FROM posts WHERE COALESCE(updated_at,created_at,to_timestamp(0))<=$1 AND NOT source_removed AND NOT source_deleted),
author_marginal AS (SELECT subreddit_id,count(*)::float8 n FROM author_activity GROUP BY subreddit_id),
author_pair AS (SELECT c.source_id,c.target_id,count(*)::float8 n FROM affinity_candidates c JOIN author_activity a ON a.subreddit_id=c.source_id JOIN author_activity b ON b.subreddit_id=c.target_id AND b.author_id=a.author_id GROUP BY c.source_id,c.target_id),
repeat_pair AS (SELECT c.source_id,c.target_id,LEAST(sum(LEAST(a.activity_count,b.activity_count)),100)::float8 n FROM affinity_candidates c JOIN user_subreddit_activity a ON a.subreddit_id=c.source_id AND a.updated_at<=$1 JOIN user_subreddit_activity b ON b.subreddit_id=c.target_id AND b.user_id=a.user_id AND b.updated_at<=$1 GROUP BY c.source_id,c.target_id),
repeat_marginal AS (SELECT subreddit_id,sum(activity_count)::float8 n FROM user_subreddit_activity WHERE updated_at<=$1 GROUP BY subreddit_id),
cross_activity AS (SELECT source_subreddit_id subreddit_id FROM subreddit_cross_references WHERE target_subreddit_id IS NOT NULL AND observed_at<=$1 UNION ALL SELECT target_subreddit_id FROM subreddit_cross_references WHERE target_subreddit_id IS NOT NULL AND observed_at<=$1),
cross_marginal AS (SELECT subreddit_id,count(*)::float8 n FROM cross_activity GROUP BY subreddit_id),
cross_pair AS (SELECT LEAST(source_subreddit_id,target_subreddit_id) source_id,GREATEST(source_subreddit_id,target_subreddit_id) target_id,count(*)::float8 n FROM subreddit_cross_references WHERE target_subreddit_id IS NOT NULL AND source_subreddit_id<>target_subreddit_id AND observed_at<=$1 GROUP BY 1,2),
totals AS (SELECT (SELECT count(DISTINCT user_id)::float8 FROM user_subreddit_activity WHERE updated_at<=$1) active_users,(SELECT count(DISTINCT author_id)::float8 FROM author_activity) authors,(SELECT sum(activity_count)::float8 FROM user_subreddit_activity WHERE updated_at<=$1) repeat_activity,(SELECT count(*)::float8 FROM cross_activity) cross_activity)
SELECT c.source_id,c.target_id,COALESCE(r.overlap_count,0)::float8,la.n,ra.n,t.active_users,COALESCE(ap.n,0),lsa.n,rsa.n,t.authors,COALESCE(rp.n,0),lr.n,rr.n,t.repeat_activity,COALESCE(cp.n,0),lc.n,rc.n,t.cross_activity
FROM affinity_candidates c LEFT JOIN subreddit_relationships r ON r.source_subreddit_id=c.source_id AND r.target_subreddit_id=c.target_id
LEFT JOIN active_marginal la ON la.subreddit_id=c.source_id LEFT JOIN active_marginal ra ON ra.subreddit_id=c.target_id
LEFT JOIN author_pair ap USING(source_id,target_id) LEFT JOIN author_marginal lsa ON lsa.subreddit_id=c.source_id LEFT JOIN author_marginal rsa ON rsa.subreddit_id=c.target_id
LEFT JOIN repeat_pair rp USING(source_id,target_id) LEFT JOIN repeat_marginal lr ON lr.subreddit_id=c.source_id LEFT JOIN repeat_marginal rr ON rr.subreddit_id=c.target_id
LEFT JOIN cross_pair cp USING(source_id,target_id) LEFT JOIN cross_marginal lc ON lc.subreddit_id=c.source_id LEFT JOIN cross_marginal rc ON rc.subreddit_id=c.target_id CROSS JOIN totals t
ORDER BY c.source_id,c.target_id`, watermark)
	if err != nil {
		return fail("evidence_query", err)
	}
	pairs := []stagedPair{}
	crossAvailable := false
	for rows.Next() {
		var a, b int64
		var active, leftActive, rightActive, activeUniverse, authors, leftAuthors, rightAuthors, authorUniverse, repeat, leftRepeat, rightRepeat, repeatUniverse, cross, leftCross, rightCross, crossUniverse sql.NullFloat64
		if err := rows.Scan(&a, &b, &active, &leftActive, &rightActive, &activeUniverse, &authors, &leftAuthors, &rightAuthors, &authorUniverse, &repeat, &leftRepeat, &rightRepeat, &repeatUniverse, &cross, &leftCross, &rightCross, &crossUniverse); err != nil {
			rows.Close()
			return fail("evidence_scan", err)
		}
		crossAvailable = crossAvailable || crossUniverse.Float64 > 0
		e := Evidence{ActiveUsers: Layer{Observed: active.Float64, LeftMarginal: leftActive.Float64, RightMarginal: rightActive.Float64, Universe: activeUniverse.Float64, Available: true}, Authors: Layer{Observed: authors.Float64, LeftMarginal: leftAuthors.Float64, RightMarginal: rightAuthors.Float64, Universe: authorUniverse.Float64, Available: authorUniverse.Float64 > 0}, RepeatCoactivity: Layer{Observed: repeat.Float64, LeftMarginal: leftRepeat.Float64, RightMarginal: rightRepeat.Float64, Universe: repeatUniverse.Float64, Available: repeatUniverse.Float64 > 0}, CrossReferences: Layer{Observed: cross.Float64, LeftMarginal: leftCross.Float64, RightMarginal: rightCross.Float64, Universe: crossUniverse.Float64, Available: crossUniverse.Float64 > 0}}
		result := Score(e, configured)
		if result.Affinity > 0 {
			pairs = append(pairs, stagedPair{a: a, b: b, score: result})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail("evidence_rows", err)
	}
	retained := retainTopEdges(pairs, 24, 1000000)
	if len(retained) == 0 {
		return fail("edge_retention", errors.New("no positive affinity edges retained"))
	}
	copyStatement, err := tx.PrepareContext(ctx, pq.CopyIn("affinity_edges", "build_id", "source_subreddit_id", "target_subreddit_id", "affinity", "raw_affinity", "observed_evidence", "signal_composition", "rank_from_source", "rank_from_target"))
	if err != nil {
		return fail("edge_copy_prepare", err)
	}
	for _, pair := range retained {
		composition, _ := json.Marshal(map[string]any{"components": pair.score.Components, "effective_weights": pair.score.EffectiveWeights})
		var rankA any = pair.rankA
		if pair.rankA == 0 {
			rankA = nil
		}
		var rankB any = pair.rankB
		if pair.rankB == 0 {
			rankB = nil
		}
		if _, err = copyStatement.ExecContext(ctx, buildID, pair.a, pair.b, pair.score.Affinity, pair.score.RawAffinity, pair.score.ObservedEvidence, string(composition), rankA, rankB); err != nil {
			copyStatement.Close()
			return fail("edge_copy", err)
		}
	}
	if _, err = copyStatement.ExecContext(ctx); err != nil {
		return fail("edge_copy_flush", err)
	}
	_ = copyStatement.Close()
	layers, weights := layersFromPairs(retained, configured, crossAvailable)
	resolutions := []float64{.25, .4, .6, .8, 1, 1.25, 1.5, 2, 3, 4}
	fine, quality, err := SelectFinePartition(layers, weights, resolutions, PublicationSeed)
	if err != nil {
		return fail("fine_partition", err)
	}
	communityIDs := stableCommunityIDs(ctx, tx, fine.Communities)
	macro, err := selectMacroPartition(retained, fine.Communities, communityIDs)
	if err != nil {
		return fail("macro_partition", err)
	}
	macroIdentities := stableMacroIdentities(ctx, tx, fine.Communities, communityIDs, macro)
	if err = persistCommunities(ctx, tx, buildID, retained, fine, quality, communityIDs, macro, macroIdentities); err != nil {
		return fail("community_presentation", err)
	}
	if err = recordIdentityEvents(ctx, tx, buildID); err != nil {
		return fail("identity_events", err)
	}
	effective := map[string]float64{}
	for index, layer := range layers {
		effective[string(layer.Name)] = weights[index]
	}
	effectiveJSON, _ := json.Marshal(effective)
	validation, _ := json.Marshal(map[string]any{"valid": true, "edge_count": len(retained), "fine_resolution": fine.Resolution, "fine_modularity": fine.Modularity, "fine_quality": quality, "macro_group_count": len(macro.groups), "macro_resolution": macro.resolution, "current_pointers_unchanged": true})
	if _, err = tx.ExecContext(ctx, `UPDATE affinity_builds SET status='validated',effective_weights=$2::jsonb,validation_result=$3::jsonb,completed_at=now() WHERE id=$1`, buildID, string(effectiveJSON), string(validation)); err != nil {
		return fail("validate", err)
	}
	if err = tx.Commit(); err != nil {
		return fail("commit", err)
	}
	return buildID, nil
}

func recordIdentityEvents(ctx context.Context, tx *sql.Tx, buildID int64) error {
	_, err := tx.ExecContext(ctx, `WITH old_members AS (
 SELECT substring(m.node_id from 11)::bigint subreddit_id,m.community_id old_id
 FROM graph_revision_current current JOIN graph_revision_community_members m ON m.revision_id=current.revision_id
 WHERE current.singleton AND m.node_id LIKE 'subreddit_%'
), overlap AS (
 SELECT old.old_id,new.community_id new_id,count(*) weight FROM old_members old
 JOIN affinity_build_community_members new USING(subreddit_id) WHERE new.build_id=$1 GROUP BY old.old_id,new.community_id
), merges AS (
 SELECT array_agg(old_id ORDER BY old_id) old_ids,ARRAY[new_id] new_ids,jsonb_build_object('overlap_members',sum(weight)) evidence
 FROM overlap GROUP BY new_id HAVING count(*)>1
), splits AS (
 SELECT ARRAY[old_id] old_ids,array_agg(new_id ORDER BY new_id) new_ids,jsonb_build_object('overlap_members',sum(weight)) evidence
 FROM overlap GROUP BY old_id HAVING count(*)>1
), news AS (
 SELECT '{}'::text[] old_ids,ARRAY[new.community_id] new_ids,jsonb_build_object('member_count',count(*)) evidence
 FROM affinity_build_community_members new LEFT JOIN overlap o ON o.new_id=new.community_id
 WHERE new.build_id=$1 AND o.new_id IS NULL GROUP BY new.community_id
), retired AS (
 SELECT ARRAY[old.old_id] old_ids,'{}'::text[] new_ids,jsonb_build_object('member_count',count(*)) evidence
 FROM old_members old LEFT JOIN overlap o ON o.old_id=old.old_id WHERE o.old_id IS NULL GROUP BY old.old_id
), events AS (
 SELECT 'merge' event_type,* FROM merges UNION ALL SELECT 'split',* FROM splits
 UNION ALL SELECT 'new',* FROM news UNION ALL SELECT 'retired',* FROM retired
) INSERT INTO affinity_build_identity_events(build_id,layer,event_type,old_ids,new_ids,evidence)
SELECT $1,'fine',event_type,old_ids,new_ids,evidence FROM events`, buildID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `WITH old_members AS (
 SELECT substring(m.node_id from 11)::bigint subreddit_id,c.macro_group_id old_id
 FROM graph_revision_current current JOIN graph_revision_community_members m ON m.revision_id=current.revision_id
 JOIN graph_revision_communities c ON c.revision_id=m.revision_id AND c.community_id=m.community_id AND c.level=0
 WHERE current.singleton AND m.node_id LIKE 'subreddit_%' AND c.macro_group_id IS NOT NULL
), new_members AS (
 SELECT m.subreddit_id,c.macro_group_id new_id FROM affinity_build_community_members m
 JOIN affinity_build_communities c ON c.build_id=m.build_id AND c.community_id=m.community_id WHERE m.build_id=$1
), overlap AS (
 SELECT old.old_id,new.new_id,count(*) weight FROM old_members old JOIN new_members new USING(subreddit_id) GROUP BY old.old_id,new.new_id
), merges AS (SELECT array_agg(old_id ORDER BY old_id) old_ids,ARRAY[new_id] new_ids,jsonb_build_object('overlap_members',sum(weight)) evidence FROM overlap GROUP BY new_id HAVING count(*)>1),
splits AS (SELECT ARRAY[old_id] old_ids,array_agg(new_id ORDER BY new_id) new_ids,jsonb_build_object('overlap_members',sum(weight)) evidence FROM overlap GROUP BY old_id HAVING count(*)>1),
news AS (SELECT '{}'::text[] old_ids,ARRAY[g.macro_group_id] new_ids,'{}'::jsonb evidence FROM affinity_build_macro_groups g LEFT JOIN overlap o ON o.new_id=g.macro_group_id WHERE g.build_id=$1 AND o.new_id IS NULL),
retired AS (SELECT ARRAY[old_id] old_ids,'{}'::text[] new_ids,'{}'::jsonb evidence FROM (SELECT DISTINCT old_id FROM old_members) old LEFT JOIN overlap o USING(old_id) WHERE o.old_id IS NULL),
events AS (SELECT 'merge' event_type,* FROM merges UNION ALL SELECT 'split',* FROM splits UNION ALL SELECT 'new',* FROM news UNION ALL SELECT 'retired',* FROM retired)
INSERT INTO affinity_build_identity_events(build_id,layer,event_type,old_ids,new_ids,evidence) SELECT $1,'macro',event_type,old_ids,new_ids,evidence FROM events`, buildID)
	return err
}

func retainTopEdges(pairs []stagedPair, perNode, cap int) []stagedPair {
	byNode := map[int64][]int{}
	for index, pair := range pairs {
		byNode[pair.a] = append(byNode[pair.a], index)
		byNode[pair.b] = append(byNode[pair.b], index)
	}
	kept := map[int]bool{}
	for node, indexes := range byNode {
		sort.Slice(indexes, func(i, j int) bool {
			a, b := pairs[indexes[i]], pairs[indexes[j]]
			if a.score.Affinity != b.score.Affinity {
				return a.score.Affinity > b.score.Affinity
			}
			otherA := a.a
			if otherA == node {
				otherA = a.b
			}
			otherB := b.a
			if otherB == node {
				otherB = b.b
			}
			return otherA < otherB
		})
		if len(indexes) > perNode {
			indexes = indexes[:perNode]
		}
		for rank, index := range indexes {
			kept[index] = true
			if pairs[index].a == node {
				pairs[index].rankA = rank + 1
			} else {
				pairs[index].rankB = rank + 1
			}
		}
	}
	out := make([]stagedPair, 0, len(kept))
	for index := range kept {
		out = append(out, pairs[index])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score.Affinity != out[j].score.Affinity {
			return out[i].score.Affinity > out[j].score.Affinity
		}
		if out[i].a != out[j].a {
			return out[i].a < out[j].a
		}
		return out[i].b < out[j].b
	})
	if len(out) > cap {
		out = out[:cap]
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].a != out[j].a {
			return out[i].a < out[j].a
		}
		return out[i].b < out[j].b
	})
	return out
}

func layersFromPairs(pairs []stagedPair, configured Weights, crossAvailable bool) ([]GraphLayer, []float64) {
	signals := []Signal{ActiveUsers, Authors, RepeatCoactivity, CrossReferences}
	layers := []GraphLayer{}
	weights := []float64{}
	var total float64
	for _, signal := range signals {
		if signal == CrossReferences && !crossAvailable {
			continue
		}
		if configured[signal] <= 0 {
			continue
		}
		layer := GraphLayer{Name: signal}
		for _, pair := range pairs {
			if component, ok := pair.score.Components[signal]; ok && component.Raw > 0 {
				layer.Edges = append(layer.Edges, WeightedEdge{A: pair.a, B: pair.b, Weight: component.Raw})
			}
		}
		if len(layer.Edges) > 0 {
			layers = append(layers, layer)
			weights = append(weights, configured[signal])
			total += configured[signal]
		}
	}
	for index := range weights {
		weights[index] /= total
	}
	return layers, weights
}

type macroPartition struct {
	groups                 [][]int64
	communityToMacro       map[int]int
	resolution, modularity float64
}

func selectMacroPartition(pairs []stagedPair, communities [][]int64, ids []string) (macroPartition, error) {
	nodeCommunity := map[int64]int{}
	for index, members := range communities {
		for _, member := range members {
			nodeCommunity[member] = index
		}
	}
	weights := map[[2]int]float64{}
	for _, pair := range pairs {
		a, b := nodeCommunity[pair.a], nodeCommunity[pair.b]
		if a == b {
			continue
		}
		if a > b {
			a, b = b, a
		}
		weights[[2]int{a, b}] += pair.score.Affinity
	}
	layer := GraphLayer{Name: ActiveUsers}
	for key, weight := range weights {
		layer.Edges = append(layer.Edges, WeightedEdge{A: int64(key[0] + 1), B: int64(key[1] + 1), Weight: weight})
	}
	best := macroPartition{}
	for _, resolution := range []float64{.05, .1, .2, .35, .5, .75, 1, 1.5, 2, 3} {
		partition, err := PartitionMultiplex([]GraphLayer{layer}, []float64{1}, resolution, PublicationSeed+1)
		if err != nil {
			return macroPartition{}, err
		}
		if len(partition.Communities) >= 8 && len(partition.Communities) <= 12 && (len(best.groups) == 0 || partition.Modularity > best.modularity) {
			best.groups = partition.Communities
			best.resolution = resolution
			best.modularity = partition.Modularity
		}
	}
	if len(best.groups) == 0 {
		return macroPartition{}, errors.New("no macro resolution produced 8-12 groups")
	}
	best.communityToMacro = map[int]int{}
	for macro, members := range best.groups {
		for _, member := range members {
			best.communityToMacro[int(member)-1] = macro
		}
	}
	return best, nil
}

func stableCommunityIDs(ctx context.Context, tx *sql.Tx, communities [][]int64) []string {
	oldMembership := map[int64]string{}
	memberWeight := map[int64]float64{}
	rows, err := tx.QueryContext(ctx, `SELECT substring(m.node_id from 11)::bigint,m.community_id,1+ln(1+GREATEST(COALESCE(s.subscribers,0),0))
FROM graph_revision_current current JOIN graph_revision_community_members m ON m.revision_id=current.revision_id
LEFT JOIN subreddits s ON s.id=substring(m.node_id from 11)::bigint
WHERE current.singleton AND m.node_id LIKE 'subreddit_%'`)
	if err == nil {
		for rows.Next() {
			var id int64
			var community string
			var weight float64
			if rows.Scan(&id, &community, &weight) == nil {
				oldMembership[id] = community
				memberWeight[id] = weight
			}
		}
		rows.Close()
	}
	newBest := make([]string, len(communities))
	newBestCount := make([]float64, len(communities))
	oldBest := map[string]int{}
	oldBestCount := map[string]float64{}
	for index, members := range communities {
		counts := map[string]float64{}
		for _, member := range members {
			if old := oldMembership[member]; old != "" {
				counts[old] += math.Max(1, memberWeight[member])
			}
		}
		for old, count := range counts {
			if count > newBestCount[index] || (count == newBestCount[index] && old < newBest[index]) {
				newBest[index], newBestCount[index] = old, count
			}
			if count > oldBestCount[old] {
				oldBest[old], oldBestCount[old] = index, count
			}
		}
	}
	ids := make([]string, len(communities))
	for index, members := range communities {
		if old := newBest[index]; old != "" && oldBest[old] == index {
			ids[index] = old
		} else {
			body := fmt.Sprint(members)
			ids[index] = fmt.Sprintf("c:new:%x", sha256Bytes(body)[:8])
		}
	}
	return ids
}

type macroIdentity struct {
	ID          string
	PaletteRole int
	Color       string
}

// stableMacroIdentities preserves both semantic identity and color by
// mutual-best weighted overlap of the fine communities contained in each
// macro-group. Unmatched groups receive content-addressed IDs and the first
// available palette role.
func stableMacroIdentities(ctx context.Context, tx *sql.Tx, fine [][]int64, fineIDs []string, macro macroPartition) []macroIdentity {
	type prior struct {
		macroID, color string
		paletteRole    int
	}
	priorByFine := map[string]prior{}
	rows, err := tx.QueryContext(ctx, `SELECT c.community_id,c.macro_group_id,g.palette_role,g.primary_color
FROM graph_revision_current current JOIN graph_revision_communities c ON c.revision_id=current.revision_id AND c.level=0
JOIN graph_revision_macro_groups g ON g.revision_id=c.revision_id AND g.macro_group_id=c.macro_group_id WHERE current.singleton`)
	if err == nil {
		for rows.Next() {
			var fineID string
			var item prior
			if rows.Scan(&fineID, &item.macroID, &item.paletteRole, &item.color) == nil {
				priorByFine[fineID] = item
			}
		}
		rows.Close()
	}
	newBest := make([]string, len(macro.groups))
	newWeight := make([]float64, len(macro.groups))
	oldBest := map[string]int{}
	oldWeight := map[string]float64{}
	priorByID := map[string]prior{}
	for macroIndex, memberCommunities := range macro.groups {
		weights := map[string]float64{}
		for _, node := range memberCommunities {
			fineIndex := int(node) - 1
			if fineIndex < 0 || fineIndex >= len(fineIDs) {
				continue
			}
			if old, ok := priorByFine[fineIDs[fineIndex]]; ok {
				weight := float64(len(fine[fineIndex]))
				weights[old.macroID] += weight
				priorByID[old.macroID] = old
			}
		}
		for oldID, weight := range weights {
			if weight > newWeight[macroIndex] || (weight == newWeight[macroIndex] && oldID < newBest[macroIndex]) {
				newBest[macroIndex], newWeight[macroIndex] = oldID, weight
			}
			if weight > oldWeight[oldID] || (weight == oldWeight[oldID] && macroIndex < oldBest[oldID]) {
				oldBest[oldID], oldWeight[oldID] = macroIndex, weight
			}
		}
	}
	palette := presentation.MacroPalette()
	usedRoles := map[int]bool{}
	result := make([]macroIdentity, len(macro.groups))
	for index, members := range macro.groups {
		if oldID := newBest[index]; oldID != "" && oldBest[oldID] == index {
			old := priorByID[oldID]
			result[index] = macroIdentity{ID: oldID, PaletteRole: old.paletteRole, Color: old.color}
			usedRoles[old.paletteRole] = true
			continue
		}
		memberIDs := make([]string, 0, len(members))
		for _, node := range members {
			fineIndex := int(node) - 1
			if fineIndex >= 0 && fineIndex < len(fineIDs) {
				memberIDs = append(memberIDs, fineIDs[fineIndex])
			}
		}
		sort.Strings(memberIDs)
		result[index].ID = fmt.Sprintf("m:new:%x", sha256Bytes(strings.Join(memberIDs, "\x00"))[:8])
	}
	for index := range result {
		if result[index].Color != "" {
			continue
		}
		role := 0
		for usedRoles[role] {
			role++
		}
		role %= len(palette)
		usedRoles[role] = true
		result[index].PaletteRole = role
		result[index].Color = palette[role]
	}
	return result
}

func sha256Bytes(value string) []byte { sum := sha256Sum([]byte(value)); return sum[:] }
func sha256Sum(value []byte) [32]byte { return sha256.Sum256(value) }

func persistCommunities(ctx context.Context, tx *sql.Tx, buildID int64, pairs []stagedPair, fine Partition, quality FinePartitionQuality, ids []string, macro macroPartition, macroIdentities []macroIdentity) error {
	nodeStrength := map[int64]float64{}
	for _, pair := range pairs {
		nodeStrength[pair.a] += pair.score.Affinity
		nodeStrength[pair.b] += pair.score.Affinity
	}
	metadata := map[int64]subredditMetadata{}
	documentFrequency := map[string]int{}
	rows, err := tx.QueryContext(ctx, `SELECT id,name,COALESCE(title,''),COALESCE(description,'') FROM subreddits`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var item subredditMetadata
		if err := rows.Scan(&id, &item.name, &item.title, &item.description); err != nil {
			return err
		}
		metadata[id] = item
		seen := map[string]bool{}
		for _, token := range topicTokens(item.name + " " + item.title + " " + item.description) {
			seen[token] = true
		}
		for token := range seen {
			documentFrequency[token]++
		}
	}
	rows.Close()
	labelConfig := clusterlabel.Config{BaseURL: os.Getenv("CLUSTER_LABEL_BASE_URL"), APIKey: os.Getenv("CLUSTER_LABEL_API_KEY"), Model: os.Getenv("CLUSTER_LABEL_MODEL")}
	clusterOfNode := map[int64]int{}
	for cluster, members := range fine.Communities {
		for _, member := range members {
			clusterOfNode[member] = cluster
		}
	}
	macroAffinity := map[int]map[int]float64{}
	for cluster := range fine.Communities {
		macroAffinity[cluster] = map[int]float64{}
	}
	for _, pair := range pairs {
		ca, cb := clusterOfNode[pair.a], clusterOfNode[pair.b]
		ma, mb := macro.communityToMacro[ca], macro.communityToMacro[cb]
		if ca == cb {
			macroAffinity[ca][ma] += pair.score.Affinity
			continue
		}
		macroAffinity[ca][mb] += pair.score.Affinity
		macroAffinity[cb][ma] += pair.score.Affinity
	}
	spectralPosition := localSpectralPositions(len(fine.Communities), pairs, clusterOfNode, macro.communityToMacro)
	macroLabels := make([]clusterlabel.Result, len(macro.groups))
	usedNames := map[string]bool{}
	for index, members := range macro.groups {
		representatives := []string{}
		for _, communityNode := range members {
			communityIndex := int(communityNode) - 1
			if communityIndex >= 0 && communityIndex < len(fine.Communities) {
				for _, node := range topRepresentativeMembers(fine.Communities[communityIndex], nodeStrength, metadata, 2) {
					representatives = append(representatives, metadata[node].name)
				}
			}
		}
		labelEvidence := clusterlabel.Evidence{Representatives: representatives}
		label := uniqueLabel(generateCachedLabel(ctx, tx, labelEvidence, labelConfig), labelEvidence, usedNames)
		macroLabels[index] = label
		metrics, _ := json.Marshal(map[string]any{"community_count": len(members), "resolution": macro.resolution, "representative_subreddits": representatives})
		identity := macroIdentities[index]
		if _, err := tx.ExecContext(ctx, `INSERT INTO affinity_build_macro_groups(build_id,macro_group_id,display_name,evidence_label,palette_role,primary_color,evidence_metrics) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`, buildID, identity.ID, label.DisplayName, label.EvidenceLabel, identity.PaletteRole, identity.Color, string(metrics)); err != nil {
			return err
		}
	}
	for index, members := range fine.Communities {
		macroIndex := macro.communityToMacro[index]
		representativeIDs := topRepresentativeMembers(members, nodeStrength, metadata, 12)
		representatives := make([]string, 0, len(representativeIDs))
		for _, id := range representativeIDs {
			representatives = append(representatives, metadata[id].name)
		}
		topics := tfidfTopics(members, metadata, documentFrequency, len(metadata), 12)
		labelEvidence := clusterlabel.Evidence{Representatives: representatives, Topics: topics, NeighborMacroGroups: []string{macroLabels[macroIndex].DisplayName}, Metrics: map[string]float64{"size": float64(len(members))}}
		label := uniqueLabel(generateCachedLabel(ctx, tx, labelEvidence, labelConfig), labelEvidence, usedNames)
		distinctiveness := math.Min(1, averageStrength(members, nodeStrength)/(1+averageStrength(members, nodeStrength)))
		confidence := math.Min(1, float64(len(members))/20)
		var totalAffinity, primaryAffinity, secondaryAffinity float64
		secondaryMacro := -1
		for target, value := range macroAffinity[index] {
			totalAffinity += value
			if target == macroIndex {
				primaryAffinity = value
			} else if value > secondaryAffinity {
				secondaryAffinity, secondaryMacro = value, target
			}
		}
		if totalAffinity > 0 {
			primaryAffinity /= totalAffinity
			secondaryAffinity /= totalAffinity
		}
		bridge := presentation.IsBridge(primaryAffinity, secondaryAffinity)
		secondaryColor := ""
		if bridge && secondaryMacro >= 0 {
			secondaryColor = macroIdentities[secondaryMacro].Color
		}
		metrics, _ := json.Marshal(map[string]any{"size": len(members), "fine_resolution": fine.Resolution, "fine_modularity": fine.Modularity, "quality": quality, "tfidf_topics": topics, "primary_macro_affinity": primaryAffinity, "bridge_affinity": secondaryAffinity, "local_spectral_position": spectralPosition[index]})
		representativeJSON, _ := json.Marshal(representatives)
		macroID := macroIdentities[macroIndex].ID
		primaryColor := presentation.ClusterShade(macroIdentities[macroIndex].Color, spectralPosition[index], (distinctiveness+confidence)/2)
		if _, err := tx.ExecContext(ctx, `INSERT INTO affinity_build_communities(build_id,community_id,macro_group_id,display_name,evidence_label,primary_color,secondary_color,is_bridge,distinctiveness,affinity_confidence,naming_method,naming_version,naming_confidence,representative_members,evidence_metrics) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14::jsonb,$15::jsonb)`, buildID, ids[index], macroID, label.DisplayName, label.EvidenceLabel, primaryColor, secondaryColor, bridge, distinctiveness, confidence, label.Method, label.MethodVersion, label.Confidence, string(representativeJSON), string(metrics)); err != nil {
			return err
		}
		for _, member := range members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO affinity_build_community_members(build_id,community_id,subreddit_id) VALUES($1,$2,$3)`, buildID, ids[index], member); err != nil {
				return err
			}
		}
	}
	return nil
}

// localSpectralPositions approximates the first non-constant eigenvector of
// each macro-group's sparse normalized fine-community adjacency. The scalar is
// used only for shade within that macro-group.
func localSpectralPositions(count int, pairs []stagedPair, clusterOfNode map[int64]int, communityToMacro map[int]int) []float64 {
	adjacency := make([]map[int]float64, count)
	degree := make([]float64, count)
	for index := range adjacency {
		adjacency[index] = map[int]float64{}
	}
	for _, pair := range pairs {
		a, b := clusterOfNode[pair.a], clusterOfNode[pair.b]
		if a == b || communityToMacro[a] != communityToMacro[b] {
			continue
		}
		adjacency[a][b] += pair.score.Affinity
		adjacency[b][a] += pair.score.Affinity
		degree[a] += pair.score.Affinity
		degree[b] += pair.score.Affinity
	}
	result := make([]float64, count)
	groups := map[int][]int{}
	for community := 0; community < count; community++ {
		groups[communityToMacro[community]] = append(groups[communityToMacro[community]], community)
	}
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		vector := make([]float64, count)
		for _, node := range members {
			vector[node] = float64((node*1103515245+12345)%2001)/1000 - 1
		}
		for iteration := 0; iteration < 48; iteration++ {
			next := make([]float64, count)
			for _, node := range members {
				for neighbor, weight := range adjacency[node] {
					if degree[node] > 0 && degree[neighbor] > 0 {
						next[node] += weight * vector[neighbor] / math.Sqrt(degree[node]*degree[neighbor])
					}
				}
			}
			mean := 0.0
			for _, node := range members {
				mean += next[node]
			}
			mean /= float64(len(members))
			norm := 0.0
			for _, node := range members {
				next[node] -= mean
				norm += next[node] * next[node]
			}
			norm = math.Sqrt(norm)
			if norm == 0 {
				break
			}
			for _, node := range members {
				vector[node] = next[node] / norm
			}
		}
		maxAbs := 0.0
		for _, node := range members {
			maxAbs = math.Max(maxAbs, math.Abs(vector[node]))
		}
		if maxAbs > 0 {
			// Fix eigenvector sign deterministically at the lowest community index.
			sign := 1.0
			if vector[members[0]] < 0 {
				sign = -1
			}
			for _, node := range members {
				result[node] = sign * vector[node] / maxAbs
			}
		}
	}
	return result
}

type subredditMetadata struct{ name, title, description string }

func topRepresentativeMembers(members []int64, strength map[int64]float64, metadata map[int64]subredditMetadata, limit int) []int64 {
	out := append([]int64(nil), members...)
	maxStrength := 0.0
	for _, member := range members {
		if strength[member] > maxStrength {
			maxStrength = strength[member]
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := metadata[out[i]], metadata[out[j]]
		leftScore := RepresentativeScore(left.name, strength[out[i]]/math.Max(1, maxStrength), .5, .7, genericness(left.name))
		rightScore := RepresentativeScore(right.name, strength[out[j]]/math.Max(1, maxStrength), .5, .7, genericness(right.name))
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return out[i] < out[j]
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
func genericness(name string) float64 {
	lower := strings.ToLower(name)
	for _, term := range []string{"funny", "pics", "videos", "news", "askreddit", "bestof"} {
		if lower == term {
			return 1
		}
	}
	return 0
}

var topicStopwords = map[string]bool{"the": true, "and": true, "for": true, "with": true, "this": true, "that": true, "from": true, "your": true, "you": true, "are": true, "subreddit": true, "community": true, "reddit": true, "all": true, "about": true, "into": true}

func topicTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') })
	out := []string{}
	for _, field := range fields {
		if len(field) >= 3 && !topicStopwords[field] {
			out = append(out, field)
		}
	}
	return out
}
func tfidfTopics(members []int64, metadata map[int64]subredditMetadata, documentFrequency map[string]int, documentCount, limit int) []string {
	frequency := map[string]int{}
	for _, member := range members {
		item := metadata[member]
		for _, token := range topicTokens(item.name + " " + item.title + " " + item.description) {
			frequency[token]++
		}
	}
	type scoredTopic struct {
		token string
		score float64
	}
	scored := make([]scoredTopic, 0, len(frequency))
	for token, count := range frequency {
		df := documentFrequency[token]
		score := float64(count) * math.Log((1+float64(documentCount))/(1+float64(df)))
		scored = append(scored, scoredTopic{token, score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].token < scored[j].token
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]string, len(scored))
	for index, item := range scored {
		out[index] = item.token
	}
	return out
}

func generateCachedLabel(ctx context.Context, tx *sql.Tx, evidence clusterlabel.Evidence, config clusterlabel.Config) clusterlabel.Result {
	fingerprint := clusterlabel.Fingerprint(evidence)
	model := config.Model
	if model == "" {
		model = "none"
	}
	var result clusterlabel.Result
	err := tx.QueryRowContext(ctx, `SELECT display_name,evidence_label,confidence,method,prompt_version,COALESCE(grounding::text,'') FROM cluster_label_cache WHERE evidence_fingerprint=$1 AND prompt_version=$2 AND provider_model=$3 AND policy_version=$4`, fingerprint, clusterlabel.PromptVersion, model, clusterlabel.PolicyVersion).Scan(&result.DisplayName, &result.EvidenceLabel, &result.Confidence, &result.Method, &result.MethodVersion, &result.Grounding)
	if err == nil {
		return result
	}
	result, _ = clusterlabel.Generate(ctx, evidence, config)
	groundingJSON, _ := json.Marshal(map[string]any{"evidence": result.Grounding})
	sum := sha256.Sum256([]byte(result.DisplayName + "\x00" + result.EvidenceLabel + "\x00" + result.Method))
	_, _ = tx.ExecContext(ctx, `INSERT INTO cluster_label_cache(evidence_fingerprint,prompt_version,provider_model,policy_version,display_name,evidence_label,confidence,method,grounding,result_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10) ON CONFLICT DO NOTHING`, fingerprint, clusterlabel.PromptVersion, model, clusterlabel.PolicyVersion, result.DisplayName, result.EvidenceLabel, result.Confidence, result.Method, string(groundingJSON), fmt.Sprintf("%x", sum))
	return result
}

func uniqueLabel(result clusterlabel.Result, evidence clusterlabel.Evidence, used map[string]bool) clusterlabel.Result {
	key := strings.ToLower(result.DisplayName)
	if !used[key] {
		used[key] = true
		return result
	}
	fallback, _ := clusterlabel.Generate(context.Background(), evidence, clusterlabel.Config{})
	candidate := fallback.DisplayName
	if used[strings.ToLower(candidate)] && len(evidence.Representatives) > 0 {
		words := strings.Fields(candidate)
		if len(words) >= 5 {
			words = words[:4]
		}
		candidate = strings.Join(append(words, evidence.Representatives[0]), " ")
	}
	fallback.DisplayName = candidate
	used[strings.ToLower(candidate)] = true
	return fallback
}

func averageStrength(members []int64, strength map[int64]float64) float64 {
	if len(members) == 0 {
		return 0
	}
	var total float64
	for _, member := range members {
		total += strength[member]
	}
	return total / float64(len(members))
}
