package graph

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestIntegration_PublishRevisionIsAtomicAndStable3D(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `TRUNCATE graph_nodes,graph_links CASCADE; DELETE FROM graph_communities;
INSERT INTO subreddits(id,name) VALUES (1900000001,'revision_source_a'),(1900000002,'revision_source_b') ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name;
INSERT INTO users(id,username) VALUES (1900000001,'revision_user_a'),(1900000002,'revision_user_b') ON CONFLICT(id) DO UPDATE SET username=EXCLUDED.username;
INSERT INTO posts(id,subreddit_id,author_id,title) VALUES
 ('revision_pa1',1900000001,1900000001,'a1'),('revision_pb1',1900000002,1900000001,'b1'),
 ('revision_pa2',1900000001,1900000002,'a2'),('revision_pb2',1900000002,1900000002,'b2')
ON CONFLICT(id) DO UPDATE SET subreddit_id=EXCLUDED.subreddit_id,author_id=EXCLUDED.author_id;
INSERT INTO user_subreddit_activity(user_id,subreddit_id,activity_count) VALUES
 (1900000001,1900000001,1),(1900000001,1900000002,1),
 (1900000002,1900000001,1),(1900000002,1900000002,1)
ON CONFLICT(user_id,subreddit_id) DO UPDATE SET activity_count=EXCLUDED.activity_count;
INSERT INTO subreddit_relationships(source_subreddit_id,target_subreddit_id,overlap_count) VALUES
 (1900000001,1900000002,2),(1900000002,1900000001,2)
ON CONFLICT(source_subreddit_id,target_subreddit_id) DO UPDATE SET overlap_count=EXCLUDED.overlap_count;
INSERT INTO graph_nodes(id,name,val,type,pos_x,pos_y,pos_z) VALUES
	 ('subreddit_1900000001','A','100','subreddit',0,0,0),('subreddit_1900000002','B','90','subreddit',100,0,0),('user_1900000001','U','5','user',50,20,0);
	INSERT INTO graph_links(source,target) VALUES ('subreddit_1900000001','subreddit_1900000002'),('subreddit_1900000002','subreddit_1900000001'),('user_1900000001','subreddit_1900000001');
INSERT INTO graph_communities(id,label,size) VALUES (900001,'Alpha',2),(900002,'Beta',1);
	INSERT INTO graph_community_members(community_id,node_id) VALUES (900001,'subreddit_1900000001'),(900001,'user_1900000001'),(900002,'subreddit_1900000002')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM user_subreddit_activity WHERE user_id IN (1900000001,1900000002);
DELETE FROM subreddit_relationships WHERE source_subreddit_id IN (1900000001,1900000002) OR target_subreddit_id IN (1900000001,1900000002);
DELETE FROM posts WHERE id LIKE 'revision_p%'; DELETE FROM users WHERE id IN (1900000001,1900000002); DELETE FROM subreddits WHERE id IN (1900000001,1900000002)`)
	})
	expectedWatermark := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	first, err := PublishRevisionAtWatermark(ctx, conn, expectedWatermark)
	if err != nil {
		t.Fatal(err)
	}
	var actualWatermark time.Time
	if err := conn.QueryRowContext(ctx, `SELECT source_watermark FROM graph_revisions WHERE id=$1`, first).Scan(&actualWatermark); err != nil || !actualWatermark.Equal(expectedWatermark) {
		t.Fatalf("source watermark = %v, want %v, err=%v", actualWatermark, expectedWatermark, err)
	}
	var minZ, maxZ float64
	if err := conn.QueryRowContext(ctx, `SELECT min_z,max_z FROM graph_revisions WHERE id=$1`, first).Scan(&minZ, &maxZ); err != nil || minZ == maxZ {
		t.Fatalf("revision is not genuine 3D: min_z=%v max_z=%v err=%v", minZ, maxZ, err)
	}
	var weightedOverlap int64
	if err := conn.QueryRowContext(ctx, `SELECT weight FROM graph_revision_links WHERE revision_id=$1 AND relation='subreddit_overlap'`, first).Scan(&weightedOverlap); err != nil || weightedOverlap != 2 {
		t.Fatalf("undirected repeated overlap was not weighted: weight=%d err=%v", weightedOverlap, err)
	}
	var landmarkID string
	var x1, y1, z1 float64
	if err := conn.QueryRowContext(ctx, `SELECT c.community_id,c.x,c.y,c.z
FROM graph_revision_communities c
JOIN graph_revision_community_members m
  ON m.revision_id=c.revision_id AND m.community_id=c.community_id
WHERE c.revision_id=$1 AND m.node_id='subreddit_1900000001'`, first).Scan(&landmarkID, &x1, &y1, &z1); err != nil {
		t.Fatal(err)
	}
	second, err := PublishRevision(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	var matchedID string
	var x2, y2, z2 float64
	if err := conn.QueryRowContext(ctx, `SELECT c.community_id,c.x,c.y,c.z
FROM graph_revision_communities c
JOIN graph_revision_community_members m
  ON m.revision_id=c.revision_id AND m.community_id=c.community_id
WHERE c.revision_id=$1 AND m.node_id='subreddit_1900000001'`, second).Scan(&matchedID, &x2, &y2, &z2); err != nil {
		t.Fatal(err)
	}
	if matchedID != landmarkID || x1 != x2 || y1 != y2 || z1 != z2 {
		t.Fatalf("landmark continuity failed: %s (%v,%v,%v) -> %s (%v,%v,%v)", landmarkID, x1, y1, z1, matchedID, x2, y2, z2)
	}
	// Publication caps select deterministically before projection.  The link
	// insert joins both selected endpoints, so a capped world cannot contain an
	// orphan even when its source workspace does.
	capped, err := PublishRevisionWithOptions(ctx, conn, RevisionOptions{NodeCap: 2, LinkCap: 100, Retention: 3})
	if err != nil {
		t.Fatal(err)
	}
	var cappedNodes, orphanLinks int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM graph_revision_nodes WHERE revision_id=$1`, capped).Scan(&cappedNodes); err != nil || cappedNodes != 2 {
		t.Fatalf("node cap not honored: count=%d err=%v", cappedNodes, err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM graph_revision_links l LEFT JOIN graph_revision_nodes s ON s.revision_id=l.revision_id AND s.id=l.source LEFT JOIN graph_revision_nodes t ON t.revision_id=l.revision_id AND t.id=l.target WHERE l.revision_id=$1 AND (s.id IS NULL OR t.id IS NULL)`, capped).Scan(&orphanLinks); err != nil || orphanLinks != 0 {
		t.Fatalf("capped revision has orphan links: count=%d err=%v", orphanLinks, err)
	}
	// A graph with nodes but no authoritative community partition must not
	// replace the current explorer world with an empty overview.
	if _, err := conn.ExecContext(ctx, `DELETE FROM graph_community_members; DELETE FROM graph_communities`); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishRevision(ctx, conn); err == nil {
		t.Fatal("community-less world unexpectedly published")
	}
	var currentAfterCommunityFailure int64
	if err := conn.QueryRowContext(ctx, `SELECT revision_id FROM graph_revision_current WHERE singleton`).Scan(&currentAfterCommunityFailure); err != nil || currentAfterCommunityFailure != capped {
		t.Fatalf("community-less publication changed current pointer: got=%d want=%d err=%v", currentAfterCommunityFailure, capped, err)
	}
	if _, err := conn.ExecContext(ctx, `TRUNCATE graph_nodes,graph_links CASCADE; DELETE FROM graph_communities`); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishRevision(ctx, conn); err == nil {
		t.Fatal("empty world unexpectedly published")
	}
	var current int64
	if err := conn.QueryRowContext(ctx, `SELECT revision_id FROM graph_revision_current WHERE singleton`).Scan(&current); err != nil || current != capped {
		t.Fatalf("failed publication changed current pointer: got=%d want=%d err=%v", current, capped, err)
	}
}
