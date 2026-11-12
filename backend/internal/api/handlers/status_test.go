package handlers

import (
	"testing"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

func TestGetCrawlStatus_Factory(t *testing.T) {
	// Note: This test is limited because GetCrawlStatus requires real DB access
	// with QueryContext for custom queries. Full testing requires integration tests.
	// For now, we verify the handler factory works.
	handler := GetCrawlStatus((*db.Queries)(nil))
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}
