package handlers

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/cache"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"net/http/httptest"
)

// mockLargeGraphDataReader simulates a realistic graph with many nodes
type mockLargeGraphDataReader struct {
	MockGraphDataReader
	nodeCount int
}

func (m *mockLargeGraphDataReader) GetPaginatedGraphNodes(ctx context.Context, arg db.GetPaginatedGraphNodesParams) ([]db.GetPaginatedGraphNodesRow, error) {
	// Generate nodes on the fly to simulate database query
	limit := int(arg.Limit)
	if limit > m.nodeCount {
		limit = m.nodeCount
	}
	
	nodes := make([]db.GetPaginatedGraphNodesRow, limit)
	for i := 0; i < limit; i++ {
		weight := m.nodeCount - i // Descending weight
		nodes[i] = db.GetPaginatedGraphNodesRow{
			ID:   "node_" + strconv.Itoa(i),
			Name: "Node " + strconv.Itoa(i),
			Val:  sql.NullString{String: strconv.Itoa(weight), Valid: true},
			Type: sql.NullString{String: "user", Valid: true},
			PosX: sql.NullFloat64{Float64: float64(i), Valid: true},
			PosY: sql.NullFloat64{Float64: float64(i) * 2, Valid: true},
			PosZ: sql.NullFloat64{Float64: float64(i) * 3, Valid: true},
		}
	}
	
	return nodes, nil
}

func (m *mockLargeGraphDataReader) GetLinksForPaginatedNodes(ctx context.Context, arg db.GetLinksForPaginatedNodesParams) ([]db.GetLinksForPaginatedNodesRow, error) {
	// Generate some links
	nodeIDs := arg.Column1
	if len(nodeIDs) == 0 {
		return []db.GetLinksForPaginatedNodesRow{}, nil
	}
	
	// Create links between sequential nodes
	links := make([]db.GetLinksForPaginatedNodesRow, 0, len(nodeIDs)-1)
	for i := 0; i < len(nodeIDs)-1; i++ {
		links = append(links, db.GetLinksForPaginatedNodesRow{
			ID:     int32(i),
			Source: nodeIDs[i],
			Target: nodeIDs[i+1],
		})
	}
	
	return links, nil
}

// BenchmarkPaginatedGraphAPI benchmarks the paginated graph API
func BenchmarkPaginatedGraphAPI(b *testing.B) {
	tests := []struct {
		name      string
		pageSize  int
		nodeCount int
	}{
		{
			name:      "small_page_1000_nodes",
			pageSize:  1000,
			nodeCount: 10000,
		},
		{
			name:      "medium_page_5000_nodes",
			pageSize:  5000,
			nodeCount: 50000,
		},
		{
			name:      "large_page_10000_nodes",
			pageSize:  10000,
			nodeCount: 100000,
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			mock := &mockLargeGraphDataReader{
				nodeCount: tt.nodeCount,
			}
			handler := NewHandler(mock, cache.NewMockCache())

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("GET", "/api/graph?page_size="+strconv.Itoa(tt.pageSize), nil)
				w := httptest.NewRecorder()
				handler.GetGraphData(w, req)
			}
		})
	}
}

// BenchmarkCursorEncoding benchmarks cursor encoding/decoding operations
func BenchmarkCursorEncoding(b *testing.B) {
	testData := []struct {
		weight int64
		id     string
	}{
		{100, "node_123"},
		{999999, "subreddit_very_long_name_12345"},
		{0, "short"},
	}

	b.Run("encode", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data := testData[i%len(testData)]
			_ = encodeCursor(data.weight, data.id)
		}
	})

	b.Run("decode", func(b *testing.B) {
		// Pre-encode cursors
		cursors := make([]string, len(testData))
		for i, data := range testData {
			cursors[i] = encodeCursor(data.weight, data.id)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cursor := cursors[i%len(cursors)]
			_, _ = decodeCursor(cursor)
		}
	})
}

// BenchmarkPaginationWithCursor benchmarks pagination with cursor (subsequent pages)
func BenchmarkPaginationWithCursor(b *testing.B) {
	mock := &mockLargeGraphDataReader{
		nodeCount: 100000,
	}
	handler := NewHandler(mock, cache.NewMockCache())

	// Generate a valid cursor
	cursor := encodeCursor(50000, "node_50000")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/graph?page_size=5000&cursor="+cursor, nil)
		w := httptest.NewRecorder()
		handler.GetGraphData(w, req)
	}
}
