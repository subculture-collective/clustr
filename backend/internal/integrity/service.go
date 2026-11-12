package integrity

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// Service provides data integrity operations
type Service struct {
	queries *db.Queries
	db      *sql.DB
}

// NewService creates a new integrity service
func NewService(database *sql.DB) *Service {
	return &Service{
		queries: db.New(database),
		db:      database,
	}
}

// CheckResult contains the result of an integrity check
type CheckResult struct {
	CheckName  string
	IssueCount int64
	Details    string
	CheckedAt  time.Time
	HasIssues  bool
}

// CheckAllIntegrity runs all integrity checks
func (s *Service) CheckAllIntegrity(ctx context.Context, limit int32, offset int32) ([]CheckResult, error) {
	results := make([]CheckResult, 0)
	now := time.Now()

	// Check orphan posts
	orphanPostsCount, err := s.queries.CountOrphanPosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count orphan posts: %w", err)
	}
	results = append(results, CheckResult{
		CheckName:  "orphan_posts",
		IssueCount: orphanPostsCount,
		Details:    "Posts referencing non-existent subreddits or users",
		CheckedAt:  now,
		HasIssues:  orphanPostsCount > 0,
	})

	// Check orphan comments
	orphanCommentsCount, err := s.queries.CountOrphanComments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count orphan comments: %w", err)
	}
	results = append(results, CheckResult{
		CheckName:  "orphan_comments",
		IssueCount: orphanCommentsCount,
		Details:    "Comments referencing non-existent posts, users, or subreddits",
		CheckedAt:  now,
		HasIssues:  orphanCommentsCount > 0,
	})

	// Check dangling graph links
	danglingLinksCount, err := s.queries.CountDanglingGraphLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count dangling graph links: %w", err)
	}
	results = append(results, CheckResult{
		CheckName:  "dangling_graph_links",
		IssueCount: danglingLinksCount,
		Details:    "Graph links referencing non-existent nodes",
		CheckedAt:  now,
		HasIssues:  danglingLinksCount > 0,
	})

	// Check orphan graph nodes
	orphanNodesCount, err := s.queries.CountOrphanGraphNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count orphan graph nodes: %w", err)
	}
	results = append(results, CheckResult{
		CheckName:  "orphan_graph_nodes",
		IssueCount: orphanNodesCount,
		Details:    "Graph nodes with no links",
		CheckedAt:  now,
		HasIssues:  orphanNodesCount > 0,
	})

	// Check invalid comment parents
	invalidParentsCount, err := s.queries.CountInvalidCommentParents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count invalid comment parents: %w", err)
	}
	results = append(results, CheckResult{
		CheckName:  "invalid_comment_parents",
		IssueCount: invalidParentsCount,
		Details:    "Comments with parent_id that doesn't exist",
		CheckedAt:  now,
		HasIssues:  invalidParentsCount > 0,
	})

	return results, nil
}

// CleanupOrphanPosts removes posts referencing non-existent data
func (s *Service) CleanupOrphanPosts(ctx context.Context, batchSize int32) (int64, error) {
	var totalDeleted int64
	for {
		// Count before deletion
		countBefore, err := s.queries.CountOrphanPosts(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count orphan posts: %w", err)
		}
		if countBefore == 0 {
			break
		}

		err = s.queries.DeleteOrphanPosts(ctx, batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete orphan posts: %w", err)
		}

		// Count after deletion to determine actual deleted count
		countAfter, err := s.queries.CountOrphanPosts(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count remaining orphan posts: %w", err)
		}

		deleted := countBefore - countAfter
		totalDeleted += deleted

		if countAfter == 0 {
			break
		}

		log.Printf("Deleted %d orphan posts, %d remaining", deleted, countAfter)
	}
	return totalDeleted, nil
}

// CleanupOrphanComments removes comments referencing non-existent data
func (s *Service) CleanupOrphanComments(ctx context.Context, batchSize int32) (int64, error) {
	var totalDeleted int64
	for {
		// Count before deletion
		countBefore, err := s.queries.CountOrphanComments(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count orphan comments: %w", err)
		}
		if countBefore == 0 {
			break
		}

		err = s.queries.DeleteOrphanComments(ctx, batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete orphan comments: %w", err)
		}

		// Count after deletion to determine actual deleted count
		countAfter, err := s.queries.CountOrphanComments(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count remaining orphan comments: %w", err)
		}

		deleted := countBefore - countAfter
		totalDeleted += deleted

		if countAfter == 0 {
			break
		}

		log.Printf("Deleted %d orphan comments, %d remaining", deleted, countAfter)
	}
	return totalDeleted, nil
}

// CleanupDanglingGraphLinks removes graph links referencing non-existent nodes
func (s *Service) CleanupDanglingGraphLinks(ctx context.Context, batchSize int32) (int64, error) {
	var totalDeleted int64
	for {
		// Count before deletion
		countBefore, err := s.queries.CountDanglingGraphLinks(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count dangling graph links: %w", err)
		}
		if countBefore == 0 {
			break
		}

		err = s.queries.DeleteDanglingGraphLinks(ctx, batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete dangling graph links: %w", err)
		}

		// Count after deletion to determine actual deleted count
		countAfter, err := s.queries.CountDanglingGraphLinks(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count remaining dangling graph links: %w", err)
		}

		deleted := countBefore - countAfter
		totalDeleted += deleted

		if countAfter == 0 {
			break
		}

		log.Printf("Deleted %d dangling graph links, %d remaining", deleted, countAfter)
	}
	return totalDeleted, nil
}

// CleanupOrphanGraphNodes removes graph nodes with no links
func (s *Service) CleanupOrphanGraphNodes(ctx context.Context, batchSize int32) (int64, error) {
	var totalDeleted int64
	for {
		// Count before deletion
		countBefore, err := s.queries.CountOrphanGraphNodes(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count orphan graph nodes: %w", err)
		}
		if countBefore == 0 {
			break
		}

		err = s.queries.DeleteOrphanGraphNodes(ctx, batchSize)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete orphan graph nodes: %w", err)
		}

		// Count after deletion to determine actual deleted count
		countAfter, err := s.queries.CountOrphanGraphNodes(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count remaining orphan graph nodes: %w", err)
		}

		deleted := countBefore - countAfter
		totalDeleted += deleted

		if countAfter == 0 {
			break
		}

		log.Printf("Deleted %d orphan graph nodes, %d remaining", deleted, countAfter)
	}
	return totalDeleted, nil
}

// DatabaseStats contains database statistics
type DatabaseStats struct {
	TableName       string
	Size            string
	RowCount        int64
	DeadRows        int64
	LastVacuum      *time.Time
	LastAutoVacuum  *time.Time
	LastAnalyze     *time.Time
	LastAutoAnalyze *time.Time
}

// GetDatabaseStatistics retrieves database statistics for monitoring
func (s *Service) GetDatabaseStatistics(ctx context.Context) ([]DatabaseStats, error) {
	query := `
		SELECT 
			schemaname,
			tablename,
			pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size,
			n_live_tup as row_count,
			n_dead_tup as dead_rows,
			last_vacuum,
			last_autovacuum,
			last_analyze,
			last_autoanalyze
		FROM pg_stat_user_tables
		WHERE schemaname = 'public'
		ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query table statistics: %w", err)
	}
	defer rows.Close()

	var stats []DatabaseStats
	for rows.Next() {
		var schema, tablename, size string
		var rowCount, deadRows int64
		var lastVacuum, lastAutoVacuum, lastAnalyze, lastAutoAnalyze sql.NullTime

		err := rows.Scan(&schema, &tablename, &size, &rowCount, &deadRows,
			&lastVacuum, &lastAutoVacuum, &lastAnalyze, &lastAutoAnalyze)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		stat := DatabaseStats{
			TableName: tablename,
			Size:      size,
			RowCount:  rowCount,
			DeadRows:  deadRows,
		}
		if lastVacuum.Valid {
			stat.LastVacuum = &lastVacuum.Time
		}
		if lastAutoVacuum.Valid {
			stat.LastAutoVacuum = &lastAutoVacuum.Time
		}
		if lastAnalyze.Valid {
			stat.LastAnalyze = &lastAnalyze.Time
		}
		if lastAutoAnalyze.Valid {
			stat.LastAutoAnalyze = &lastAutoAnalyze.Time
		}

		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// GetBloatAnalysis identifies tables with high bloat that need vacuum
func (s *Service) GetBloatAnalysis(ctx context.Context) ([]DatabaseStats, error) {
	query := `
		SELECT
			schemaname,
			tablename,
			pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS total_size,
			n_live_tup as row_count,
			n_dead_tup as dead_rows
		FROM pg_stat_user_tables
		WHERE schemaname = 'public'
		  AND (n_live_tup + n_dead_tup) > 0
		ORDER BY n_dead_tup DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query bloat analysis: %w", err)
	}
	defer rows.Close()

	var stats []DatabaseStats
	for rows.Next() {
		var schema, tablename, size string
		var rowCount, deadRows int64

		err := rows.Scan(&schema, &tablename, &size, &rowCount, &deadRows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		stats = append(stats, DatabaseStats{
			TableName: tablename,
			Size:      size,
			RowCount:  rowCount,
			DeadRows:  deadRows,
		})
	}

	return stats, rows.Err()
}
