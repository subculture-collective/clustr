package handlers

import (
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

func TestGetCrawlJobs_Factory(t *testing.T) {
	// Note: GetCrawlJobs requires real db.Queries to work properly.
	// Full testing requires integration tests with a real database.
	// For now, we verify the handler factory works.
	handler := GetCrawlJobs((*db.Queries)(nil))
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}
