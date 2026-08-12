package graph

import (
	"context"
	"testing"
	"time"
)

func TestDefaultRevisionOptionsUsesPositiveEnvironmentValues(t *testing.T) {
	t.Setenv("GRAPH_REVISION_NODE_CAP", "17")
	t.Setenv("GRAPH_REVISION_LINK_CAP", "29")
	t.Setenv("GRAPH_REVISION_RETENTION", "3")
	t.Setenv("GRAPH_REVISION_SUBREDDIT_CAP", "11")
	t.Setenv("GRAPH_REVISION_USER_CAP", "5")
	t.Setenv("GRAPH_REVISION_POST_CAP", "1")
	t.Setenv("GRAPH_REVISION_COMMENT_CAP", "0")
	got := DefaultRevisionOptions()
	if got.NodeCap != 17 || got.LinkCap != 29 || got.Retention != 3 ||
		got.SubredditCap != 11 || got.UserCap != 5 || got.PostCap != 1 || got.CommentCap != 0 {
		t.Fatalf("options = %+v", got)
	}
}

func TestNonNegativeEnvAllowsZero(t *testing.T) {
	t.Setenv("GRAPH_TEST_CAP", "0")
	if got := nonNegativeEnv("GRAPH_TEST_CAP", 11); got != 0 {
		t.Fatalf("nonNegativeEnv(0) = %d", got)
	}
}

func TestPositiveEnvRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "not-a-number"} {
		t.Setenv("GRAPH_TEST_CAP", value)
		if got := positiveEnv("GRAPH_TEST_CAP", 11); got != 11 {
			t.Fatalf("positiveEnv(%q) = %d, want fallback", value, got)
		}
	}
}

func TestPublishRevisionAtWatermarkRejectsZero(t *testing.T) {
	if _, err := PublishRevisionAtWatermark(context.Background(), nil, time.Time{}); err == nil {
		t.Fatal("expected zero source watermark to be rejected")
	}
}
