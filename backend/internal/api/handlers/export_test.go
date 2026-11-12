package handlers

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// mockExportDataReader implements ExportDataReader for testing
type mockExportDataReader struct {
	allRows      []db.GetPrecalculatedGraphDataCappedAllRow
	filteredRows []db.GetPrecalculatedGraphDataCappedFilteredRow
	err          error
}

func (m *mockExportDataReader) GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error) {
	return m.allRows, m.err
}

func (m *mockExportDataReader) GetPrecalculatedGraphDataCappedFiltered(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedFilteredParams) ([]db.GetPrecalculatedGraphDataCappedFilteredRow, error) {
	return m.filteredRows, m.err
}

func TestExportGraph(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		mockAllRows    []db.GetPrecalculatedGraphDataCappedAllRow
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "invalid format",
			format:         "xml",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:   "export json",
			format: "json",
			mockAllRows: []db.GetPrecalculatedGraphDataCappedAllRow{
				{
					DataType: "node",
					ID:       "user_123",
					Name:     "testuser",
					Val:      "100",
					Type:     sql.NullString{String: "user", Valid: true},
				},
				{
					DataType: "link",
					ID:       "1",
					Source:   "user_123",
					Target:   "subreddit_456",
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:   "export csv",
			format: "csv",
			mockAllRows: []db.GetPrecalculatedGraphDataCappedAllRow{
				{
					DataType: "node",
					ID:       "user_123",
					Name:     "testuser",
					Val:      "100",
					Type:     sql.NullString{String: "user", Valid: true},
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "default to json",
			format:         "",
			mockAllRows:    []db.GetPrecalculatedGraphDataCappedAllRow{},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockExportDataReader{
				allRows: tt.mockAllRows,
			}

			handler := ExportGraph(mock)
			url := "/api/export"
			if tt.format != "" {
				url += "?format=" + tt.format
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()

			handler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if !tt.expectError && rr.Code == http.StatusOK {
				format := tt.format
				if format == "" {
					format = "json"
				}

				if format == "json" {
					contentType := rr.Header().Get("Content-Type")
					if !strings.Contains(contentType, "application/json") {
						t.Errorf("expected JSON content type, got %s", contentType)
					}

					var response map[string]interface{}
					if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
						t.Fatalf("failed to decode JSON response: %v", err)
					}

					if _, ok := response["nodes"]; !ok {
						t.Error("expected nodes field in JSON response")
					}
					if _, ok := response["links"]; !ok {
						t.Error("expected links field in JSON response")
					}
				} else if format == "csv" {
					contentType := rr.Header().Get("Content-Type")
					if !strings.Contains(contentType, "text/csv") {
						t.Errorf("expected CSV content type, got %s", contentType)
					}

					reader := csv.NewReader(rr.Body)
					records, err := reader.ReadAll()
					if err != nil {
						t.Fatalf("failed to read CSV: %v", err)
					}

					if len(records) < 1 {
						t.Error("expected CSV to have at least header row")
					}

					// Check header
					header := records[0]
					expectedHeaders := []string{"data_type", "id", "name", "val", "type", "source", "target"}
					if len(header) != len(expectedHeaders) {
						t.Errorf("expected %d headers, got %d", len(expectedHeaders), len(header))
					}
				}
			}
		})
	}
}
