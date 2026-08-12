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

func TestIntegration_FullSpatialCatalogCoversLabelsLinksAndWarmStarts(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
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
	if _, err := database.ExecContext(ctx, `INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count) VALUES($1,$2,9)
ON CONFLICT(user_id,subreddit_id) DO UPDATE SET activity_count=EXCLUDED.activity_count,updated_at=now()`, userID, subredditA); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count) VALUES($1,$2,4)
ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count,updated_at=now()`, subredditA, subredditB); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = database.Exec(`DELETE FROM spatial_catalog_current`)
		_, _ = database.Exec(`UPDATE graph_revisions SET spatial_catalog_id=NULL`)
		_, _ = database.Exec(`DELETE FROM spatial_catalogs`)
		_, _ = database.Exec(`DELETE FROM comments WHERE id=$1`, commentID)
		_, _ = database.Exec(`DELETE FROM posts WHERE id=$1`, postID)
		_, _ = database.Exec(`DELETE FROM user_subreddit_activity WHERE user_id=$1`, userID)
		_, _ = database.Exec(`DELETE FROM subreddit_relationships WHERE source_subreddit_id IN ($1,$2) OR target_subreddit_id IN ($1,$2)`, subredditA, subredditB)
		_, _ = database.Exec(`DELETE FROM users WHERE id=$1`, userID)
		_, _ = database.Exec(`DELETE FROM subreddits WHERE id IN ($1,$2)`, subredditA, subredditB)
	}()

	watermark := time.Now().UTC().Add(time.Second)
	first, err := BuildSpatialCatalogWithOptions(ctx, database, watermark, SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
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

	second, err := BuildSpatialCatalogWithOptions(ctx, database, watermark.Add(time.Second), SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
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
}
