package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// mockNodeSearcher implements NodeSearcher for testing
type mockNodeSearcher struct {
	results []db.SearchGraphNodesRow
	err     error
}

func (m *mockNodeSearcher) SearchGraphNodes(ctx context.Context, arg db.SearchGraphNodesParams) ([]db.SearchGraphNodesRow, error) {
	return m.results, m.err
}

func TestSearchNode(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		limit          string
		mockResults    []db.SearchGraphNodesRow
		mockErr        error
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "missing node parameter",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:  "successful search",
			query: "test",
			limit: "10",
			mockResults: []db.SearchGraphNodesRow{
				{
					ID:   "user_123",
					Name: "testuser",
					Val:  "100",
					Type: sql.NullString{String: "user", Valid: true},
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "search with default limit",
			query:          "test",
			mockResults:    []db.SearchGraphNodesRow{},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "database error",
			query:          "test",
			mockErr:        sql.ErrConnDone,
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNodeSearcher{
				results: tt.mockResults,
				err:     tt.mockErr,
			}

			handler := SearchNode(mock)
			req := httptest.NewRequest(http.MethodGet, "/api/search?node="+tt.query+"&limit="+tt.limit, nil)
			rr := httptest.NewRecorder()

			handler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if !tt.expectError && rr.Code == http.StatusOK {
				var response map[string]interface{}
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if response["query"] != tt.query {
					t.Errorf("expected query %s, got %v", tt.query, response["query"])
				}

				count, ok := response["count"].(float64)
				if !ok {
					t.Fatalf("count is not a number")
				}
				if int(count) != len(tt.mockResults) {
					t.Errorf("expected count %d, got %d", len(tt.mockResults), int(count))
				}
			}
		})
	}
}
