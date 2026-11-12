package graph

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// TestIntegration_StoreCommunitiesBatched verifies that batched persistence
// assigns ascending IDs in community order and stores every member, link and
// hierarchy row, including NULL parents and centroids.
func TestIntegration_StoreCommunitiesBatched(t *testing.T) {
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

	nodeIDs := make([]string, 6)
	for i := range nodeIDs {
		nodeIDs[i] = fmt.Sprintf("batch_store_%d", i)
		if _, err := conn.ExecContext(ctx, `INSERT INTO graph_nodes(id,name,val,type) VALUES($1,$1,'1','subreddit') ON CONFLICT(id) DO NOTHING`, nodeIDs[i]); err != nil {
			t.Fatalf("seed node: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = conn.ExecContext(ctx, `TRUNCATE graph_communities CASCADE`)
		_, _ = conn.ExecContext(ctx, `DELETE FROM graph_community_hierarchy WHERE node_id LIKE 'batch_store_%'`)
		_, _ = conn.ExecContext(ctx, `DELETE FROM graph_nodes WHERE id LIKE 'batch_store_%'`)
	})

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	svc := NewService(q)

	result := &CommunityResult{Modularity: 0.42, Communities: []Community{
		{ID: 0, Label: "first", Members: []string{nodeIDs[0], nodeIDs[1], nodeIDs[2]}},
		{ID: 1, Label: "second", Members: []string{nodeIDs[3], nodeIDs[4]}},
		{ID: 2, Label: "third", Members: []string{nodeIDs[5]}},
	}}
	links := []db.ListGraphLinksAmongRow{
		{Source: nodeIDs[0], Target: nodeIDs[3]},
		{Source: nodeIDs[4], Target: nodeIDs[1]},
		{Source: nodeIDs[2], Target: nodeIDs[5]},
		{Source: nodeIDs[0], Target: nodeIDs[1]}, // intra-community, not stored
	}
	nodeToDB, err := svc.storeCommunities(ctx, q, result, nil, links)
	if err != nil {
		t.Fatalf("storeCommunities: %v", err)
	}

	rows, err := tx.QueryContext(ctx, `SELECT id,label,size,modularity FROM graph_communities ORDER BY id`)
	if err != nil {
		t.Fatalf("read communities: %v", err)
	}
	var ids []int32
	for i := 0; rows.Next(); i++ {
		var id, size int32
		var label string
		var modularity float64
		if err := rows.Scan(&id, &label, &size, &modularity); err != nil {
			t.Fatal(err)
		}
		want := result.Communities[i]
		if label != want.Label || int(size) != len(want.Members) || modularity != 0.42 {
			t.Fatalf("community %d = (%q,%d,%v), want (%q,%d,0.42)", i, label, size, modularity, want.Label, len(want.Members))
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 3 || ids[1] != ids[0]+1 || ids[2] != ids[1]+1 {
		t.Fatalf("community ids = %v, want three consecutive ascending ids", ids)
	}
	for i, comm := range result.Communities {
		for _, member := range comm.Members {
			if nodeToDB[member] != ids[i] {
				t.Fatalf("member %s mapped to %d, want %d", member, nodeToDB[member], ids[i])
			}
		}
	}
	var members int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM graph_community_members`).Scan(&members); err != nil || members != 6 {
		t.Fatalf("members = %d (%v), want 6", members, err)
	}
	type pair struct{ a, b, w int32 }
	got := map[pair]bool{}
	linkRows, err := tx.QueryContext(ctx, `SELECT source_community_id,target_community_id,weight FROM graph_community_links`)
	if err != nil {
		t.Fatal(err)
	}
	for linkRows.Next() {
		var p pair
		if err := linkRows.Scan(&p.a, &p.b, &p.w); err != nil {
			t.Fatal(err)
		}
		got[p] = true
	}
	linkRows.Close()
	want := map[pair]bool{{ids[0], ids[1], 2}: true, {ids[0], ids[2], 1}: true}
	if len(got) != len(want) || !got[pair{ids[0], ids[1], 2}] || !got[pair{ids[0], ids[2], 1}] {
		t.Fatalf("community links = %v, want %v", got, want)
	}

	hierarchy := []HierarchyLevel{
		{Level: 0, NodeToCommunity: map[string]int{nodeIDs[0]: 0, nodeIDs[1]: 1},
			CommunityToParent:  map[int]int{},
			CommunityCentroids: map[int][3]float64{0: {1, 2, 3}}},
		{Level: 1, NodeToCommunity: map[string]int{nodeIDs[0]: 7, nodeIDs[1]: 8},
			CommunityToParent:  map[int]int{7: 0, 8: -1},
			CommunityCentroids: map[int][3]float64{}},
		{Level: 2, NodeToCommunity: map[string]int{}},
	}
	if err := svc.storeHierarchy(ctx, q, hierarchy); err != nil {
		t.Fatalf("storeHierarchy: %v", err)
	}
	type hrow struct {
		level, community int32
		parent           sql.NullInt32
		cx               sql.NullFloat64
	}
	hrows, err := tx.QueryContext(ctx, `SELECT level,community_id,parent_community_id,centroid_x FROM graph_community_hierarchy WHERE node_id=$1 ORDER BY level`, nodeIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	var stored []hrow
	for hrows.Next() {
		var r hrow
		if err := hrows.Scan(&r.level, &r.community, &r.parent, &r.cx); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, r)
	}
	hrows.Close()
	if len(stored) != 2 ||
		stored[0] != (hrow{0, 0, sql.NullInt32{}, sql.NullFloat64{Float64: 1, Valid: true}}) ||
		stored[1] != (hrow{1, 7, sql.NullInt32{Int32: 0, Valid: true}, sql.NullFloat64{}}) {
		t.Fatalf("hierarchy rows = %+v", stored)
	}
	var mixedParent sql.NullInt32
	if err := tx.QueryRowContext(ctx, `SELECT parent_community_id FROM graph_community_hierarchy WHERE node_id=$1 AND level=1`, nodeIDs[1]).Scan(&mixedParent); err != nil || mixedParent.Valid {
		t.Fatalf("mixed parent = %+v (%v), want NULL", mixedParent, err)
	}
}
