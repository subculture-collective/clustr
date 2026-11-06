package metrics

import (
	"context"
	"log"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// databaseStatsEvery is how many collection ticks pass between exact source
// table counts. Counting every comment and user takes seconds of sequential
// scanning, and the totals move slowly, so they are not recounted every tick.
const databaseStatsEvery = 10

// Collector periodically collects and updates Prometheus metrics
type Collector struct {
	queries  *db.Queries
	interval time.Duration
	stop     chan struct{}
	ticks    int
}

// NewCollector creates a new metrics collector
func NewCollector(queries *db.Queries, interval time.Duration) *Collector {
	return &Collector{
		queries:  queries,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// Start begins the metrics collection loop
func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Collect initial metrics
	c.collectMetrics(ctx)

	for {
		select {
		case <-ticker.C:
			c.collectMetrics(ctx)
		case <-c.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop stops the metrics collector
func (c *Collector) Stop() {
	close(c.stop)
}

// collectMetrics collects all metrics from the database
func (c *Collector) collectMetrics(ctx context.Context) {
	c.collectGraphMetrics(ctx)
	c.collectCommunityMetrics(ctx)
	if c.ticks%databaseStatsEvery == 0 {
		c.collectDatabaseStats(ctx)
	}
	c.ticks++
	c.collectCrawlJobStats(ctx)
}

// collectGraphMetrics collects graph node and link counts
func (c *Collector) collectGraphMetrics(ctx context.Context) {
	// Count total links
	linkCount, err := c.queries.CountGraphLinks(ctx)
	if err != nil {
		log.Printf("Error counting graph links: %v", err)
		MetricsCollectionErrors.WithLabelValues("graph").Inc()
		GraphLinksTotal.Set(-1) // Signal stale data
	} else {
		GraphLinksTotal.Set(float64(linkCount))
	}
}

// collectCommunityMetrics collects community-related metrics
func (c *Collector) collectCommunityMetrics(ctx context.Context) {
	// Count communities from graph_communities (created by migration 000019)
	count, err := c.queries.CountCommunities(ctx)
	if err != nil {
		log.Printf("Error counting communities: %v", err)
		MetricsCollectionErrors.WithLabelValues("community").Inc()
		CommunitiesTotal.Set(-1) // Signal stale data
	} else {
		CommunitiesTotal.Set(float64(count))
	}
}

// collectDatabaseStats collects database entity counts
func (c *Collector) collectDatabaseStats(ctx context.Context) {
	stats, err := c.queries.GetDatabaseStats(ctx)
	if err != nil {
		log.Printf("Error getting database stats: %v", err)
		MetricsCollectionErrors.WithLabelValues("database").Inc()
		// Signal stale data for all node types
		GraphNodesTotal.WithLabelValues("subreddit").Set(-1)
		GraphNodesTotal.WithLabelValues("user").Set(-1)
		GraphNodesTotal.WithLabelValues("post").Set(-1)
		GraphNodesTotal.WithLabelValues("comment").Set(-1)
		return
	}

	// Update node counts with database stats
	GraphNodesTotal.WithLabelValues("subreddit").Set(float64(stats.SubredditCount))
	GraphNodesTotal.WithLabelValues("user").Set(float64(stats.UserCount))
	GraphNodesTotal.WithLabelValues("post").Set(float64(stats.PostCount))
	GraphNodesTotal.WithLabelValues("comment").Set(float64(stats.CommentCount))
}

// collectCrawlJobStats collects crawl job status metrics
func (c *Collector) collectCrawlJobStats(ctx context.Context) {
	stats, err := c.queries.GetCrawlJobStats(ctx)
	if err != nil {
		log.Printf("Error getting crawl job stats: %v", err)
		MetricsCollectionErrors.WithLabelValues("crawl_jobs").Inc()
		// Signal stale data for all job status metrics
		CrawlJobsPending.Set(-1)
		CrawlJobsProcessing.Set(-1)
		CrawlJobsCompleted.Set(-1)
		CrawlJobsFailed.Set(-1)
		return
	}

	// Update crawl job metrics
	CrawlJobsPending.Set(float64(stats.PendingJobs))
	CrawlJobsProcessing.Set(float64(stats.ProcessingJobs))
	CrawlJobsCompleted.Set(float64(stats.CompletedJobs))
	CrawlJobsFailed.Set(float64(stats.FailedJobs))
}
