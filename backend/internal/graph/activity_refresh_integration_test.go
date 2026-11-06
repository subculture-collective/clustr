package graph

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

type activityKey struct{ a, b int32 }

func readActivityTables(t *testing.T, ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[activityKey]int32, map[activityKey]int32) {
	t.Helper()
	read := func(query string) map[activityKey]int32 {
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("read %q: %v", query, err)
		}
		defer rows.Close()
		out := map[activityKey]int32{}
		for rows.Next() {
			var k activityKey
			var v int32
			if err := rows.Scan(&k.a, &k.b, &v); err != nil {
				t.Fatal(err)
			}
			out[k] = v
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	return read(`SELECT user_id,subreddit_id,activity_count FROM user_subreddit_activity`),
		read(`SELECT source_subreddit_id,target_subreddit_id,overlap_count FROM subreddit_relationships`)
}

// TestIntegration_IncrementalActivityMatchesFullRebuild mutates source rows
// in every way the triggers track and requires the incremental refresh to
// produce exactly what a full rebuild produces.
func TestIntegration_IncrementalActivityMatchesFullRebuild(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	cleanup := func() {
		exec(`TRUNCATE user_subreddit_activity, subreddit_relationships RESTART IDENTITY`)
		exec(`DELETE FROM comments WHERE id LIKE 'actref_%'`)
		exec(`DELETE FROM posts WHERE id LIKE 'actref_%'`)
		exec(`DELETE FROM users WHERE username LIKE 'actref_%'`)
		exec(`DELETE FROM subreddits WHERE name LIKE 'actref_%'`)
		exec(`DELETE FROM activity_dirty_users`)
	}
	cleanup()
	t.Cleanup(cleanup)

	const subs, users = 12, 30
	subIDs := make([]int32, subs)
	userIDs := make([]int32, users)
	for i := range subIDs {
		if err := conn.QueryRowContext(ctx, `INSERT INTO subreddits(name) VALUES($1) RETURNING id`, fmt.Sprintf("actref_s%d", i)).Scan(&subIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	for i := range userIDs {
		if err := conn.QueryRowContext(ctx, `INSERT INTO users(username) VALUES($1) RETURNING id`, fmt.Sprintf("actref_u%d", i)).Scan(&userIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	rng := rand.New(rand.NewSource(20261106))
	posts := 0
	addPost := func() string {
		posts++
		id := fmt.Sprintf("actref_p%d", posts)
		exec(`INSERT INTO posts(id,subreddit_id,author_id,title) VALUES($1,$2,$3,'t')`, id, subIDs[rng.Intn(subs)], userIDs[rng.Intn(users)])
		return id
	}
	comments := 0
	addComment := func(postID string) {
		comments++
		var sub int32
		if err := conn.QueryRowContext(ctx, `SELECT subreddit_id FROM posts WHERE id=$1`, postID).Scan(&sub); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO comments(id,post_id,author_id,subreddit_id,body) VALUES($1,$2,$3,$4,'b')`, fmt.Sprintf("actref_c%d", comments), postID, userIDs[rng.Intn(users)], sub)
	}
	var postIDs []string
	for i := 0; i < 60; i++ {
		postIDs = append(postIDs, addPost())
	}
	for i := 0; i < 300; i++ {
		addComment(postIDs[rng.Intn(len(postIDs))])
	}

	run := func(full bool) {
		t.Helper()
		if err := refreshActivityAndRelationships(ctx, conn, full); err != nil {
			t.Fatalf("refresh(full=%v): %v", full, err)
		}
	}
	expectFull := func(round int) {
		t.Helper()
		gotActivity, gotRelationships := readActivityTables(t, ctx, conn)
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := refreshActivityInTx(ctx, tx, true); err != nil {
			t.Fatal(err)
		}
		wantActivity, wantRelationships := readActivityTables(t, ctx, tx)
		if !reflect.DeepEqual(gotActivity, wantActivity) {
			t.Fatalf("round %d: incremental activity has %d rows, full rebuild %d; contents differ", round, len(gotActivity), len(wantActivity))
		}
		if !reflect.DeepEqual(gotRelationships, wantRelationships) {
			t.Fatalf("round %d: incremental relationships have %d rows, full rebuild %d; contents differ", round, len(gotRelationships), len(wantRelationships))
		}
		if len(wantRelationships) == 0 {
			t.Fatalf("round %d: fixture produced no relationships", round)
		}
	}

	exec(`INSERT INTO activity_dirty_users(user_id) VALUES (NULL)`)
	run(false) // the sentinel forces the initial full rebuild
	expectFull(0)

	for round := 1; round <= 8; round++ {
		for i := 0; i < 15; i++ {
			switch rng.Intn(6) {
			case 0:
				postIDs = append(postIDs, addPost())
			case 1:
				addComment(postIDs[rng.Intn(len(postIDs))])
			case 2: // reassign a comment's author
				exec(`UPDATE comments SET author_id=$1 WHERE id=(SELECT id FROM comments WHERE id LIKE 'actref_%' ORDER BY id OFFSET $2 LIMIT 1)`, userIDs[rng.Intn(users)], rng.Intn(comments/2))
			case 3: // move a post to another subreddit
				exec(`UPDATE posts SET subreddit_id=$1 WHERE id=$2`, subIDs[rng.Intn(subs)], postIDs[rng.Intn(len(postIDs))])
			case 4: // delete a comment
				exec(`DELETE FROM comments WHERE id=(SELECT id FROM comments WHERE id LIKE 'actref_%' ORDER BY id OFFSET $1 LIMIT 1)`, rng.Intn(comments/2))
			case 5: // re-upsert a post unchanged; must not be logged
				exec(`UPDATE posts SET author_id=author_id,subreddit_id=subreddit_id,score=coalesce(score,0)+1 WHERE id=$1`, postIDs[rng.Intn(len(postIDs))])
			}
		}
		if round == 2 {
			// One author alone links two otherwise unused subreddits; deleting
			// those posts in round 5 must remove the pair entirely.
			var lonerUser, lonerA, lonerB int32
			if err := conn.QueryRowContext(ctx, `INSERT INTO users(username) VALUES('actref_loner') RETURNING id`).Scan(&lonerUser); err != nil {
				t.Fatal(err)
			}
			for _, target := range []*int32{&lonerA, &lonerB} {
				if err := conn.QueryRowContext(ctx, `INSERT INTO subreddits(name) VALUES($1) RETURNING id`, fmt.Sprintf("actref_loner_%p", target)).Scan(target); err != nil {
					t.Fatal(err)
				}
			}
			exec(`INSERT INTO posts(id,subreddit_id,author_id,title) VALUES('actref_loner_a',$1,$3,'t'),('actref_loner_b',$2,$3,'t')`, lonerA, lonerB, lonerUser)
		}
		if round == 5 {
			exec(`DELETE FROM posts WHERE id IN ('actref_loner_a','actref_loner_b')`)
		}
		if round == 4 {
			// Deleting a post cascades to its comments, whose triggers also fire.
			exec(`DELETE FROM posts WHERE id=$1`, postIDs[0])
			postIDs = postIDs[1:]
		}
		run(false)
		expectFull(round)
	}

	var pending int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM activity_dirty_users`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("dirty log after refresh = %d (%v), want 0", pending, err)
	}
	exec(`UPDATE posts SET score=coalesce(score,0)+1 WHERE id=$1`, postIDs[0])
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM activity_dirty_users`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("non-structural update logged %d authors (%v), want 0", pending, err)
	}
}
