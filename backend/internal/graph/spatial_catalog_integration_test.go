package graph

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestIntegration_FullSpatialCatalogCoversLabelsLinksAndWarmStarts(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	requireIsolatedSpatialCatalogState(t, ctx, database)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var catalogIDs []int64
	t.Cleanup(func() {
		cleanupSpatialCatalogs(t, ctx, database, catalogIDs)
	})
	var subredditA, subredditB, userID int32
	if err := database.QueryRowContext(ctx, `INSERT INTO subreddits(name,title,subscribers) VALUES($1,'Alpha territory',100) RETURNING id`, "catalog_alpha_"+suffix).Scan(&subredditA); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO subreddits(name,title,subscribers) VALUES($1,'Beta territory',50) RETURNING id`, "catalog_beta_"+suffix).Scan(&subredditB); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO users(username) VALUES($1) RETURNING id`, "catalog_user_"+suffix).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	postID := "catalog_post_" + suffix
	commentID := "catalog_comment_" + suffix
	if _, err := database.ExecContext(ctx, `INSERT INTO posts(id,subreddit_id,author_id,title,score) VALUES($1,$2,$3,'A semantic post label',42)`, postID, subredditA, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO comments(id,post_id,author_id,subreddit_id,parent_id,body,score,depth) VALUES($1,$2,$3,$4,'t3_'||$2,'A semantic comment label',7,1)`, commentID, postID, userID, subredditA); err != nil {
		t.Fatal(err)
	}
	// Projection rows are rebuilt after the fixed source watermark. They must be
	// selected through their watermark-pinned entity endpoints, not their own
	// later materialization timestamp.
	watermark := time.Now().UTC()
	if _, err := database.ExecContext(ctx, `INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count) VALUES($1,$2,9)
ON CONFLICT(user_id,subreddit_id) DO UPDATE SET activity_count=EXCLUDED.activity_count,updated_at=now()`, userID, subredditA); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count) VALUES($1,$2,4)
ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count,updated_at=now()`, subredditA, subredditB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM comments WHERE id=$1`, commentID)
		_, _ = database.Exec(`DELETE FROM posts WHERE id=$1`, postID)
		_, _ = database.Exec(`DELETE FROM user_subreddit_activity WHERE user_id=$1`, userID)
		_, _ = database.Exec(`DELETE FROM subreddit_relationships WHERE source_subreddit_id IN ($1,$2) OR target_subreddit_id IN ($1,$2)`, subredditA, subredditB)
		_, _ = database.Exec(`DELETE FROM users WHERE id=$1`, userID)
		_, _ = database.Exec(`DELETE FROM subreddits WHERE id IN ($1,$2)`, subredditA, subredditB)
	})

	first, err := BuildSpatialCatalogWithOptions(ctx, database, watermark, SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	catalogIDs = append(catalogIDs, first)
	var sourceCount, catalogCount int64
	if err := database.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM subreddits WHERE updated_at <= $1) +
 (SELECT count(*) FROM users WHERE updated_at <= $1) +
 (SELECT count(*) FROM posts WHERE updated_at <= $1) +
 (SELECT count(*) FROM comments WHERE updated_at <= $1)`, watermark).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT entity_count FROM spatial_catalogs WHERE id=$1`, first).Scan(&catalogCount); err != nil {
		t.Fatal(err)
	}
	if catalogCount != sourceCount {
		t.Fatalf("catalog entities=%d source entities=%d", catalogCount, sourceCount)
	}
	var label, provenance string
	var authorID, anchorID sql.NullString
	var x1, y1, z1 float64
	if err := database.QueryRowContext(ctx, `SELECT label,position_provenance,author_id,anchor_id,x,y,z FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, first, "comment_"+commentID).Scan(&label, &provenance, &authorID, &anchorID, &x1, &y1, &z1); err != nil {
		t.Fatal(err)
	}
	if label != "A semantic comment label" || provenance != "post-seeded" || authorID.String != "user_"+fmt.Sprint(userID) || anchorID.String != "post_"+postID {
		t.Fatalf("comment label/provenance/author/anchor = %q/%q/%q/%q", label, provenance, authorID.String, anchorID.String)
	}
	var semanticLinks int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM spatial_catalog_links
WHERE catalog_id=$1 AND (
  (source=$2 AND target=$3 AND relation='user_activity') OR
  (source=$3 AND target=$4 AND relation='subreddit_overlap')
)`, first, "user_"+fmt.Sprint(userID), "subreddit_"+fmt.Sprint(subredditA), "subreddit_"+fmt.Sprint(subredditB)).Scan(&semanticLinks); err != nil {
		t.Fatal(err)
	}
	if semanticLinks != 2 {
		t.Fatalf("materialized cross-cutting links=%d, want 2", semanticLinks)
	}
	var orphanLinks int
	if err := database.QueryRowContext(ctx, `SELECT count(*)
FROM spatial_catalog_links link
LEFT JOIN spatial_catalog_entities source
  ON source.catalog_id=link.catalog_id AND source.id=link.source
LEFT JOIN spatial_catalog_entities target
  ON target.catalog_id=link.catalog_id AND target.id=link.target
WHERE link.catalog_id=$1 AND (source.id IS NULL OR target.id IS NULL)`, first).Scan(&orphanLinks); err != nil {
		t.Fatal(err)
	}
	if orphanLinks != 0 {
		t.Fatalf("endpoint-constrained projection produced %d orphan links", orphanLinks)
	}

	second, err := BuildSpatialCatalogWithOptions(ctx, database, watermark.Add(time.Second), SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	catalogIDs = append(catalogIDs, second)
	var x2, y2, z2 float64
	if err := database.QueryRowContext(ctx, `SELECT x,y,z,position_provenance FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, second, "comment_"+commentID).Scan(&x2, &y2, &z2, &provenance); err != nil {
		t.Fatal(err)
	}
	if x1 != x2 || y1 != y2 || z1 != z2 || provenance != "warm-start" {
		t.Fatalf("warm start changed position: first=(%g,%g,%g) second=(%g,%g,%g) provenance=%s", x1, y1, z1, x2, y2, z2, provenance)
	}
	var current int64
	if err := database.QueryRowContext(ctx, `SELECT catalog_id FROM spatial_catalog_current WHERE singleton`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != second {
		t.Fatalf("current catalog=%d want %d", current, second)
	}
	var projectionTable sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT to_regclass('public.catalog_subreddit_overlap_projection')::text`).Scan(&projectionTable); err != nil {
		t.Fatal(err)
	}
	if projectionTable.Valid {
		t.Fatalf("overlap projection table leaked after publication: %s", projectionTable.String)
	}
}

func TestIntegration_SpatialCatalogExcludesChildrenOfSubredditAfterWatermark(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	requireIsolatedSpatialCatalogState(t, ctx, database)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var catalogIDs []int64
	t.Cleanup(func() {
		cleanupSpatialCatalogs(t, ctx, database, catalogIDs)
	})
	watermark := time.Now().UTC()
	beforeWatermark := watermark.Add(-time.Hour)
	afterWatermark := watermark.Add(time.Hour)
	var eligibleSubreddit, anchorSubreddit, ineligibleSubreddit, userID int32
	for _, subreddit := range []struct {
		name      string
		updatedAt time.Time
		id        *int32
	}{
		{"catalog_eligible_" + suffix, beforeWatermark, &eligibleSubreddit},
		{"catalog_anchor_" + suffix, beforeWatermark, &anchorSubreddit},
		{"catalog_ineligible_" + suffix, afterWatermark, &ineligibleSubreddit},
	} {
		if err := database.QueryRowContext(ctx, `INSERT INTO subreddits(name,title,subscribers,created_at,updated_at)
VALUES($1,'Catalog eligibility',1,$2,$2) RETURNING id`, subreddit.name, subreddit.updatedAt).Scan(subreddit.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO users(username,created_at,updated_at) VALUES($1,$2,$2) RETURNING id`, "catalog_eligibility_user_"+suffix, beforeWatermark).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	eligiblePostID := "catalog_eligible_post_" + suffix
	eligibleCommentID := "catalog_eligible_comment_" + suffix
	ineligiblePostID := "catalog_ineligible_post_" + suffix
	ineligibleCommentID := "catalog_ineligible_comment_" + suffix
	for _, post := range []struct {
		id          string
		subredditID int32
	}{
		{eligiblePostID, eligibleSubreddit},
		{ineligiblePostID, ineligibleSubreddit},
	} {
		if _, err := database.ExecContext(ctx, `INSERT INTO posts(id,subreddit_id,author_id,title,created_at,updated_at)
VALUES($1,$2,$3,'Catalog eligibility post',$4,$4)`, post.id, post.subredditID, userID, beforeWatermark); err != nil {
			t.Fatal(err)
		}
	}
	for _, comment := range []struct {
		id          string
		postID      string
		subredditID int32
	}{
		{eligibleCommentID, eligiblePostID, eligibleSubreddit},
		{ineligibleCommentID, ineligiblePostID, ineligibleSubreddit},
	} {
		if _, err := database.ExecContext(ctx, `INSERT INTO comments(id,post_id,author_id,subreddit_id,parent_id,body,created_at,updated_at)
VALUES($1,$2,$3,$4,'t3_'||$2,'Catalog eligibility comment',$5,$5)`, comment.id, comment.postID, userID, comment.subredditID, beforeWatermark); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count)
VALUES($1,$2,1) ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count`, eligibleSubreddit, anchorSubreddit); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM comments WHERE id IN ($1,$2)`, eligibleCommentID, ineligibleCommentID)
		_, _ = database.Exec(`DELETE FROM posts WHERE id IN ($1,$2)`, eligiblePostID, ineligiblePostID)
		_, _ = database.Exec(`DELETE FROM subreddit_relationships WHERE source_subreddit_id IN ($1,$2,$3) OR target_subreddit_id IN ($1,$2,$3)`, eligibleSubreddit, anchorSubreddit, ineligibleSubreddit)
		_, _ = database.Exec(`DELETE FROM users WHERE id=$1`, userID)
		_, _ = database.Exec(`DELETE FROM subreddits WHERE id IN ($1,$2,$3)`, eligibleSubreddit, anchorSubreddit, ineligibleSubreddit)
	})

	catalogID, err := BuildSpatialCatalogWithOptions(ctx, database, watermark, SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	catalogIDs = append(catalogIDs, catalogID)
	for _, entity := range []struct {
		id      string
		present bool
	}{
		{"post_" + eligiblePostID, true},
		{"comment_" + eligibleCommentID, true},
		{"post_" + ineligiblePostID, false},
		{"comment_" + ineligibleCommentID, false},
	} {
		var entities, documents int
		if err := database.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2),
 (SELECT count(*) FROM spatial_catalog_documents WHERE catalog_id=$1 AND entity_id=$2)`, catalogID, entity.id).Scan(&entities, &documents); err != nil {
			t.Fatal(err)
		}
		want := 0
		if entity.present {
			want = 1
		}
		if entities != want || documents != want {
			t.Fatalf("%s entities/documents=%d/%d, want %d/%d", entity.id, entities, documents, want, want)
		}
	}
}

func requireIsolatedSpatialCatalogState(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var catalogs, pointers, revisionRefs int
	if err := database.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM spatial_catalogs),
 (SELECT count(*) FROM spatial_catalog_current),
 (SELECT count(*) FROM graph_revisions WHERE spatial_catalog_id IS NOT NULL)`).Scan(&catalogs, &pointers, &revisionRefs); err != nil {
		t.Fatal(err)
	}
	if catalogs != 0 || pointers != 0 || revisionRefs != 0 {
		t.Fatalf("spatial catalog integration test requires isolated state: catalogs=%d pointers=%d revision_refs=%d", catalogs, pointers, revisionRefs)
	}
}

func cleanupSpatialCatalogs(t *testing.T, ctx context.Context, database *sql.DB, catalogIDs []int64) {
	t.Helper()
	if len(catalogIDs) == 0 {
		return
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM spatial_catalog_current WHERE catalog_id = ANY($1)`, pq.Array(catalogIDs)); err != nil {
		t.Errorf("clear test catalog pointer: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE graph_revisions SET spatial_catalog_id=NULL WHERE spatial_catalog_id = ANY($1)`, pq.Array(catalogIDs)); err != nil {
		t.Errorf("clear test catalog revision references: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM spatial_catalogs WHERE id = ANY($1)`, pq.Array(catalogIDs)); err != nil {
		t.Errorf("delete test catalogs: %v", err)
	}
}
