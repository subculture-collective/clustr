package graph

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// graph_links is preserved across incremental runs, so every hourly run
// re-offers mostly existing (source,target) pairs. PostgreSQL evaluates the
// id default (nextval) before ON CONFLICT discards a duplicate, so a skipped
// row still consumes an identifier. These tests pin the requirement that
// re-offering existing links leaves graph_links_id_seq untouched.

func openLinkSequenceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func seedLinkSequenceFixture(t *testing.T, conn *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `TRUNCATE graph_nodes,graph_links CASCADE;
INSERT INTO subreddits(id,name) VALUES (1910000001,'linkseq_a'),(1910000002,'linkseq_b'),(1910000003,'linkseq_c')
 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name;
INSERT INTO users(id,username) VALUES (1910000001,'linkseq_user_a'),(1910000002,'linkseq_user_b')
 ON CONFLICT(id) DO UPDATE SET username=EXCLUDED.username;
INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count) VALUES
 (1910000001,1910000001,3),(1910000001,1910000002,2),(1910000002,1910000003,1)
 ON CONFLICT(user_id,subreddit_id) DO UPDATE SET activity_count=EXCLUDED.activity_count;
INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count) VALUES
 (1910000001,1910000002,4),(1910000002,1910000003,2)
 ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count;
INSERT INTO graph_nodes(id,name,val,type) VALUES
 ('subreddit_1910000001','A','10','subreddit'),('subreddit_1910000002','B','9','subreddit'),
 ('subreddit_1910000003','C','8','subreddit'),
 ('user_1910000001','UA','3','user'),('user_1910000002','UB','1','user')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`TRUNCATE graph_nodes,graph_links CASCADE;
DELETE FROM user_subreddit_activity WHERE user_id IN (1910000001,1910000002);
DELETE FROM subreddit_relationships WHERE source_subreddit_id IN (1910000001,1910000002,1910000003);
DELETE FROM users WHERE id IN (1910000001,1910000002);
DELETE FROM subreddits WHERE id IN (1910000001,1910000002,1910000003)`)
	})
}

// graphLinkSequenceDraws reports how many identifiers graph_links_id_seq has
// handed out. It reads the sequence relation directly, so it does not itself
// advance the sequence.
func graphLinkSequenceDraws(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	var lastValue int64
	var isCalled bool
	if err := conn.QueryRow(`SELECT last_value,is_called FROM graph_links_id_seq`).Scan(&lastValue, &isCalled); err != nil {
		t.Fatal(err)
	}
	if !isCalled {
		return lastValue - 1
	}
	return lastValue
}

func graphLinkCount(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := conn.QueryRow(`SELECT count(*) FROM graph_links`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegration_WorkspaceLinkMaterializationReusesExistingLinksWithoutSequenceDraws(t *testing.T) {
	conn := openLinkSequenceTestDB(t)
	seedLinkSequenceFixture(t, conn)
	ctx := context.Background()

	inserted, err := materializeWorkspaceLinks(ctx, conn, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 5 {
		t.Fatalf("first materialization inserted %d links, want 5", inserted)
	}
	before := graphLinkSequenceDraws(t, conn)

	for run := 0; run < 3; run++ {
		inserted, err = materializeWorkspaceLinks(ctx, conn, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if inserted != 0 {
			t.Fatalf("repeat materialization %d inserted %d links, want 0", run, inserted)
		}
	}
	if after := graphLinkSequenceDraws(t, conn); after != before {
		t.Fatalf("repeat materialization drew %d graph_links ids for zero new links", after-before)
	}

	// A genuinely new candidate still gets exactly one identifier.
	if _, err := conn.ExecContext(ctx, `INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count)
VALUES (1910000002,1910000001,5)`); err != nil {
		t.Fatal(err)
	}
	inserted, err = materializeWorkspaceLinks(ctx, conn, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 1 {
		t.Fatalf("materialization after one new activity inserted %d links, want 1", inserted)
	}
	if after := graphLinkSequenceDraws(t, conn); after != before+1 {
		t.Fatalf("one new link drew %d graph_links ids, want 1", after-before)
	}
	if n := graphLinkCount(t, conn); n != 6 {
		t.Fatalf("graph_links count = %d, want 6", n)
	}
}

func TestIntegration_WorkspaceLinkMaterializationKeepsTheCapOverCandidates(t *testing.T) {
	conn := openLinkSequenceTestDB(t)
	seedLinkSequenceFixture(t, conn)
	ctx := context.Background()

	// The cap selects the strongest candidates. Existing links among them must
	// not let weaker candidates in, or the workspace would grow every run.
	if _, err := materializeWorkspaceLinks(ctx, conn, 2); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 3; run++ {
		if _, err := materializeWorkspaceLinks(ctx, conn, 2); err != nil {
			t.Fatal(err)
		}
	}
	if n := graphLinkCount(t, conn); n != 2 {
		t.Fatalf("graph_links count after capped repeats = %d, want 2", n)
	}
}

func TestIntegration_BatchInsertGraphLinksSkipsExistingWithoutSequenceDraws(t *testing.T) {
	conn := openLinkSequenceTestDB(t)
	seedLinkSequenceFixture(t, conn)
	ctx := context.Background()
	q := db.New(conn)

	links := []db.BulkInsertGraphLinkParams{
		{Source: "user_1910000001", Target: "subreddit_1910000001"},
		{Source: "user_1910000002", Target: "subreddit_1910000003"},
	}
	if err := q.BatchInsertGraphLinks(ctx, links, 1000); err != nil {
		t.Fatal(err)
	}
	before := graphLinkSequenceDraws(t, conn)
	if err := q.BatchInsertGraphLinks(ctx, links, 1000); err != nil {
		t.Fatal(err)
	}
	if after := graphLinkSequenceDraws(t, conn); after != before {
		t.Fatalf("re-inserting existing batch drew %d graph_links ids", after-before)
	}

	withNew := append(links, db.BulkInsertGraphLinkParams{Source: "user_1910000001", Target: "subreddit_1910000002"})
	if err := q.BatchInsertGraphLinks(ctx, withNew, 1000); err != nil {
		t.Fatal(err)
	}
	if after := graphLinkSequenceDraws(t, conn); after != before+1 {
		t.Fatalf("one new batched link drew %d graph_links ids, want 1", after-before)
	}
	if n := graphLinkCount(t, conn); n != 3 {
		t.Fatalf("graph_links count = %d, want 3", n)
	}
}

func TestIntegration_BulkInsertGraphLinkSkipsExistingWithoutSequenceDraws(t *testing.T) {
	conn := openLinkSequenceTestDB(t)
	seedLinkSequenceFixture(t, conn)
	ctx := context.Background()
	q := db.New(conn)

	link := db.BulkInsertGraphLinkParams{Source: "subreddit_1910000001", Target: "subreddit_1910000002"}
	if err := q.BulkInsertGraphLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	before := graphLinkSequenceDraws(t, conn)
	for i := 0; i < 3; i++ {
		if err := q.BulkInsertGraphLink(ctx, link); err != nil {
			t.Fatal(err)
		}
	}
	if after := graphLinkSequenceDraws(t, conn); after != before {
		t.Fatalf("re-inserting an existing link drew %d graph_links ids", after-before)
	}
	if n := graphLinkCount(t, conn); n != 1 {
		t.Fatalf("graph_links count = %d, want 1", n)
	}
}

func TestIntegration_ClearGraphTablesRestartsGraphLinkIdentity(t *testing.T) {
	conn := openLinkSequenceTestDB(t)
	seedLinkSequenceFixture(t, conn)
	ctx := context.Background()
	q := db.New(conn)

	if _, err := materializeWorkspaceLinks(ctx, conn, 1000); err != nil {
		t.Fatal(err)
	}
	if err := q.ClearGraphTables(ctx); err != nil {
		t.Fatal(err)
	}
	if draws := graphLinkSequenceDraws(t, conn); draws != 0 {
		t.Fatalf("graph_links_id_seq after ClearGraphTables has %d draws, want 0", draws)
	}
}
