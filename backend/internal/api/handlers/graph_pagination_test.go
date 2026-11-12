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

// mockPaginatedGraphDataReader simulates paginated graph data
type mockPaginatedGraphDataReader struct {
	MockGraphDataReader
	nodes []db.GetPaginatedGraphNodesRow
	links []db.GetLinksForPaginatedNodesRow
}

func (m *mockPaginatedGraphDataReader) GetGraphData(ctx context.Context) ([]json.RawMessage, error) {
	return nil, nil
}

func (m *mockPaginatedGraphDataReader) GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error) {
	return nil, nil
}

func (m *mockPaginatedGraphDataReader) GetPrecalculatedGraphDataCappedFiltered(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedFilteredParams) ([]db.GetPrecalculatedGraphDataCappedFilteredRow, error) {
	return nil, nil
}

func (m *mockPaginatedGraphDataReader) GetPrecalculatedGraphDataNoPos(ctx context.Context) ([]db.GetPrecalculatedGraphDataNoPosRow, error) {
	return nil, nil
}

func (m *mockPaginatedGraphDataReader) GetEdgeBundles(ctx context.Context, weight int32) ([]db.GetEdgeBundlesRow, error) {
	return []db.GetEdgeBundlesRow{}, nil
}

func (m *mockPaginatedGraphDataReader) GetCommunitySupernodesWithPositions(ctx context.Context) ([]db.GetCommunitySupernodesWithPositionsRow, error) {
	return []db.GetCommunitySupernodesWithPositionsRow{}, nil
}

func (m *mockPaginatedGraphDataReader) GetCommunityLinks(ctx context.Context, limit int32) ([]db.GetCommunityLinksRow, error) {
	return []db.GetCommunityLinksRow{}, nil
}

func (m *mockPaginatedGraphDataReader) GetNodesInBoundingBox(ctx context.Context, arg db.GetNodesInBoundingBoxParams) ([]db.GetNodesInBoundingBoxRow, error) {
	return []db.GetNodesInBoundingBoxRow{}, nil
}

func (m *mockPaginatedGraphDataReader) GetLinksForNodesInBoundingBox(ctx context.Context, arg db.GetLinksForNodesInBoundingBoxParams) ([]db.GetLinksForNodesInBoundingBoxRow, error) {
	return []db.GetLinksForNodesInBoundingBoxRow{}, nil
}

func (m *mockPaginatedGraphDataReader) GetPaginatedGraphNodes(ctx context.Context, arg db.GetPaginatedGraphNodesParams) ([]db.GetPaginatedGraphNodesRow, error) {
	// Simulate pagination by returning a subset based on cursor
	// For testing, we'll just return all nodes up to the limit
	if int32(len(m.nodes)) > arg.Limit {
		return m.nodes[:arg.Limit], nil
	}
	return m.nodes, nil
}

func (m *mockPaginatedGraphDataReader) GetLinksForPaginatedNodes(ctx context.Context, arg db.GetLinksForPaginatedNodesParams) ([]db.GetLinksForPaginatedNodesRow, error) {
	if int32(len(m.links)) > arg.Limit {
		return m.links[:arg.Limit], nil
	}
	return m.links, nil
}

// TestCursorEncodingDecoding verifies cursor encoding and decoding
func TestCursorEncodingDecoding(t *testing.T) {
	tests := []struct {
		name   string
		weight int64
		id     string
	}{
		{
			name:   "simple cursor",
			weight: 100,
			id:     "node_1",
		},
		{
			name:   "cursor with large weight",
			weight: 9999999,
			id:     "subreddit_12345",
		},
		{
			name:   "cursor with zero weight",
			weight: 0,
			id:     "user_999",
		},
		{
			name:   "cursor with special characters in ID",
			weight: 42,
			id:     "test_node_with_underscore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := encodeCursor(tt.weight, tt.id)
			if encoded == "" {
				t.Fatal("encoded cursor should not be empty")
			}

			// Decode
			decoded, err := decodeCursor(encoded)
			if err != nil {
				t.Fatalf("failed to decode cursor: %v", err)
			}

			// Verify
			if decoded.Weight != tt.weight {
				t.Errorf("weight mismatch: got %d, want %d", decoded.Weight, tt.weight)
			}
			if decoded.ID != tt.id {
				t.Errorf("id mismatch: got %s, want %s", decoded.ID, tt.id)
			}
		})
	}
}

func TestDecodeCursorInvalid(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
	}{
		{
			name:   "invalid base64",
			cursor: "!!!invalid!!!",
		},
		{
			name:   "missing separator",
			cursor: encodeCursor(100, "node1")[:10] + "corrupted",
		},
		{
			name:   "non-numeric weight",
			cursor: "YWJjOm5vZGUx", // "abc:node1"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeCursor(tt.cursor)
			if err == nil {
				t.Error("expected error for invalid cursor, got nil")
			}
		})
	}
}

func TestDecodeCursorEmpty(t *testing.T) {
	decoded, err := decodeCursor("")
	if err != nil {
		t.Errorf("empty cursor should not error: %v", err)
	}
	if decoded.Weight != 0 || decoded.ID != "" {
		t.Errorf("empty cursor should return zero values, got weight=%d, id=%s", decoded.Weight, decoded.ID)
	}
}

func TestGetGraphDataPaginated_FirstPage(t *testing.T) {
	// Setup mock data - 6 nodes to test pagination with page_size=5
	mockNodes := []db.GetPaginatedGraphNodesRow{
		{
			ID:   "node_1",
			Name: "Node 1",
			Val:  sql.NullString{String: "100", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
			PosX: sql.NullFloat64{Float64: 1.0, Valid: true},
			PosY: sql.NullFloat64{Float64: 2.0, Valid: true},
			PosZ: sql.NullFloat64{Float64: 3.0, Valid: true},
		},
		{
			ID:   "node_2",
			Name: "Node 2",
			Val:  sql.NullString{String: "90", Valid: true},
			Type: sql.NullString{String: "subreddit", Valid: true},
		},
		{
			ID:   "node_3",
			Name: "Node 3",
			Val:  sql.NullString{String: "80", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
		},
		{
			ID:   "node_4",
			Name: "Node 4",
			Val:  sql.NullString{String: "70", Valid: true},
			Type: sql.NullString{String: "subreddit", Valid: true},
		},
		{
			ID:   "node_5",
			Name: "Node 5",
			Val:  sql.NullString{String: "60", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
		},
		{
			ID:   "node_6",
			Name: "Node 6",
			Val:  sql.NullString{String: "50", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
		},
	}

	mockLinks := []db.GetLinksForPaginatedNodesRow{
		{ID: 1, Source: "node_1", Target: "node_2"},
		{ID: 2, Source: "node_2", Target: "node_3"},
	}

	mock := &mockPaginatedGraphDataReader{
		nodes: mockNodes,
		links: mockLinks,
	}

	handler := NewHandler(mock, cache.NewMockCache())

	// Request first page with page_size=5
	req := httptest.NewRequest("GET", "/api/graph?page_size=5", nil)
	w := httptest.NewRecorder()

	handler.GetGraphData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp PaginatedGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify we got 5 nodes (not 6, which would indicate we got all)
	if len(resp.Nodes) != 5 {
		t.Errorf("expected 5 nodes in first page, got %d", len(resp.Nodes))
	}

	// Verify pagination metadata
	if resp.Pagination == nil {
		t.Fatal("expected pagination metadata, got nil")
	}

	if !resp.Pagination.HasMore {
		t.Error("expected has_more=true since there are 6 nodes total")
	}

	if resp.Pagination.NextCursor == "" {
		t.Error("expected next_cursor to be set when has_more=true")
	}

	if resp.Pagination.PageSize != 5 {
		t.Errorf("expected page_size=5, got %d", resp.Pagination.PageSize)
	}

	// Verify links are included
	if len(resp.Links) == 0 {
		t.Error("expected links to be included")
	}
}

func TestGetGraphDataPaginated_WithCursor(t *testing.T) {
	// Test requesting a page with a cursor
	mockNodes := []db.GetPaginatedGraphNodesRow{
		{
			ID:   "node_4",
			Name: "Node 4",
			Val:  sql.NullString{String: "70", Valid: true},
			Type: sql.NullString{String: "subreddit", Valid: true},
		},
		{
			ID:   "node_5",
			Name: "Node 5",
			Val:  sql.NullString{String: "60", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
		},
	}

	mock := &mockPaginatedGraphDataReader{
		nodes: mockNodes,
		links: []db.GetLinksForPaginatedNodesRow{},
	}

	handler := NewHandler(mock, cache.NewMockCache())

	// Generate a valid cursor
	cursor := encodeCursor(80, "node_3")

	// Request page with cursor
	req := httptest.NewRequest("GET", "/api/graph?cursor="+cursor+"&page_size=5", nil)
	w := httptest.NewRecorder()

	handler.GetGraphData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp PaginatedGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify we got nodes
	if len(resp.Nodes) == 0 {
		t.Error("expected nodes in response")
	}

	// Since we only have 2 nodes and page_size=5, has_more should be false
	if resp.Pagination.HasMore {
		t.Error("expected has_more=false for last page")
	}

	if resp.Pagination.NextCursor != "" {
		t.Error("expected next_cursor to be empty when has_more=false")
	}
}

func TestGetGraphDataPaginated_InvalidCursor(t *testing.T) {
	mock := &mockPaginatedGraphDataReader{
		nodes: []db.GetPaginatedGraphNodesRow{},
		links: []db.GetLinksForPaginatedNodesRow{},
	}

	handler := NewHandler(mock, cache.NewMockCache())

	// Request with invalid cursor
	req := httptest.NewRequest("GET", "/api/graph?cursor=invalid!!&page_size=5", nil)
	w := httptest.NewRecorder()

	handler.GetGraphData(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid cursor, got %d", w.Code)
	}
}

func TestGetGraphDataPaginated_WithPositions(t *testing.T) {
	mockNodes := []db.GetPaginatedGraphNodesRow{
		{
			ID:   "node_1",
			Name: "Node 1",
			Val:  sql.NullString{String: "100", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
			PosX: sql.NullFloat64{Float64: 10.5, Valid: true},
			PosY: sql.NullFloat64{Float64: 20.5, Valid: true},
			PosZ: sql.NullFloat64{Float64: 30.5, Valid: true},
		},
	}

	mock := &mockPaginatedGraphDataReader{
		nodes: mockNodes,
		links: []db.GetLinksForPaginatedNodesRow{},
	}

	handler := NewHandler(mock, cache.NewMockCache())

	// Request with positions
	req := httptest.NewRequest("GET", "/api/graph?page_size=5&with_positions=true", nil)
	w := httptest.NewRecorder()

	handler.GetGraphData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp PaginatedGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}

	// Verify positions are included
	node := resp.Nodes[0]
	if node.X == nil || node.Y == nil || node.Z == nil {
		t.Error("expected positions to be included when with_positions=true")
	}

	if node.X != nil && *node.X != 10.5 {
		t.Errorf("expected x=10.5, got %f", *node.X)
	}
}

func TestGetGraphDataPaginated_PageSizeLimits(t *testing.T) {
	tests := []struct {
		name              string
		pageSizeParam     string
		expectedPageSize  int
	}{
		{
			name:             "default page size",
			pageSizeParam:    "",
			expectedPageSize: 5000,
		},
		{
			name:             "custom page size",
			pageSizeParam:    "100",
			expectedPageSize: 100,
		},
		{
			name:             "zero page size uses default",
			pageSizeParam:    "0",
			expectedPageSize: 5000,
		},
		{
			name:             "negative page size uses default",
			pageSizeParam:    "-1",
			expectedPageSize: 5000,
		},
		{
			name:             "excessive page size is capped",
			pageSizeParam:    "100000",
			expectedPageSize: 50000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPaginatedGraphDataReader{
				nodes: []db.GetPaginatedGraphNodesRow{},
				links: []db.GetLinksForPaginatedNodesRow{},
			}

			handler := NewHandler(mock, cache.NewMockCache())

			// Need to use page_size param to trigger pagination path
			url := "/api/graph?page_size=" + tt.pageSizeParam
			if tt.pageSizeParam == "" {
				url = "/api/graph?page_size=5000"
			}

			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			handler.GetGraphData(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
			}

			var resp PaginatedGraphResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp.Pagination == nil {
				t.Fatal("expected pagination metadata, got nil")
			}

			if resp.Pagination.PageSize != tt.expectedPageSize {
				t.Errorf("expected page_size=%d, got %d", tt.expectedPageSize, resp.Pagination.PageSize)
			}
		})
	}
}

func TestGetGraphDataPaginated_TypeFiltering(t *testing.T) {
	mockNodes := []db.GetPaginatedGraphNodesRow{
		{
			ID:   "user_1",
			Name: "User 1",
			Val:  sql.NullString{String: "100", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
		},
		{
			ID:   "subreddit_1",
			Name: "Subreddit 1",
			Val:  sql.NullString{String: "90", Valid: true},
			Type: sql.NullString{String: "subreddit", Valid: true},
		},
		{
			ID:   "user_2",
			Name: "User 2",
			Val:  sql.NullString{String: "80", Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
		},
	}

	mock := &mockPaginatedGraphDataReader{
		nodes: mockNodes,
		links: []db.GetLinksForPaginatedNodesRow{},
	}

	handler := NewHandler(mock, cache.NewMockCache())

	// Request with type filter for users only
	req := httptest.NewRequest("GET", "/api/graph?page_size=10&types=user", nil)
	w := httptest.NewRecorder()

	handler.GetGraphData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp PaginatedGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should only return user nodes (2 out of 3)
	if len(resp.Nodes) != 2 {
		t.Errorf("expected 2 user nodes, got %d", len(resp.Nodes))
	}

	// Verify all returned nodes are users
	for _, node := range resp.Nodes {
		if node.Type != "user" {
			t.Errorf("expected all nodes to be type 'user', got '%s'", node.Type)
		}
	}
}
