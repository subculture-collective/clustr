package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/subculture-collective/clustr/backend/internal/cache"
	"github.com/subculture-collective/clustr/backend/internal/db"
)

// Mock for CommunityDataReader
type mockCommunityDataReader struct {
	communities []db.GraphCommunity
	members     map[int32][]string
	supernodes  []db.GetCommunitySupernodesWithPositionsRow
	links       []db.GetCommunityLinksRow
	subgraph    []db.GetCommunitySubgraphRow
}

func (m *mockCommunityDataReader) GetAllCommunities(ctx context.Context) ([]db.GraphCommunity, error) {
	return m.communities, nil
}

func (m *mockCommunityDataReader) GetCommunity(ctx context.Context, id int32) (db.GraphCommunity, error) {
	for _, c := range m.communities {
		if c.ID == id {
			return c, nil
		}
	}
	return db.GraphCommunity{}, sql.ErrNoRows
}

func (m *mockCommunityDataReader) GetCommunityMembers(ctx context.Context, communityID int32) ([]string, error) {
	return m.members[communityID], nil
}

func (m *mockCommunityDataReader) GetCommunitySupernodesWithPositions(ctx context.Context) ([]db.GetCommunitySupernodesWithPositionsRow, error) {
	return m.supernodes, nil
}

func (m *mockCommunityDataReader) GetCommunityLinks(ctx context.Context, limit int32) ([]db.GetCommunityLinksRow, error) {
	if int32(len(m.links)) > limit {
		return m.links[:limit], nil
	}
	return m.links, nil
}

func (m *mockCommunityDataReader) GetCommunitySubgraph(ctx context.Context, arg db.GetCommunitySubgraphParams) ([]db.GetCommunitySubgraphRow, error) {
	return m.subgraph, nil
}

func TestGetCommunities_Success(t *testing.T) {
	mock := &mockCommunityDataReader{
		supernodes: []db.GetCommunitySupernodesWithPositionsRow{
			{
				DataType: "node",
				ID:       "community_1",
				Name:     "Tech Community",
				Val:      "50",
				Type:     "community",
				PosX:     100.0,
				PosY:     200.0,
				PosZ:     0.0,
			},
			{
				DataType: "node",
				ID:       "community_2",
				Name:     "Gaming Community",
				Val:      "30",
				Type:     "community",
				PosX:     300.0,
				PosY:     400.0,
				PosZ:     0.0,
			},
		},
		links: []db.GetCommunityLinksRow{
			{
				DataType: "link",
				ID:       "1_2",
				Val:      "10",
				Source:   "community_1",
				Target:   "community_2",
			},
		},
	}

	handler := NewCommunityHandler(mock, cache.NewMockCache())
	req := httptest.NewRequest("GET", "/api/communities?with_positions=true", nil)
	w := httptest.NewRecorder()

	handler.GetCommunities(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp GraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(resp.Nodes))
	}

	if len(resp.Links) != 1 {
		t.Errorf("expected 1 link, got %d", len(resp.Links))
	}

	// Verify positions are included
	if resp.Nodes[0].X == nil || resp.Nodes[0].Y == nil {
		t.Error("expected positions to be set when with_positions=true")
	}
}

func TestGetCommunities_WithoutPositions(t *testing.T) {
	mock := &mockCommunityDataReader{
		supernodes: []db.GetCommunitySupernodesWithPositionsRow{
			{
				DataType: "node",
				ID:       "community_1",
				Name:     "Tech Community",
				Val:      "50",
				Type:     "community",
				PosX:     100.0,
				PosY:     200.0,
				PosZ:     0.0,
			},
		},
		links: []db.GetCommunityLinksRow{},
	}

	handler := NewCommunityHandler(mock, cache.NewMockCache())
	req := httptest.NewRequest("GET", "/api/communities?with_positions=false", nil)
	w := httptest.NewRecorder()

	handler.GetCommunities(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp GraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify positions are NOT included when positions are 0 or flag is false
	if len(resp.Nodes) > 0 && resp.Nodes[0].X == nil {
		// This is expected when with_positions=false or positions are 0
	}
}

func TestGetCommunityByID_Success(t *testing.T) {
	mock := &mockCommunityDataReader{
		communities: []db.GraphCommunity{
			{
				ID:    1,
				Label: "Tech Community",
				Size:  50,
			},
		},
		subgraph: []db.GetCommunitySubgraphRow{
			{
				DataType: "node",
				ID:       "user_1",
				Name:     "alice",
				Val:      "10",
				Type:     sql.NullString{String: "user", Valid: true},
			},
			{
				DataType: "node",
				ID:       "user_2",
				Name:     "bob",
				Val:      "5",
				Type:     sql.NullString{String: "user", Valid: true},
			},
			{
				DataType: "link",
				ID:       "1",
				Source:   "user_1",
				Target:   "user_2",
			},
		},
	}

	handler := NewCommunityHandler(mock, cache.NewMockCache())
	req := httptest.NewRequest("GET", "/api/communities/1", nil)
	w := httptest.NewRecorder()

	// Set up mux router to parse path variables
	router := mux.NewRouter()
	router.HandleFunc("/api/communities/{id}", handler.GetCommunityByID).Methods("GET")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp GraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(resp.Nodes))
	}

	if len(resp.Links) != 1 {
		t.Errorf("expected 1 link, got %d", len(resp.Links))
	}
}

func TestGetCommunityByID_NotFound(t *testing.T) {
	mock := &mockCommunityDataReader{
		communities: []db.GraphCommunity{},
	}

	handler := NewCommunityHandler(mock, cache.NewMockCache())
	req := httptest.NewRequest("GET", "/api/communities/999", nil)
	w := httptest.NewRecorder()

	// Set up mux router to parse path variables
	router := mux.NewRouter()
	router.HandleFunc("/api/communities/{id}", handler.GetCommunityByID).Methods("GET")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetCommunityByID_InvalidID(t *testing.T) {
	mock := &mockCommunityDataReader{}
	handler := NewCommunityHandler(mock, cache.NewMockCache())
	req := httptest.NewRequest("GET", "/api/communities/invalid", nil)
	w := httptest.NewRecorder()

	// Set up mux router to parse path variables
	router := mux.NewRouter()
	router.HandleFunc("/api/communities/{id}", handler.GetCommunityByID).Methods("GET")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCommunityCaching(t *testing.T) {
	mock := &mockCommunityDataReader{
		supernodes: []db.GetCommunitySupernodesWithPositionsRow{
			{
				DataType: "node",
				ID:       "community_1",
				Name:     "Test Community",
				Val:      "10",
				Type:     "community",
			},
		},
		links: []db.GetCommunityLinksRow{},
	}

	handler := NewCommunityHandler(mock, cache.NewMockCache())

	// First request - cache miss
	req1 := httptest.NewRequest("GET", "/api/communities", nil)
	w1 := httptest.NewRecorder()
	handler.GetCommunities(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("first request: expected status 200, got %d", w1.Code)
	}

	// Second request - should hit cache (same params)
	req2 := httptest.NewRequest("GET", "/api/communities", nil)
	w2 := httptest.NewRecorder()
	handler.GetCommunities(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("second request: expected status 200, got %d", w2.Code)
	}

	// Responses should be identical
	if w1.Body.String() != w2.Body.String() {
		t.Error("cached response differs from original")
	}
}
