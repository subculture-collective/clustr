package crawler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrawlLifecycleMigrationBackfillsEverySubredditWithSpreadDueTimes(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "000028_crawl_lifecycle.up.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sql := string(body)
	for _, required := range []string{
		"FROM subreddits s",
		"LEFT JOIN crawl_jobs j ON j.subreddit_id = s.id",
		"activation_cohort TEXT NOT NULL DEFAULT 'active'",
		"THEN 'active' ELSE 'backlog' END",
		"lower(COALESCE(j.enqueued_by, '')) IN ('api', 'manual')",
		"hashtext('crawl-request:' || s.id::text)",
		"ON CONFLICT (subreddit_id) DO NOTHING",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration must contain %q", required)
		}
	}
}
