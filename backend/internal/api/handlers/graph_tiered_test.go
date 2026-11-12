package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/cache"
	"github.com/subculture-collective/clustr/backend/internal/db"
)

// mockTieredGraphDataReader implements GraphDataReader with tiered API methods
type mockTieredGraphDataReader struct {
	MockGraphDataReader
	supernodes []db.GetCommunitySupernodesWithPositionsRow
	commLinks  []db.GetCommunityLinksRow
	bboxNodes  []db.GetNodesInBoundingBoxRow
	bboxLinks  []db.GetLinksForNodesInBoundingBoxRow
}

func (m *mockTieredGraphDataReader) GetCommunitySupernodesWithPositions(ctx context.Context) ([]db.GetCommunitySupernodesWithPositionsRow, error) {
	return m.supernodes, nil
}

func (m *mockTieredGraphDataReader) GetCommunityLinks(ctx context.Context, limit int32) ([]db.GetCommunityLinksRow, error) {
	if int32(len(m.commLinks)) > limit {
		return m.commLinks[:limit], nil
	}
	return m.commLinks, nil
}

func (m *mockTieredGraphDataReader) GetNodesInBoundingBox(ctx context.Context, arg db.GetNodesInBoundingBoxParams) ([]db.GetNodesInBoundingBoxRow, error) {
	return m.bboxNodes, nil
}

func (m *mockTieredGraphDataReader) GetLinksForNodesInBoundingBox(ctx context.Context, arg db.GetLinksForNodesInBoundingBoxParams) ([]db.GetLinksForNodesInBoundingBoxRow, error) {
	return m.bboxLinks, nil
}

func (m *mockTieredGraphDataReader) GetPaginatedGraphNodes(ctx context.Context, arg db.GetPaginatedGraphNodesParams) ([]db.GetPaginatedGraphNodesRow, error) {
	return []db.GetPaginatedGraphNodesRow{}, nil
}

func (m *mockTieredGraphDataReader) GetLinksForPaginatedNodes(ctx context.Context, arg db.GetLinksForPaginatedNodesParams) ([]db.GetLinksForPaginatedNodesRow, error) {
	return []db.GetLinksForPaginatedNodesRow{}, nil
}

func TestGetGraphOverview(t *testing.T) {
	// Setup mock data
	mock := &mockTieredGraphDataReader{
		supernodes: []db.GetCommunitySupernodesWithPositionsRow{
			{
				ID:   "community_1",
				Name: "Community 1",
				Val:  "100",
				PosX: 1.0,
				PosY: 2.0,
				PosZ: 3.0,
			},
			{
				ID:   "community_2",
				Name: "Community 2",
				Val:  "200",
				PosX: 4.0,
				PosY: 5.0,
				PosZ: 6.0,
			},
		},
		commLinks: []db.GetCommunityLinksRow{
			{
				Source: "community_1",
				Target: "community_2",
			},
		},
	}

	handler := NewHandler(mock, cache.NewMockCache())

	tests := []struct {
		name         string
		queryParams  string
		wantStatus   int
		checkNodes   bool
		wantMinNodes int
	}{
		{
			name:         "overview without params",
			queryParams:  "",
			wantStatus:   http.StatusOK,
			checkNodes:   true,
			wantMinNodes: 2,
		},
		{
			name:         "overview with positions",
			queryParams:  "?with_positions=true",
			wantStatus:   http.StatusOK,
			checkNodes:   true,
			wantMinNodes: 2,
		},
		{
			name:         "overview with custom limits",
			queryParams:  "?max_nodes=1&max_links=1",
			wantStatus:   http.StatusOK,
			checkNodes:   true,
			wantMinNodes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/graph/overview"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.GetGraphOverview(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetGraphOverview() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.checkNodes && w.Code == http.StatusOK {
				var resp GraphResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if len(resp.Nodes) < tt.wantMinNodes {
					t.Errorf("GetGraphOverview() nodes = %v, want at least %v", len(resp.Nodes), tt.wantMinNodes)
				}

				// Verify all nodes are community type
				for _, node := range resp.Nodes {
					if node.Type != "community" {
						t.Errorf("Expected node type 'community', got '%s'", node.Type)
					}
				}

				// When with_positions=true, check positions are included
				if tt.queryParams == "?with_positions=true" {
					for _, node := range resp.Nodes {
						if node.X == nil || node.Y == nil || node.Z == nil {
							t.Errorf("Expected positions for node %s when with_positions=true", node.ID)
						}
					}
				}
			}
		})
	}
}

func TestGetGraphRegion(t *testing.T) {
	// Setup mock data
	mock := &mockTieredGraphDataReader{
		bboxNodes: []db.GetNodesInBoundingBoxRow{
			{
				ID:   "node_1",
				Name: "Node 1",
				Val:  sql.NullString{String: "50", Valid: true},
				Type: sql.NullString{String: "subreddit", Valid: true},
				PosX: sql.NullFloat64{Float64: 1.5, Valid: true},
				PosY: sql.NullFloat64{Float64: 2.5, Valid: true},
				PosZ: sql.NullFloat64{Float64: 3.5, Valid: true},
			},
		},
		bboxLinks: []db.GetLinksForNodesInBoundingBoxRow{
			{
				Source: "node_1",
				Target: "node_2",
			},
		},
	}

	handler := NewHandler(mock, cache.NewMockCache())

	tests := []struct {
		name       string
		url        string
		wantStatus int
		checkBody  bool
	}{
		{
			name:       "valid bounding box",
			url:        "/api/graph/region?x_min=0&x_max=10&y_min=0&y_max=10&z_min=0&z_max=10",
			wantStatus: http.StatusOK,
			checkBody:  true,
		},
		{
			name:       "valid bounding box with limits",
			url:        "/api/graph/region?x_min=0&x_max=5&y_min=0&y_max=5&z_min=0&z_max=5&max_nodes=100&max_links=200",
			wantStatus: http.StatusOK,
			checkBody:  true,
		},
		{
			name:       "missing x_min",
			url:        "/api/graph/region?x_max=10&y_min=0&y_max=10&z_min=0&z_max=10",
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "invalid x_min",
			url:        "/api/graph/region?x_min=invalid&x_max=10&y_min=0&y_max=10&z_min=0&z_max=10",
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "invalid bounding box (min > max)",
			url:        "/api/graph/region?x_min=10&x_max=0&y_min=0&y_max=10&z_min=0&z_max=10",
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()

			handler.GetGraphRegion(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetGraphRegion() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.checkBody && w.Code == http.StatusOK {
				var resp GraphResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				// Verify nodes have positions (since spatial query requires them)
				for _, node := range resp.Nodes {
					if node.X == nil || node.Y == nil || node.Z == nil {
						t.Errorf("Expected positions for node %s in spatial query", node.ID)
					}
				}
			}
		})
	}
}

func TestGetGraphOverviewCaching(t *testing.T) {
	mock := &mockTieredGraphDataReader{
		supernodes: []db.GetCommunitySupernodesWithPositionsRow{
			{
				ID:   "community_1",
				Name: "Community 1",
				Val:  "100",
				PosX: 1.0,
				PosY: 2.0,
				PosZ: 3.0,
			},
		},
		commLinks: []db.GetCommunityLinksRow{},
	}

	mockCache := cache.NewMockCache()
	handler := NewHandler(mock, mockCache)

	// First request - should be cache miss
	req1 := httptest.NewRequest("GET", "/api/graph/overview", nil)
	w1 := httptest.NewRecorder()
	handler.GetGraphOverview(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request failed with status %v", w1.Code)
	}

	// Second request with same params - should be cache hit
	req2 := httptest.NewRequest("GET", "/api/graph/overview", nil)
	w2 := httptest.NewRecorder()
	handler.GetGraphOverview(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Second request failed with status %v", w2.Code)
	}

	// Responses should be identical (cached)
	if w1.Body.String() != w2.Body.String() {
		t.Error("Cached response differs from original")
	}
}

func TestGetGraphRegionCaching(t *testing.T) {
	mock := &mockTieredGraphDataReader{
		bboxNodes: []db.GetNodesInBoundingBoxRow{
			{
				ID:   "node_1",
				Name: "Node 1",
				Val:  sql.NullString{String: "50", Valid: true},
				Type: sql.NullString{String: "subreddit", Valid: true},
				PosX: sql.NullFloat64{Float64: 1.5, Valid: true},
				PosY: sql.NullFloat64{Float64: 2.5, Valid: true},
				PosZ: sql.NullFloat64{Float64: 3.5, Valid: true},
			},
		},
		bboxLinks: []db.GetLinksForNodesInBoundingBoxRow{},
	}

	mockCache := cache.NewMockCache()
	handler := NewHandler(mock, mockCache)

	url := "/api/graph/region?x_min=0&x_max=10&y_min=0&y_max=10&z_min=0&z_max=10"

	// First request - should be cache miss
	req1 := httptest.NewRequest("GET", url, nil)
	w1 := httptest.NewRecorder()
	handler.GetGraphRegion(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request failed with status %v", w1.Code)
	}

	// Second request with same params - should be cache hit
	req2 := httptest.NewRequest("GET", url, nil)
	w2 := httptest.NewRecorder()
	handler.GetGraphRegion(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Second request failed with status %v", w2.Code)
	}

	// Responses should be identical (cached)
	if w1.Body.String() != w2.Body.String() {
		t.Error("Cached response differs from original")
	}
}
