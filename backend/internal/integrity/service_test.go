package integrity

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestNewService verifies service creation
func TestNewService(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	svc := NewService(db)
	if svc == nil {
		t.Fatal("Expected service to be non-nil")
	}
	if svc.queries == nil {
		t.Fatal("Expected queries to be non-nil")
	}
	if svc.db == nil {
		t.Fatal("Expected db to be non-nil")
	}
}

// TestCheckAllIntegrity verifies check execution
func TestCheckAllIntegrity(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	svc := NewService(db)
	ctx := context.Background()

	results, err := svc.CheckAllIntegrity(ctx, 10, 0)
	if err != nil {
		t.Fatalf("Failed to run integrity checks: %v", err)
	}

	// Verify we get all expected checks
	expectedChecks := []string{
		"orphan_posts",
		"orphan_comments",
		"dangling_graph_links",
		"orphan_graph_nodes",
		"invalid_comment_parents",
	}

	if len(results) != len(expectedChecks) {
		t.Errorf("Expected %d checks, got %d", len(expectedChecks), len(results))
	}

	for i, result := range results {
		if result.CheckName != expectedChecks[i] {
			t.Errorf("Expected check %s at position %d, got %s", expectedChecks[i], i, result.CheckName)
		}
		if result.CheckedAt.IsZero() {
			t.Error("Expected CheckedAt to be set")
		}
		if result.Details == "" {
			t.Error("Expected Details to be non-empty")
		}
	}
}

// TestCheckResult verifies CheckResult struct
func TestCheckResult(t *testing.T) {
	result := CheckResult{
		CheckName:  "test_check",
		IssueCount: 5,
		Details:    "Test details",
		HasIssues:  true,
	}

	if result.CheckName != "test_check" {
		t.Errorf("Expected CheckName to be 'test_check', got %s", result.CheckName)
	}
	if result.IssueCount != 5 {
		t.Errorf("Expected IssueCount to be 5, got %d", result.IssueCount)
	}
	if !result.HasIssues {
		t.Error("Expected HasIssues to be true")
	}
}

// TestDatabaseStats verifies DatabaseStats struct
func TestDatabaseStats(t *testing.T) {
	stats := DatabaseStats{
		TableName: "test_table",
		Size:      "1 MB",
		RowCount:  1000,
		DeadRows:  50,
	}

	if stats.TableName != "test_table" {
		t.Errorf("Expected TableName to be 'test_table', got %s", stats.TableName)
	}
	if stats.RowCount != 1000 {
		t.Errorf("Expected RowCount to be 1000, got %d", stats.RowCount)
	}
}
