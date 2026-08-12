package graph

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// fakeStore implements GraphStore for testing without a real DB.
type fakeStore struct {
	// inputs tracked
	insertedNodes map[string]db.BulkInsertGraphNodeParams
	insertedLinks [][2]string
	clearGraphErr error
	stateUpdates  []db.UpdatePrecalcStateParams
}

func newFakeStore() *fakeStore {
	return &fakeStore{insertedNodes: map[string]db.BulkInsertGraphNodeParams{}}
}

// Cleanup
func (f *fakeStore) ClearSubredditRelationships(ctx context.Context) error { return nil }
func (f *fakeStore) ClearUserSubredditActivity(ctx context.Context) error  { return nil }
func (f *fakeStore) ClearGraphTables(ctx context.Context) error            { return f.clearGraphErr }

// Reads
func (f *fakeStore) GetAllSubreddits(ctx context.Context) ([]db.GetAllSubredditsRow, error) {
	return []db.GetAllSubredditsRow{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}, nil
}
func (f *fakeStore) GetAllUsers(ctx context.Context) ([]db.GetAllUsersRow, error) {
	return []db.GetAllUsersRow{{ID: 10, Username: "u1"}}, nil
}
func (f *fakeStore) GetAllSubredditRelationships(ctx context.Context) ([]db.GetAllSubredditRelationshipsRow, error) {
	return nil, nil
}
func (f *fakeStore) GetAllUserSubredditActivity(ctx context.Context) ([]db.GetAllUserSubredditActivityRow, error) {
	return nil, nil
}

// Overlap + activity
func (f *fakeStore) GetSubredditOverlap(ctx context.Context, arg db.GetSubredditOverlapParams) (int64, error) {
	return 0, nil
}
func (f *fakeStore) CreateSubredditRelationship(ctx context.Context, arg db.CreateSubredditRelationshipParams) (db.SubredditRelationship, error) {
	return db.SubredditRelationship{}, nil
}
func (f *fakeStore) GetUserSubreddits(ctx context.Context, authorID int32) ([]db.GetUserSubredditsRow, error) {
	return []db.GetUserSubredditsRow{{ID: 1, Name: "a"}}, nil
}
func (f *fakeStore) GetUserSubredditActivityCount(ctx context.Context, arg db.GetUserSubredditActivityCountParams) (int32, error) {
	return 1, nil
}
func (f *fakeStore) CreateUserSubredditActivity(ctx context.Context, arg db.CreateUserSubredditActivityParams) (db.UserSubredditActivity, error) {
	return db.UserSubredditActivity{}, nil
}

// Graph data
func (f *fakeStore) BulkInsertGraphNode(ctx context.Context, arg db.BulkInsertGraphNodeParams) error {
	f.insertedNodes[arg.ID] = arg
	return nil
}
func (f *fakeStore) BulkInsertGraphLink(ctx context.Context, arg db.BulkInsertGraphLinkParams) error {
	f.insertedLinks = append(f.insertedLinks, [2]string{arg.Source, arg.Target})
	return nil
}

func (f *fakeStore) ListUsersWithActivity(ctx context.Context) ([]db.ListUsersWithActivityRow, error) {
	return []db.ListUsersWithActivityRow{{ID: 10, Username: "u1", TotalActivity: 2}}, nil
}

// Detailed content
func (f *fakeStore) ListPostsBySubreddit(ctx context.Context, arg db.ListPostsBySubredditParams) ([]db.Post, error) {
	// Two posts by same author in different subs to enable cross-linking
	if arg.SubredditID == 1 {
		return []db.Post{{ID: "p1", SubredditID: 1, AuthorID: 10, Title: sql.NullString{String: "t1", Valid: true}}}, nil
	}
	return []db.Post{{ID: "p2", SubredditID: 2, AuthorID: 10, Title: sql.NullString{String: "t2", Valid: true}}}, nil
}
func (f *fakeStore) ListCommentsByPost(ctx context.Context, postID string) ([]db.Comment, error) {
	return nil, nil
}
func (f *fakeStore) GetUserTotalActivity(ctx context.Context, authorID int32) (int32, error) {
	return 2, nil
}

// Incremental precalculation methods
func (f *fakeStore) GetPrecalcState(ctx context.Context) (db.PrecalcState, error) {
	// Return empty state for initial testing
	return db.PrecalcState{
		ID:                1,
		LastPrecalcAt:     sql.NullTime{Valid: false},
		LastFullPrecalcAt: sql.NullTime{Valid: false},
		TotalNodes:        sql.NullInt32{Int32: 0, Valid: false},
		TotalLinks:        sql.NullInt32{Int32: 0, Valid: false},
	}, nil
}

func (f *fakeStore) UpdatePrecalcState(ctx context.Context, arg db.UpdatePrecalcStateParams) error {
	f.stateUpdates = append(f.stateUpdates, arg)
	return nil
}

func (f *fakeStore) GetChangedSubredditsSince(ctx context.Context, updatedAt sql.NullTime) ([]db.GetChangedSubredditsSinceRow, error) {
	return nil, nil
}

func (f *fakeStore) GetChangedUsersSince(ctx context.Context, updatedAt sql.NullTime) ([]db.GetChangedUsersSinceRow, error) {
	return nil, nil
}

func (f *fakeStore) CountChangedEntities(ctx context.Context, updatedAt sql.NullTime) (db.CountChangedEntitiesRow, error) {
	return db.CountChangedEntitiesRow{
		ChangedSubreddits: 0,
		ChangedUsers:      0,
		ChangedPosts:      0,
		ChangedComments:   0,
	}, nil
}

func (f *fakeStore) GetUserActivitySince(ctx context.Context, updatedAt sql.NullTime) ([]db.GetUserActivitySinceRow, error) {
	return nil, nil
}

func (f *fakeStore) GetAffectedUserIDs(ctx context.Context, updatedAt sql.NullTime) ([]int32, error) {
	return nil, nil
}

func (f *fakeStore) GetAffectedSubredditIDs(ctx context.Context, updatedAt sql.NullTime) ([]int32, error) {
	return nil, nil
}

// Version tracking methods
func (f *fakeStore) CreateGraphVersion(ctx context.Context, arg db.CreateGraphVersionParams) (db.GraphVersion, error) {
	return db.GraphVersion{ID: 1, NodeCount: arg.NodeCount, LinkCount: arg.LinkCount, Status: arg.Status}, nil
}

func (f *fakeStore) GetCurrentGraphVersion(ctx context.Context) (db.GraphVersion, error) {
	return db.GraphVersion{ID: 1, NodeCount: 100, LinkCount: 200, Status: "completed"}, nil
}

func (f *fakeStore) UpdateGraphVersionStatus(ctx context.Context, arg db.UpdateGraphVersionStatusParams) error {
	return nil
}

func (f *fakeStore) DeleteOldGraphVersions(ctx context.Context, retention int32) error {
	return nil
}

func (f *fakeStore) CountGraphVersions(ctx context.Context) (int64, error) {
	return 5, nil
}

func (f *fakeStore) CreateGraphDiff(ctx context.Context, arg db.CreateGraphDiffParams) error {
	return nil
}

func (f *fakeStore) UpdatePrecalcStateVersion(ctx context.Context, versionID sql.NullInt64) error {
	return nil
}

func (f *fakeStore) GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error) {
	return nil, nil
}

func TestPrecalculateGraphData_MinimalWithoutDetailed(t *testing.T) {
	os.Setenv("DETAILED_GRAPH", "false")
	t.Cleanup(func() { os.Unsetenv("DETAILED_GRAPH") })
	config.ResetForTest()
	config.Load()

	fs := newFakeStore()
	svc := NewService(fs)
	if err := svc.PrecalculateGraphData(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect user and subreddit nodes, but no post/comment nodes
	if _, ok := fs.insertedNodes["user_10"]; !ok {
		t.Fatalf("expected user node to be inserted")
	}
	if _, ok := fs.insertedNodes["subreddit_1"]; !ok {
		t.Fatalf("expected subreddit_1 node to be inserted")
	}
	for id := range fs.insertedNodes {
		if strings.HasPrefix(id, "post_") || strings.HasPrefix(id, "comment_") {
			t.Fatalf("did not expect content node in minimal mode: %s", id)
		}
	}
}

func TestPrecalculateGraphData_AuthorCrossLinkCap(t *testing.T) {
	os.Setenv("DETAILED_GRAPH", "true")
	os.Setenv("MAX_AUTHOR_CONTENT_LINKS", "1")
	os.Setenv("POSTS_PER_SUB_IN_GRAPH", "5")
	t.Cleanup(func() {
		os.Unsetenv("DETAILED_GRAPH")
		os.Unsetenv("MAX_AUTHOR_CONTENT_LINKS")
		os.Unsetenv("POSTS_PER_SUB_IN_GRAPH")
	})
	config.ResetForTest()
	config.Load()

	fs := newFakeStore()
	svc := NewService(fs)
	if err := svc.PrecalculateGraphData(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Ensure at least one cross-link between p1 and p2 was added due to cap=1
	cross := 0
	for _, l := range fs.insertedLinks {
		if (l[0] == "post_p1" && l[1] == "post_p2") || (l[0] == "post_p2" && l[1] == "post_p1") {
			cross++
		}
	}
	if cross == 0 {
		t.Fatalf("expected at least one author cross-link between posts")
	}
}

func TestCheckPositionColumnsExist_Fake(t *testing.T) {
	// Test with fake store (should return false gracefully, not panic)
	fs := newFakeStore()
	svc := NewService(fs)

	// This should not panic even though fakeStore doesn't implement db.Queries
	err := svc.computeAndStoreLayout(context.Background())
	if err != nil {
		t.Fatalf("expected no error with fake store, got: %v", err)
	}
}

func TestPrecalculateGraphDataWithMode_IncrementalFirstRun(t *testing.T) {
	// Test that first run always does full rebuild
	os.Setenv("DETAILED_GRAPH", "false")
	t.Cleanup(func() { os.Unsetenv("DETAILED_GRAPH") })
	config.ResetForTest()
	config.Load()

	fs := newFakeStore()
	svc := NewService(fs)

	// First run should be full rebuild since no last_precalc_at
	if err := svc.PrecalculateGraphDataWithMode(context.Background(), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify nodes were inserted
	if _, ok := fs.insertedNodes["user_10"]; !ok {
		t.Fatalf("expected user node to be inserted")
	}
	if _, ok := fs.insertedNodes["subreddit_1"]; !ok {
		t.Fatalf("expected subreddit_1 node to be inserted")
	}
}

func TestPrecalculateGraphDataWithMode_ForceFullRebuild(t *testing.T) {
	// Test that fullRebuild flag forces full rebuild
	os.Setenv("DETAILED_GRAPH", "false")
	t.Cleanup(func() { os.Unsetenv("DETAILED_GRAPH") })
	config.ResetForTest()
	config.Load()

	fs := newFakeStore()
	svc := NewService(fs)

	// Force full rebuild
	if err := svc.PrecalculateGraphDataWithMode(context.Background(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify nodes were inserted
	if _, ok := fs.insertedNodes["user_10"]; !ok {
		t.Fatalf("expected user node to be inserted")
	}
}

func TestPrecalculateGraphDataFailureDoesNotAdvanceWatermark(t *testing.T) {
	config.ResetForTest()
	fs := newFakeStore()
	fs.clearGraphErr = errors.New("injected clear failure")
	svc := NewService(fs)

	if err := svc.PrecalculateGraphDataWithMode(context.Background(), true); err == nil {
		t.Fatal("expected injected calculation failure")
	}
	if len(fs.stateUpdates) != 0 {
		t.Fatalf("failed calculation wrote %d precalc state updates", len(fs.stateUpdates))
	}
}
