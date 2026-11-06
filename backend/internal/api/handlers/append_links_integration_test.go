package handlers

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"testing"

	_ "github.com/lib/pq"
)

// TestIntegration_AppendLinksStructuralRelations checks every structural link
// type and a catalog link, and that links to entities outside the visible set
// are excluded.
func TestIntegration_AppendLinksStructuralRelations(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(query string) {
		t.Helper()
		if _, err := tx.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO spatial_catalogs(id,status,source_watermark,algorithm_version) VALUES(9001,'published',now(),'append-links-test')`)
	exec(`INSERT INTO spatial_catalog_entities(catalog_id,id,label,type,x,y,z,position_provenance,source_updated_at,parent_id,anchor_id,author_id) VALUES
 (9001,'subreddit_1','s','subreddit',0,0,0,'test',now(),NULL,NULL,NULL),
 (9001,'user_1','u','user',1,0,0,'test',now(),NULL,NULL,NULL),
 (9001,'user_2','hidden author','user',2,0,0,'test',now(),NULL,NULL,NULL),
 (9001,'post_1','p','post',0,1,0,'test',now(),'subreddit_1',NULL,'user_1'),
 (9001,'comment_1','c1','comment',0,2,0,'test',now(),'post_1','post_1','user_1'),
 (9001,'comment_2','c2','comment',0,3,0,'test',now(),'comment_1','post_1','user_2')`)
	exec(`INSERT INTO spatial_catalog_links(catalog_id,source,target,relation,directed,weight) VALUES
 (9001,'user_1','subreddit_1','user_activity',true,7),
 (9001,'user_2','subreddit_1','user_activity',true,9)`)

	h := NewRevisionHandler(tx)
	payload := scenePayload{RevisionID: 1, SpatialCatalog: 9001}
	visible := []string{"subreddit_1", "user_1", "post_1", "comment_1", "comment_2"}
	if err := h.appendLinks(ctx, &payload, visible, 100); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(payload.Links))
	for _, l := range payload.Links {
		got = append(got, l.Source+">"+l.Target+":"+l.Type)
	}
	sort.Strings(got)
	want := []string{
		"comment_1>comment_2:reply",
		"comment_1>post_1:comment_on",
		"comment_2>post_1:comment_on",
		"post_1>subreddit_1:published_in",
		"user_1>comment_1:authorship",
		"user_1>post_1:authorship",
		"user_1>subreddit_1:user_activity",
	}
	if len(got) != len(want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("links = %v, want %v", got, want)
		}
	}
	if payload.Links[0].Type != "user_activity" || payload.Links[0].Weight != 7 {
		t.Fatalf("first link = %+v, want the weight-7 catalog link first", payload.Links[0])
	}
}
