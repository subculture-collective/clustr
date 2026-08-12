package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// ensure defaults kick in with empty env
	os.Unsetenv("REDDIT_USER_AGENT")
	os.Unsetenv("HTTP_MAX_RETRIES")
	os.Unsetenv("HTTP_RETRY_BASE_MS")
	os.Unsetenv("DETAILED_GRAPH")
	os.Unsetenv("POSTS_PER_SUB_IN_GRAPH")
	os.Unsetenv("COMMENTS_PER_POST_IN_GRAPH")
	os.Unsetenv("MAX_AUTHOR_CONTENT_LINKS")

	cfg := Load()
	if cfg.UserAgent == "" {
		t.Fatalf("expected default UA, got empty")
	}
	if cfg.HTTPMaxRetries != 3 {
		t.Fatalf("expected default retries=3, got %d", cfg.HTTPMaxRetries)
	}
	if cfg.PostsPerSubInGraph != 10 || cfg.CommentsPerPost != 50 {
		t.Fatalf("unexpected defaults: posts=%d comments=%d", cfg.PostsPerSubInGraph, cfg.CommentsPerPost)
	}
	if cfg.MaxAuthorLinks != 3 {
		t.Fatalf("expected default MaxAuthorLinks=3, got %d", cfg.MaxAuthorLinks)
	}
}

func TestLayoutConfigDefaults(t *testing.T) {
	// Clear any existing layout env vars
	os.Unsetenv("LAYOUT_MAX_NODES")
	os.Unsetenv("LAYOUT_ITERATIONS")
	os.Unsetenv("LAYOUT_BATCH_SIZE")
	os.Unsetenv("LAYOUT_EPSILON")

	ResetForTest() // Clear cached config
	cfg := Load()

	// Check defaults match expected values
	if cfg.LayoutMaxNodes != 5000 {
		t.Errorf("expected LayoutMaxNodes=5000, got %d", cfg.LayoutMaxNodes)
	}
	if cfg.LayoutIterations != 400 {
		t.Errorf("expected LayoutIterations=400, got %d", cfg.LayoutIterations)
	}
	if cfg.LayoutBatchSize != 5000 {
		t.Errorf("expected LayoutBatchSize=5000, got %d", cfg.LayoutBatchSize)
	}
	if cfg.LayoutEpsilon != 0.0 {
		t.Errorf("expected LayoutEpsilon=0.0, got %f", cfg.LayoutEpsilon)
	}
}

func TestLayoutConfigCustom(t *testing.T) {
	// Set custom values
	os.Setenv("LAYOUT_MAX_NODES", "1000")
	os.Setenv("LAYOUT_ITERATIONS", "100")
	os.Setenv("LAYOUT_BATCH_SIZE", "2000")
	os.Setenv("LAYOUT_EPSILON", "1.5")

	ResetForTest() // Clear cached config
	cfg := Load()

	// Verify custom values are loaded
	if cfg.LayoutMaxNodes != 1000 {
		t.Errorf("expected LayoutMaxNodes=1000, got %d", cfg.LayoutMaxNodes)
	}
	if cfg.LayoutIterations != 100 {
		t.Errorf("expected LayoutIterations=100, got %d", cfg.LayoutIterations)
	}
	if cfg.LayoutBatchSize != 2000 {
		t.Errorf("expected LayoutBatchSize=2000, got %d", cfg.LayoutBatchSize)
	}
	if cfg.LayoutEpsilon != 1.5 {
		t.Errorf("expected LayoutEpsilon=1.5, got %f", cfg.LayoutEpsilon)
	}

	// Clean up
	os.Unsetenv("LAYOUT_MAX_NODES")
	os.Unsetenv("LAYOUT_ITERATIONS")
	os.Unsetenv("LAYOUT_BATCH_SIZE")
	os.Unsetenv("LAYOUT_EPSILON")
	ResetForTest()
}

func TestBacklogActivationConfigHasSafeDefaultsAndAcceptsOverrides(t *testing.T) {
	t.Setenv("CRAWL_BACKLOG_ACTIVATION_LIMIT", "")
	t.Setenv("CRAWL_BACKLOG_ACTIVATION_WINDOW", "")
	ResetForTest()
	if cfg := Load(); cfg.BacklogActivationLimit != 24 || cfg.BacklogActivationWindow.String() != "1h0m0s" {
		t.Fatalf("unexpected backlog activation defaults: limit=%d window=%s", cfg.BacklogActivationLimit, cfg.BacklogActivationWindow)
	}
	t.Setenv("CRAWL_BACKLOG_ACTIVATION_LIMIT", "7")
	t.Setenv("CRAWL_BACKLOG_ACTIVATION_WINDOW", "15m")
	ResetForTest()
	if cfg := Load(); cfg.BacklogActivationLimit != 7 || cfg.BacklogActivationWindow.String() != "15m0s" {
		t.Fatalf("unexpected backlog activation overrides: limit=%d window=%s", cfg.BacklogActivationLimit, cfg.BacklogActivationWindow)
	}
	ResetForTest()
}
