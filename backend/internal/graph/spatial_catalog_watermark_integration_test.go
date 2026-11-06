package graph

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestIntegration_SpatialCatalogDocumentsMatchWatermarkScopedEntities(t *testing.T) {
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

	var existingCatalogs, existingPointers, revisionRefs int
	if err := database.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM spatial_catalogs),
 (SELECT count(*) FROM spatial_catalog_current),
 (SELECT count(*) FROM graph_revisions WHERE spatial_catalog_id IS NOT NULL)`).Scan(&existingCatalogs, &existingPointers, &revisionRefs); err != nil {
		t.Fatal(err)
	}
	if existingCatalogs != 0 || existingPointers != 0 || revisionRefs != 0 {
		t.Skipf("requires isolated spatial catalog state: catalogs=%d pointers=%d revision_refs=%d", existingCatalogs, existingPointers, revisionRefs)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	watermark := time.Now().UTC()
	before := watermark.Add(-time.Hour)
	after := watermark.Add(time.Hour)
	var eligibleSubreddit, anchorSubreddit, ineligibleSubreddit, userID int32
	for _, subreddit := range []struct {
		name      string
		updatedAt time.Time
		id        *int32
	}{
		{"catalog_eligible_" + suffix, before, &eligibleSubreddit},
		{"catalog_anchor_" + suffix, before, &anchorSubreddit},
		{"catalog_ineligible_" + suffix, after, &ineligibleSubreddit},
	} {
		if err := database.QueryRowContext(ctx, `INSERT INTO subreddits(name,title,subscribers,created_at,updated_at)
VALUES($1,'Catalog eligibility',1,$2,$2) RETURNING id`, subreddit.name, subreddit.updatedAt).Scan(subreddit.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO users(username,created_at,updated_at) VALUES($1,$2,$2) RETURNING id`, "catalog_eligibility_user_"+suffix, before).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	eligiblePostID := "catalog_eligible_post_" + suffix
	ineligiblePostID := "catalog_ineligible_post_" + suffix
	eligibleCommentID := "catalog_eligible_comment_" + suffix
	ineligibleCommentID := "catalog_ineligible_comment_" + suffix
	for _, post := range []struct {
		id          string
		subredditID int32
	}{{eligiblePostID, eligibleSubreddit}, {ineligiblePostID, ineligibleSubreddit}} {
		if _, err := database.ExecContext(ctx, `INSERT INTO posts(id,subreddit_id,author_id,title,created_at,updated_at)
VALUES($1,$2,$3,'Catalog eligibility post',$4,$4)`, post.id, post.subredditID, userID, before); err != nil {
			t.Fatal(err)
		}
	}
	for _, comment := range []struct {
		id, postID string
		subreddit  int32
	}{{eligibleCommentID, eligiblePostID, eligibleSubreddit}, {ineligibleCommentID, ineligiblePostID, ineligibleSubreddit}} {
		if _, err := database.ExecContext(ctx, `INSERT INTO comments(id,post_id,author_id,subreddit_id,parent_id,body,created_at,updated_at)
VALUES($1,$2,$3,$4,'t3_'||$2,'Catalog eligibility comment',$5,$5)`, comment.id, comment.postID, userID, comment.subreddit, before); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count)
VALUES($1,$2,1) ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count`, eligibleSubreddit, anchorSubreddit); err != nil {
		t.Fatal(err)
	}

	var catalogID int64
	t.Cleanup(func() {
		if catalogID != 0 {
			_, _ = database.Exec(`DELETE FROM spatial_catalog_current WHERE catalog_id=$1`, catalogID)
			_, _ = database.Exec(`UPDATE graph_revisions SET spatial_catalog_id=NULL WHERE spatial_catalog_id=$1`, catalogID)
			_, _ = database.Exec(`DELETE FROM spatial_catalogs WHERE id=$1`, catalogID)
		}
		_, _ = database.Exec(`DELETE FROM comments WHERE id IN ($1,$2)`, eligibleCommentID, ineligibleCommentID)
		_, _ = database.Exec(`DELETE FROM posts WHERE id IN ($1,$2)`, eligiblePostID, ineligiblePostID)
		_, _ = database.Exec(`DELETE FROM subreddit_relationships WHERE source_subreddit_id IN ($1,$2,$3) OR target_subreddit_id IN ($1,$2,$3)`, eligibleSubreddit, anchorSubreddit, ineligibleSubreddit)
		_, _ = database.Exec(`DELETE FROM users WHERE id=$1`, userID)
		_, _ = database.Exec(`DELETE FROM subreddits WHERE id IN ($1,$2,$3)`, eligibleSubreddit, anchorSubreddit, ineligibleSubreddit)
	})

	catalogID, err = BuildSpatialCatalogWithOptions(ctx, database, watermark, SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range []struct {
		id      string
		present int
	}{
		{"post_" + eligiblePostID, 1},
		{"comment_" + eligibleCommentID, 1},
		{"post_" + ineligiblePostID, 0},
		{"comment_" + ineligibleCommentID, 0},
	} {
		var entities, documents int
		if err := database.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2),
 (SELECT count(*) FROM spatial_catalog_documents WHERE catalog_id=$1 AND entity_id=$2)`, catalogID, entity.id).Scan(&entities, &documents); err != nil {
			t.Fatal(err)
		}
		if entities != entity.present || documents != entity.present {
			t.Fatalf("%s entity/document coverage=(%d,%d), want (%d,%d)", entity.id, entities, documents, entity.present, entity.present)
		}
	}
	var mismatches int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM spatial_catalog_documents d
LEFT JOIN spatial_catalog_entities e ON e.catalog_id=d.catalog_id AND e.id=d.entity_id AND e.type=d.entity_type
WHERE d.catalog_id=$1 AND e.id IS NULL`, catalogID).Scan(&mismatches); err != nil {
		t.Fatal(err)
	}
	if mismatches != 0 {
		t.Fatalf("catalog has %d document/entity mismatches", mismatches)
	}
}
