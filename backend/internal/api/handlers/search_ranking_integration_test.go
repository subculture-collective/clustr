package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// TestIntegration_SearchRanksExactLabelsThenValue checks the catalog search
// tiers: the entity whose id is the query, then entities whose whole label is
// the query, then prefix matches, each ordered by value. It also checks that
// many low-value prefix matches cannot crowd out a high-value one.
func TestIntegration_SearchRanksExactLabelsThenValue(t *testing.T) {
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
	exec(`INSERT INTO spatial_catalogs(id,status,source_watermark,algorithm_version) VALUES(9002,'published',now(),'search-ranking-test')`)
	exec(`INSERT INTO graph_revisions(id,status,source_watermark,layout_algorithm,layout_seed,layout_dimensions,published_at,spatial_catalog_id)
 VALUES(9002,'published',now(),'search-ranking-test',1,3,now(),9002)`)
	rows := []string{
		"(9002,'subreddit_1569','funny','subreddit',67059287,0,0,0,'test',now())",
		"(9002,'post_1r89lzg','Funny','post',917,1,0,0,'test',now())",
		"(9002,'subreddit_14301','FunnyAnimals','subreddit',8526783,2,0,0,'test',now())",
		"(9002,'subreddit_124','ask','subreddit',1286953,3,0,0,'test',now())",
		"(9002,'user_633135','ask','user',2,4,0,0,'test',now())",
		"(9002,'subreddit_1','AskReddit','subreddit',59435431,5,0,0,'test',now())",
		"(9002,'comment_1','Ask me later','comment',900000000,6,0,0,'test',now())",
	}
	for i := 0; i < 60; i++ {
		rows = append(rows, fmt.Sprintf("(9002,'post_ask_%02d','Ask a question %02d','post',5,%d,1,0,'test',now())", i, i, i))
	}
	exec(`INSERT INTO spatial_catalog_entities(catalog_id,id,label,type,value,x,y,z,position_provenance,source_updated_at) VALUES ` + strings.Join(rows, ","))

	h := NewRevisionHandler(tx)
	search := func(query string, limit int) []string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/search?node=%s&limit=%d&revision=9002", query, limit), nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("search %q status = %d: %s", query, rr.Code, rr.Body.String())
		}
		var body struct {
			Results []struct {
				ID string `json:"ID"`
			} `json:"results"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode search %q: %v", query, err)
		}
		ids := make([]string, 0, len(body.Results))
		for _, result := range body.Results {
			ids = append(ids, result.ID)
		}
		return ids
	}
	expectPrefix := func(query string, got, want []string) {
		t.Helper()
		if len(got) < len(want) {
			t.Fatalf("search %q = %v, want it to start with %v", query, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("search %q = %v, want it to start with %v", query, got, want)
			}
		}
	}

	// A label that matches exactly outranks a bigger prefix match, and the
	// exact matches are ordered by value: the subreddit before the post.
	expectPrefix("funny", search("funny", 10), []string{"subreddit_1569", "post_1r89lzg", "subreddit_14301"})

	// Sixty "Ask a question" posts of value 5 exist. AskReddit must still
	// appear right after the two exact "ask" labels, and comments are never
	// searchable.
	got := search("ask", 10)
	expectPrefix("ask", got, []string{"subreddit_124", "user_633135", "subreddit_1"})
	for _, id := range got {
		if id == "comment_1" {
			t.Fatalf("search ask returned a comment: %v", got)
		}
	}

	// Case does not matter, and an id query puts that entity first.
	expectPrefix("FUNNY", search("FUNNY", 3), []string{"subreddit_1569", "post_1r89lzg"})
	expectPrefix("subreddit_1", search("subreddit_1", 3), []string{"subreddit_1"})
}
