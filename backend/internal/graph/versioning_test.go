package graph

import (
	"context"
	"database/sql"
	"testing"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// mockVersionStore is a mock implementation of VersionStore for testing
type mockVersionStore struct {
	versions  map[int64]db.GraphVersion
	diffs     []db.CreateGraphDiffParams
	nextID    int64
	countFunc func(ctx context.Context) (int64, error)
}

func newMockVersionStore() *mockVersionStore {
	return &mockVersionStore{
		versions: make(map[int64]db.GraphVersion),
		diffs:    make([]db.CreateGraphDiffParams, 0),
		nextID:   1,
	}
}

// Implement all GraphStore methods (stubs)
func (m *mockVersionStore) ClearSubredditRelationships(ctx context.Context) error { return nil }
func (m *mockVersionStore) ClearUserSubredditActivity(ctx context.Context) error  { return nil }
func (m *mockVersionStore) ClearGraphTables(ctx context.Context) error            { return nil }
func (m *mockVersionStore) GetAllSubreddits(ctx context.Context) ([]db.GetAllSubredditsRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetAllUsers(ctx context.Context) ([]db.GetAllUsersRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetAllSubredditRelationships(ctx context.Context) ([]db.GetAllSubredditRelationshipsRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetAllUserSubredditActivity(ctx context.Context) ([]db.GetAllUserSubredditActivityRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetSubredditOverlap(ctx context.Context, arg db.GetSubredditOverlapParams) (int64, error) {
	return 0, nil
}
func (m *mockVersionStore) CreateSubredditRelationship(ctx context.Context, arg db.CreateSubredditRelationshipParams) (db.SubredditRelationship, error) {
	return db.SubredditRelationship{}, nil
}
func (m *mockVersionStore) GetUserSubreddits(ctx context.Context, authorID int32) ([]db.GetUserSubredditsRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetUserSubredditActivityCount(ctx context.Context, arg db.GetUserSubredditActivityCountParams) (int32, error) {
	return 0, nil
}
func (m *mockVersionStore) CreateUserSubredditActivity(ctx context.Context, arg db.CreateUserSubredditActivityParams) (db.UserSubredditActivity, error) {
	return db.UserSubredditActivity{}, nil
}
func (m *mockVersionStore) BulkInsertGraphNode(ctx context.Context, arg db.BulkInsertGraphNodeParams) error {
	return nil
}
func (m *mockVersionStore) BulkInsertGraphLink(ctx context.Context, arg db.BulkInsertGraphLinkParams) error {
	return nil
}
func (m *mockVersionStore) ListUsersWithActivity(ctx context.Context) ([]db.ListUsersWithActivityRow, error) {
	return nil, nil
}
func (m *mockVersionStore) ListPostsBySubreddit(ctx context.Context, arg db.ListPostsBySubredditParams) ([]db.Post, error) {
	return nil, nil
}
func (m *mockVersionStore) ListCommentsByPost(ctx context.Context, postID string) ([]db.Comment, error) {
	return nil, nil
}
func (m *mockVersionStore) GetUserTotalActivity(ctx context.Context, authorID int32) (int32, error) {
	return 0, nil
}
func (m *mockVersionStore) GetPrecalcState(ctx context.Context) (db.PrecalcState, error) {
	return db.PrecalcState{}, nil
}
func (m *mockVersionStore) UpdatePrecalcState(ctx context.Context, arg db.UpdatePrecalcStateParams) error {
	return nil
}
func (m *mockVersionStore) GetChangedSubredditsSince(ctx context.Context, updatedAt sql.NullTime) ([]db.GetChangedSubredditsSinceRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetChangedUsersSince(ctx context.Context, updatedAt sql.NullTime) ([]db.GetChangedUsersSinceRow, error) {
	return nil, nil
}
func (m *mockVersionStore) CountChangedEntities(ctx context.Context, updatedAt sql.NullTime) (db.CountChangedEntitiesRow, error) {
	return db.CountChangedEntitiesRow{}, nil
}
func (m *mockVersionStore) GetUserActivitySince(ctx context.Context, updatedAt sql.NullTime) ([]db.GetUserActivitySinceRow, error) {
	return nil, nil
}
func (m *mockVersionStore) GetAffectedUserIDs(ctx context.Context, updatedAt sql.NullTime) ([]int32, error) {
	return nil, nil
}
func (m *mockVersionStore) GetAffectedSubredditIDs(ctx context.Context, updatedAt sql.NullTime) ([]int32, error) {
	return nil, nil
}

// Version tracking methods (actual implementation for testing)
func (m *mockVersionStore) CreateGraphVersion(ctx context.Context, arg db.CreateGraphVersionParams) (db.GraphVersion, error) {
	version := db.GraphVersion{
		ID:                m.nextID,
		NodeCount:         arg.NodeCount,
		LinkCount:         arg.LinkCount,
		Status:            arg.Status,
		PrecalcDurationMs: arg.PrecalcDurationMs,
		IsFullRebuild:     arg.IsFullRebuild,
	}
	m.versions[m.nextID] = version
	m.nextID++
	return version, nil
}

func (m *mockVersionStore) GetCurrentGraphVersion(ctx context.Context) (db.GraphVersion, error) {
	if len(m.versions) == 0 {
		return db.GraphVersion{}, sql.ErrNoRows
	}
	// Return the highest ID
	maxID := int64(0)
	for id := range m.versions {
		if id > maxID {
			maxID = id
		}
	}
	return m.versions[maxID], nil
}

func (m *mockVersionStore) UpdateGraphVersionStatus(ctx context.Context, arg db.UpdateGraphVersionStatusParams) error {
	if v, exists := m.versions[arg.ID]; exists {
		v.Status = arg.Status
		v.PrecalcDurationMs = arg.PrecalcDurationMs
		m.versions[arg.ID] = v
	}
	return nil
}

func (m *mockVersionStore) DeleteOldGraphVersions(ctx context.Context, retention int32) error {
	return nil
}

func (m *mockVersionStore) CountGraphVersions(ctx context.Context) (int64, error) {
	if m.countFunc != nil {
		return m.countFunc(ctx)
	}
	return int64(len(m.versions)), nil
}

func (m *mockVersionStore) CreateGraphDiff(ctx context.Context, arg db.CreateGraphDiffParams) error {
	m.diffs = append(m.diffs, arg)
	return nil
}

func (m *mockVersionStore) UpdatePrecalcStateVersion(ctx context.Context, versionID sql.NullInt64) error {
	return nil
}

func (m *mockVersionStore) GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error) {
	return []db.GetPrecalculatedGraphDataCappedAllRow{}, nil
}

// Test diff calculation for first version (all adds)
func TestCalculateAndStoreDiffs_FirstVersion(t *testing.T) {
	ctx := context.Background()
	store := newMockVersionStore()

	// Create a version
	version, err := store.CreateGraphVersion(ctx, db.CreateGraphVersionParams{
		NodeCount: 2,
		LinkCount: 1,
		Status:    "completed",
	})
	if err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}

	// Create a new snapshot (oldSnapshot is nil for first version)
	newSnapshot := &GraphSnapshot{
		Nodes: map[string]GraphNode{
			"user_1": {ID: "user_1", Name: "alice", Val: "10", Type: "user"},
			"sub_1":  {ID: "sub_1", Name: "test", Val: "100", Type: "subreddit"},
		},
		Links: map[string]GraphLink{
			"user_1->sub_1": {Source: "user_1", Target: "sub_1"},
		},
	}

	err = CalculateAndStoreDiffs(ctx, store, version.ID, nil, newSnapshot)
	if err != nil {
		t.Fatalf("CalculateAndStoreDiffs failed: %v", err)
	}

	// Verify all diffs are "add" operations
	if len(store.diffs) != 3 {
		t.Errorf("Expected 3 diffs, got %d", len(store.diffs))
	}

	for _, diff := range store.diffs {
		if diff.Action != "add" {
			t.Errorf("Expected action 'add', got '%s'", diff.Action)
		}
		if diff.VersionID != version.ID {
			t.Errorf("Expected version_id %d, got %d", version.ID, diff.VersionID)
		}
	}
}

// Test diff calculation with node updates
func TestCalculateAndStoreDiffs_NodeUpdates(t *testing.T) {
	ctx := context.Background()
	store := newMockVersionStore()

	version, _ := store.CreateGraphVersion(ctx, db.CreateGraphVersionParams{
		NodeCount: 2,
		LinkCount: 1,
		Status:    "completed",
	})

	oldSnapshot := &GraphSnapshot{
		Nodes: map[string]GraphNode{
			"user_1": {ID: "user_1", Name: "alice", Val: "10", Type: "user"},
			"sub_1":  {ID: "sub_1", Name: "test", Val: "100", Type: "subreddit"},
		},
		Links: map[string]GraphLink{
			"user_1->sub_1": {Source: "user_1", Target: "sub_1"},
		},
	}

	newSnapshot := &GraphSnapshot{
		Nodes: map[string]GraphNode{
			"user_1": {ID: "user_1", Name: "alice", Val: "15", Type: "user"}, // Val changed
			"sub_1":  {ID: "sub_1", Name: "test", Val: "100", Type: "subreddit"},
		},
		Links: map[string]GraphLink{
			"user_1->sub_1": {Source: "user_1", Target: "sub_1"},
		},
	}

	err := CalculateAndStoreDiffs(ctx, store, version.ID, oldSnapshot, newSnapshot)
	if err != nil {
		t.Fatalf("CalculateAndStoreDiffs failed: %v", err)
	}

	// Should have 1 update for the changed node
	updateCount := 0
	for _, diff := range store.diffs {
		if diff.Action == "update" && diff.EntityType == "node" {
			updateCount++
			if diff.EntityID != "user_1" {
				t.Errorf("Expected update for user_1, got %s", diff.EntityID)
			}
		}
	}

	if updateCount != 1 {
		t.Errorf("Expected 1 node update, got %d", updateCount)
	}
}

// Test diff calculation with additions and removals
func TestCalculateAndStoreDiffs_AdditionsAndRemovals(t *testing.T) {
	ctx := context.Background()
	store := newMockVersionStore()

	version, _ := store.CreateGraphVersion(ctx, db.CreateGraphVersionParams{
		NodeCount: 3,
		LinkCount: 2,
		Status:    "completed",
	})

	oldSnapshot := &GraphSnapshot{
		Nodes: map[string]GraphNode{
			"user_1": {ID: "user_1", Name: "alice", Val: "10", Type: "user"},
			"user_2": {ID: "user_2", Name: "bob", Val: "5", Type: "user"},
		},
		Links: map[string]GraphLink{
			"user_1->sub_1": {Source: "user_1", Target: "sub_1"},
		},
	}

	newSnapshot := &GraphSnapshot{
		Nodes: map[string]GraphNode{
			"user_1": {ID: "user_1", Name: "alice", Val: "10", Type: "user"},
			"user_3": {ID: "user_3", Name: "charlie", Val: "8", Type: "user"}, // Added
		},
		Links: map[string]GraphLink{
			"user_1->sub_1": {Source: "user_1", Target: "sub_1"},
			"user_3->sub_1": {Source: "user_3", Target: "sub_1"}, // Added
		},
	}

	err := CalculateAndStoreDiffs(ctx, store, version.ID, oldSnapshot, newSnapshot)
	if err != nil {
		t.Fatalf("CalculateAndStoreDiffs failed: %v", err)
	}

	// Count additions and removals
	nodeAdds, nodeRemoves, linkAdds := 0, 0, 0
	for _, diff := range store.diffs {
		if diff.EntityType == "node" {
			if diff.Action == "add" {
				nodeAdds++
			} else if diff.Action == "remove" {
				nodeRemoves++
			}
		} else if diff.EntityType == "link" && diff.Action == "add" {
			linkAdds++
		}
	}

	if nodeAdds != 1 {
		t.Errorf("Expected 1 node addition, got %d", nodeAdds)
	}
	if nodeRemoves != 1 {
		t.Errorf("Expected 1 node removal, got %d", nodeRemoves)
	}
	if linkAdds != 1 {
		t.Errorf("Expected 1 link addition, got %d", linkAdds)
	}
}

// Test version cleanup
func TestCleanupOldVersions(t *testing.T) {
	ctx := context.Background()
	store := newMockVersionStore()

	// Set count function to simulate 15 versions
	store.countFunc = func(ctx context.Context) (int64, error) {
		return 15, nil
	}

	// Cleanup should succeed (we're mocking the actual deletion)
	err := CleanupOldVersions(ctx, store)
	if err != nil {
		t.Fatalf("CleanupOldVersions failed: %v", err)
	}
}
