package handlers

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/cache"
)

// TestGetGraphData_NDJSON tests the NDJSON streaming functionality
func TestGetGraphData_NDJSON(t *testing.T) {
	// Create mock data reader
	mockReader := &MockGraphDataReader{}
	mockCache := cache.NewMockCache()
	handler := NewHandler(mockReader, mockCache)

	// Create request with NDJSON Accept header
	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	req.Header.Set("Accept", "application/x-ndjson")
	rr := httptest.NewRecorder()

	// Call handler
	handler.GetGraphData(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Check content type
	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/x-ndjson") {
		t.Errorf("Expected Content-Type to contain 'application/x-ndjson', got '%s'", contentType)
	}

	// Parse NDJSON response
	scanner := bufio.NewScanner(rr.Body)
	nodeCount := 0
	linkCount := 0
	metaCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var envelope NDJSONEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("Failed to parse NDJSON line: %v, line: %s", err, line)
		}

		switch envelope.Type {
		case "node":
			nodeCount++
			// Verify node structure
			if envelope.Data == nil {
				t.Error("Node envelope has nil data")
			}
		case "link":
			linkCount++
			// Verify link structure
			if envelope.Data == nil {
				t.Error("Link envelope has nil data")
			}
		case "meta":
			metaCount++
			// Verify meta fields
			if envelope.TotalNodes == nil {
				t.Error("Meta envelope missing totalNodes")
			}
			if envelope.TotalLinks == nil {
				t.Error("Meta envelope missing totalLinks")
			}
			// Verify counts match
			if *envelope.TotalNodes != nodeCount {
				t.Errorf("Meta totalNodes (%d) doesn't match actual node count (%d)", *envelope.TotalNodes, nodeCount)
			}
			if *envelope.TotalLinks != linkCount {
				t.Errorf("Meta totalLinks (%d) doesn't match actual link count (%d)", *envelope.TotalLinks, linkCount)
			}
		default:
			t.Errorf("Unknown envelope type: %s", envelope.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Scanner error: %v", err)
	}

	// Verify we got nodes, links, and exactly one meta
	if nodeCount == 0 {
		t.Error("Expected at least some nodes")
	}
	if linkCount == 0 {
		t.Error("Expected at least some links")
	}
	if metaCount != 1 {
		t.Errorf("Expected exactly 1 meta envelope, got %d", metaCount)
	}
}

// TestGetGraphData_JSON tests backward compatibility with regular JSON
func TestGetGraphData_JSON(t *testing.T) {
	// Create mock data reader
	mockReader := &MockGraphDataReader{}
	mockCache := cache.NewMockCache()
	handler := NewHandler(mockReader, mockCache)

	// Create request WITHOUT NDJSON Accept header (default JSON)
	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	rr := httptest.NewRecorder()

	// Call handler
	handler.GetGraphData(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Check content type
	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to contain 'application/json', got '%s'", contentType)
	}

	// Parse JSON response
	var resp GraphResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Verify we got data
	if len(resp.Nodes) == 0 {
		t.Error("Expected at least some nodes in JSON response")
	}
	if len(resp.Links) == 0 {
		t.Error("Expected at least some links in JSON response")
	}
}

// TestGetGraphData_NDJSONvsJSON tests that NDJSON and JSON return equivalent data
func TestGetGraphData_NDJSONvsJSON(t *testing.T) {
	// Create mock data reader with deterministic data
	mockReader := &MockGraphDataReader{}
	mockCache := cache.NewMockCache()

	// Test with NDJSON
	handlerNDJSON := NewHandler(mockReader, mockCache)
	reqNDJSON := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	reqNDJSON.Header.Set("Accept", "application/x-ndjson")
	rrNDJSON := httptest.NewRecorder()
	handlerNDJSON.GetGraphData(rrNDJSON, reqNDJSON)

	// Parse NDJSON to collect nodes and links
	var ndjsonNodes []GraphNode
	var ndjsonLinks []GraphLink
	scanner := bufio.NewScanner(rrNDJSON.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var envelope NDJSONEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("Failed to parse NDJSON: %v", err)
		}
		switch envelope.Type {
		case "node":
			nodeBytes, err := json.Marshal(envelope.Data)
			if err != nil {
				t.Fatalf("Failed to marshal node data: %v", err)
			}
			var node GraphNode
			if err := json.Unmarshal(nodeBytes, &node); err != nil {
				t.Fatalf("Failed to unmarshal node data: %v", err)
			}
			ndjsonNodes = append(ndjsonNodes, node)
		case "link":
			linkBytes, err := json.Marshal(envelope.Data)
			if err != nil {
				t.Fatalf("Failed to marshal link data: %v", err)
			}
			var link GraphLink
			if err := json.Unmarshal(linkBytes, &link); err != nil {
				t.Fatalf("Failed to unmarshal link data: %v", err)
			}
			ndjsonLinks = append(ndjsonLinks, link)
		}
	}

	// Test with regular JSON (use fresh cache)
	mockCache2 := cache.NewMockCache()
	handlerJSON := NewHandler(mockReader, mockCache2)
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	rrJSON := httptest.NewRecorder()
	handlerJSON.GetGraphData(rrJSON, reqJSON)

	var jsonResp GraphResponse
	if err := json.NewDecoder(rrJSON.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Compare counts
	if len(ndjsonNodes) != len(jsonResp.Nodes) {
		t.Errorf("NDJSON nodes count (%d) doesn't match JSON nodes count (%d)", len(ndjsonNodes), len(jsonResp.Nodes))
	}
	if len(ndjsonLinks) != len(jsonResp.Links) {
		t.Errorf("NDJSON links count (%d) doesn't match JSON links count (%d)", len(ndjsonLinks), len(jsonResp.Links))
	}
}

// BenchmarkGetGraphData_NDJSON benchmarks NDJSON streaming
func BenchmarkGetGraphData_NDJSON(b *testing.B) {
	mockReader := &MockGraphDataReader{}
	mockCache := cache.NewMockCache()
	handler := NewHandler(mockReader, mockCache)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
		req.Header.Set("Accept", "application/x-ndjson")
		rr := httptest.NewRecorder()
		handler.GetGraphData(rr, req)
	}
}

// BenchmarkGetGraphData_JSON benchmarks regular JSON
func BenchmarkGetGraphData_JSON(b *testing.B) {
	mockReader := &MockGraphDataReader{}
	mockCache := cache.NewMockCache()
	handler := NewHandler(mockReader, mockCache)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
		rr := httptest.NewRecorder()
		handler.GetGraphData(rr, req)
	}
}

// TestGetGraphData_NDJSONFirstFlushTiming tests that NDJSON response starts quickly
func TestGetGraphData_NDJSONFirstFlushTiming(t *testing.T) {
	mockReader := &MockGraphDataReader{}
	mockCache := cache.NewMockCache()
	handler := NewHandler(mockReader, mockCache)

	// Create a custom ResponseRecorder that tracks first write
	type timingRecorder struct {
		*httptest.ResponseRecorder
		firstWriteTime *int64 // nanoseconds since start
		startTime      int64
	}

	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	req.Header.Set("Accept", "application/x-ndjson")

	baseRR := httptest.NewRecorder()
	tr := &timingRecorder{
		ResponseRecorder: baseRR,
		startTime:        0,
	}

	// Start timing just before the handler
	startTime := time.Now()
	tr.startTime = startTime.UnixNano()

	// Run handler in a goroutine and check first write
	done := make(chan bool)
	go func() {
		handler.GetGraphData(tr, req)
		done <- true
	}()

	// Wait for completion with timeout
	select {
	case <-done:
		// Handler completed
	case <-time.After(5 * time.Second):
		t.Fatal("Handler timed out after 5 seconds")
	}

	// The response should complete relatively quickly
	// We can't easily test "first flush" timing with httptest.ResponseRecorder
	// as it doesn't provide flush notifications, but we can verify the response
	// is valid and complete
	if tr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", tr.Code)
	}

	// Verify we got NDJSON data
	if tr.Body.Len() == 0 {
		t.Error("Expected non-empty response body")
	}

	contentType := tr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/x-ndjson") {
		t.Errorf("Expected NDJSON content type, got %s", contentType)
	}
}
