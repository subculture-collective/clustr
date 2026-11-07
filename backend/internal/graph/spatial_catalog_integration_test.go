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

	// Start without a current catalog so the first build is a cold layout.
	if _, err := database.ExecContext(ctx, `DELETE FROM spatial_catalog_current`); err != nil {
		t.Fatal(err)
	}
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

	// The first build had no current catalog, so it was a cold force layout:
	// the two linked subreddits are simulated bodies inside the world radius
	// and the user, post and comment follow their anchors. Retention keeps one
	// catalog, so these facts are read before the next build replaces it.
	var firstMode, subredditProvenance string
	var subredditRadius, xa, ya, za float64
	if err := database.QueryRowContext(ctx, `SELECT config->>'placement_mode' FROM spatial_catalogs WHERE id=$1`, first).Scan(&firstMode); err != nil {
		t.Fatal(err)
	}
	if firstMode != "cold" {
		t.Fatalf("first placement mode %q, want cold", firstMode)
	}
	if err := database.QueryRowContext(ctx, `SELECT position_provenance,sqrt(x*x+y*y+z*z),x,y,z FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, first, fmt.Sprintf("subreddit_%d", subredditA)).Scan(&subredditProvenance, &subredditRadius, &xa, &ya, &za); err != nil {
		t.Fatal(err)
	}
	if subredditProvenance != "force-layout" || subredditRadius > defaultCatalogWorldRadius*isolatedShellInner {
		t.Fatalf("linked subreddit placement: provenance=%s radius=%g", subredditProvenance, subredditRadius)
	}
	var postProvenance string
	var postDistance float64
	if err := database.QueryRowContext(ctx, `SELECT p.position_provenance,sqrt((p.x-s.x)^2+(p.y-s.y)^2+(p.z-s.z)^2)
FROM spatial_catalog_entities p JOIN spatial_catalog_entities s ON s.catalog_id=p.catalog_id AND s.id=p.anchor_id
WHERE p.catalog_id=$1 AND p.id=$2`, first, "post_"+postID).Scan(&postProvenance, &postDistance); err != nil {
		t.Fatal(err)
	}
	if postProvenance != "subreddit-seeded" || postDistance > 20 {
		t.Fatalf("post did not follow its subreddit: provenance=%s distance=%g", postProvenance, postDistance)
	}

	second, err := BuildSpatialCatalogWithOptions(ctx, database, watermark.Add(time.Second), SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	var secondMode string
	if err := database.QueryRowContext(ctx, `SELECT config->>'placement_mode' FROM spatial_catalogs WHERE id=$1`, second).Scan(&secondMode); err != nil {
		t.Fatal(err)
	}
	if secondMode != "warm" {
		t.Fatalf("second placement mode %q, want warm", secondMode)
	}
	var x2, y2, z2 float64
	if err := database.QueryRowContext(ctx, `SELECT x,y,z,position_provenance FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, second, "comment_"+commentID).Scan(&x2, &y2, &z2, &provenance); err != nil {
		t.Fatal(err)
	}
	if x1 != x2 || y1 != y2 || z1 != z2 || provenance != "warm-start" {
		t.Fatalf("warm start changed position: first=(%g,%g,%g) second=(%g,%g,%g) provenance=%s", x1, y1, z1, x2, y2, z2, provenance)
	}

	// A subreddit that shares no users sits on the isolated shell, and a
	// linked subreddit added to a warm build settles among its neighbours
	// without moving any placed subreddit.
	var subredditC, subredditD int32
	if err := database.QueryRowContext(ctx, `INSERT INTO subreddits(name,title,subscribers) VALUES($1,'Gamma island',10) RETURNING id`, "catalog_gamma_"+suffix).Scan(&subredditC); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO subreddits(name,title,subscribers) VALUES($1,'Delta newcomer',20) RETURNING id`, "catalog_delta_"+suffix).Scan(&subredditD); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = database.Exec(`DELETE FROM subreddit_relationships WHERE source_subreddit_id IN ($1,$2) OR target_subreddit_id IN ($1,$2)`, subredditC, subredditD)
		_, _ = database.Exec(`DELETE FROM subreddits WHERE id IN ($1,$2)`, subredditC, subredditD)
	}()
	if _, err := database.ExecContext(ctx, `INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count) VALUES($1,$2,6)
ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count,updated_at=now()`, subredditD, subredditA); err != nil {
		t.Fatal(err)
	}
	third, err := BuildSpatialCatalogWithOptions(ctx, database, watermark.Add(2*time.Hour), SpatialCatalogOptions{Retention: 1, WorkMemMB: 16})
	if err != nil {
		t.Fatal(err)
	}
	var x3, y3, z3 float64
	if err := database.QueryRowContext(ctx, `SELECT x,y,z,position_provenance FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, third, fmt.Sprintf("subreddit_%d", subredditA)).Scan(&x3, &y3, &z3, &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance != "warm-start" || x3 != xa || y3 != ya || z3 != za {
		t.Fatalf("placed subreddit moved in a warm build: provenance=%s", provenance)
	}
	var shellProvenance string
	var shellRadius float64
	if err := database.QueryRowContext(ctx, `SELECT position_provenance,sqrt(x*x+y*y+z*z) FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, third, fmt.Sprintf("subreddit_%d", subredditC)).Scan(&shellProvenance, &shellRadius); err != nil {
		t.Fatal(err)
	}
	if shellProvenance != "deterministic-shell" || shellRadius < defaultCatalogWorldRadius*isolatedShellInner-1 || shellRadius > defaultCatalogWorldRadius*isolatedShellOuter+1 {
		t.Fatalf("isolated subreddit placement: provenance=%s radius=%g", shellProvenance, shellRadius)
	}
	// With three bodies the natural spacing is the world radius itself, so
	// the check is relative: the newcomer, linked only to the first subreddit,
	// must settle nearer to it than to the unlinked second one.
	var settledProvenance string
	var toLinked, toUnlinked float64
	if err := database.QueryRowContext(ctx, `SELECT d.position_provenance,
 sqrt((d.x-a.x)^2+(d.y-a.y)^2+(d.z-a.z)^2),sqrt((d.x-b.x)^2+(d.y-b.y)^2+(d.z-b.z)^2)
FROM spatial_catalog_entities d
JOIN spatial_catalog_entities a ON a.catalog_id=d.catalog_id AND a.id=$3
JOIN spatial_catalog_entities b ON b.catalog_id=d.catalog_id AND b.id=$4
WHERE d.catalog_id=$1 AND d.id=$2`, third, fmt.Sprintf("subreddit_%d", subredditD), fmt.Sprintf("subreddit_%d", subredditA), fmt.Sprintf("subreddit_%d", subredditB)).Scan(&settledProvenance, &toLinked, &toUnlinked); err != nil {
		t.Fatal(err)
	}
	if settledProvenance != "force-settled" || toLinked >= toUnlinked {
		t.Fatalf("new linked subreddit placement: provenance=%s distance to linked=%g unlinked=%g", settledProvenance, toLinked, toUnlinked)
	}
	var current int64
	if err := database.QueryRowContext(ctx, `SELECT catalog_id FROM spatial_catalog_current WHERE singleton`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != third {
		t.Fatalf("current catalog=%d want %d", current, third)
	}
	var projectionTable sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT to_regclass('public.catalog_subreddit_overlap_projection')::text`).Scan(&projectionTable); err != nil {
		t.Fatal(err)
	}
	if projectionTable.Valid {
		t.Fatalf("overlap projection table leaked after publication: %s", projectionTable.String)
	}
}
