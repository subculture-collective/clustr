package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/cache"
	"github.com/subculture-collective/clustr/backend/internal/db"
)

// MockGraphDataReader implements GraphDataReader for benchmarking
type MockGraphDataReader struct{}

func (m *MockGraphDataReader) GetGraphData(ctx context.Context) ([]json.RawMessage, error) {
	// Return empty data for benchmarking
	return []json.RawMessage{}, nil
}

func (m *MockGraphDataReader) GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error) {
	// Generate mock nodes and links - pre-allocate for both
	nodes := make([]db.GetPrecalculatedGraphDataCappedAllRow, 0, 2000)
	for i := 0; i < 1000; i++ {
		node := db.GetPrecalculatedGraphDataCappedAllRow{
			DataType: "node",
			ID:       "node_" + strconv.Itoa(i),
			Name:     "Node " + strconv.Itoa(i),
			Val:      "100",
			Type:     sql.NullString{String: "subreddit", Valid: true},
		}
		nodes = append(nodes, node)
	}

	// Generate mock links
	for i := 1000; i < 2000; i++ {
		link := db.GetPrecalculatedGraphDataCappedAllRow{
			DataType: "link",
			Source:   "node_" + strconv.Itoa(i%1000),
			Target:   "node_" + strconv.Itoa((i+1)%1000),
		}
		nodes = append(nodes, link)
	}

	return nodes, nil
}

func (m *MockGraphDataReader) GetPrecalculatedGraphDataCappedFiltered(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedFilteredParams) ([]db.GetPrecalculatedGraphDataCappedFilteredRow, error) {
	// Generate mock filtered data
	nodes := make([]db.GetPrecalculatedGraphDataCappedFilteredRow, 500)
	for i := 0; i < 500; i++ {
		node := db.GetPrecalculatedGraphDataCappedFilteredRow{
			DataType: "node",
			ID:       "node_" + strconv.Itoa(i),
			Name:     "Node " + strconv.Itoa(i),
			Val:      "50",
			Type:     sql.NullString{String: "user", Valid: true},
		}
		nodes[i] = node
	}
	return nodes, nil
}

func (m *MockGraphDataReader) GetPrecalculatedGraphDataNoPos(ctx context.Context) ([]db.GetPrecalculatedGraphDataNoPosRow, error) {
	nodes := make([]db.GetPrecalculatedGraphDataNoPosRow, 500)
	for i := 0; i < 500; i++ {
		node := db.GetPrecalculatedGraphDataNoPosRow{
			DataType: "node",
			ID:       "node_" + strconv.Itoa(i),
			Name:     "Node " + strconv.Itoa(i),
			Val:      "25",
			Type:     sql.NullString{String: "post", Valid: true},
		}
		nodes[i] = node
	}
	return nodes, nil
}

func (m *MockGraphDataReader) GetEdgeBundles(ctx context.Context, weight int32) ([]db.GetEdgeBundlesRow, error) {
	return []db.GetEdgeBundlesRow{}, nil
}

func (m *MockGraphDataReader) GetCommunitySupernodesWithPositions(ctx context.Context) ([]db.GetCommunitySupernodesWithPositionsRow, error) {
	return []db.GetCommunitySupernodesWithPositionsRow{}, nil
}

func (m *MockGraphDataReader) GetCommunityLinks(ctx context.Context, limit int32) ([]db.GetCommunityLinksRow, error) {
	return []db.GetCommunityLinksRow{}, nil
}

func (m *MockGraphDataReader) GetNodesInBoundingBox(ctx context.Context, arg db.GetNodesInBoundingBoxParams) ([]db.GetNodesInBoundingBoxRow, error) {
	return []db.GetNodesInBoundingBoxRow{}, nil
}

func (m *MockGraphDataReader) GetLinksForNodesInBoundingBox(ctx context.Context, arg db.GetLinksForNodesInBoundingBoxParams) ([]db.GetLinksForNodesInBoundingBoxRow, error) {
	return []db.GetLinksForNodesInBoundingBoxRow{}, nil
}

func (m *MockGraphDataReader) GetPaginatedGraphNodes(ctx context.Context, arg db.GetPaginatedGraphNodesParams) ([]db.GetPaginatedGraphNodesRow, error) {
	return []db.GetPaginatedGraphNodesRow{}, nil
}

func (m *MockGraphDataReader) GetLinksForPaginatedNodes(ctx context.Context, arg db.GetLinksForPaginatedNodesParams) ([]db.GetLinksForPaginatedNodesRow, error) {
	return []db.GetLinksForPaginatedNodesRow{}, nil
}

// BenchmarkGetGraphData benchmarks the main graph data endpoint
func BenchmarkGetGraphData(b *testing.B) {
	handler := NewHandler(&MockGraphDataReader{}, cache.NewMockCache())

	b.Run("DefaultParams", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/api/graph", nil)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			handler.GetGraphData(w, req)
		}
	})

	b.Run("WithMaxNodes", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/api/graph?max_nodes=10000", nil)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			handler.GetGraphData(w, req)
		}
	})

	b.Run("WithTypeFilter", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/api/graph?types=subreddit,user", nil)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			handler.GetGraphData(w, req)
		}
	})

	b.Run("WithPositions", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/api/graph?with_positions=true", nil)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			handler.GetGraphData(w, req)
		}
	})

	b.Run("WithAllParams", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/api/graph?max_nodes=15000&max_links=40000&types=subreddit&with_positions=true", nil)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			handler.GetGraphData(w, req)
		}
	})
}

// BenchmarkGraphResponseSerialization benchmarks JSON serialization of graph data
func BenchmarkGraphResponseSerialization(b *testing.B) {
	// Create sample graph data
	nodes := make([]GraphNode, 10000)
	for i := 0; i < 10000; i++ {
		nodes[i] = GraphNode{
			ID:   "node_" + strconv.Itoa(i),
			Name: "Test Node",
			Val:  100,
			Type: "subreddit",
		}
	}

	links := make([]GraphLink, 20000)
	for i := 0; i < 20000; i++ {
		links[i] = GraphLink{
			Source: "node_" + strconv.Itoa(i%10000),
			Target: "node_" + strconv.Itoa((i+1)%10000),
		}
	}

	resp := GraphResponse{
		Nodes: nodes,
		Links: links,
	}

	b.Run("JSONMarshal", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := json.Marshal(resp)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSONEncoder", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			encoder := json.NewEncoder(w)
			err := encoder.Encode(resp)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCacheKey benchmarks the cache key generation
func BenchmarkCacheKey(b *testing.B) {
	b.Run("AllTypes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cacheKey(20000, 50000, "", false)
		}
	})

	b.Run("WithTypes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cacheKey(20000, 50000, "subreddit,user", false)
		}
	})

	b.Run("WithPositions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cacheKey(20000, 50000, "subreddit", true)
		}
	})
}
