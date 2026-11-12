# Data Integrity and Maintenance Guide

This guide covers data integrity checks, backfill operations, and database maintenance for the Reddit Cluster Map project.

## Overview

The integrity tool provides automated checks and cleanup operations for:
- **Orphan data**: Posts, comments, or graph nodes referencing non-existent entities
- **Dangling links**: Graph links pointing to missing nodes
- **Invalid references**: Comments with non-existent parent references
- **Database bloat**: Identifying tables that need vacuuming
- **Statistics**: Monitoring table sizes and maintenance status

## Quick Start

### Check Data Integrity

Run all integrity checks without making changes:

```bash
make integrity-check
```

This will report:
- Number of orphan posts (posts referencing non-existent subreddits/users)
- Number of orphan comments (comments referencing non-existent posts/users/subreddits)
- Number of dangling graph links (links to non-existent nodes)
- Number of orphan graph nodes (nodes with no connections)
- Number of invalid comment parent references

### Clean Up Issues

Dry-run mode (see what would be cleaned):
```bash
make integrity-clean-dry-run
```

Clean all issues:
```bash
make integrity-clean
```

Clean specific types:
```bash
make integrity-clean TYPE=posts
make integrity-clean TYPE=comments
make integrity-clean TYPE=graph-links
make integrity-clean TYPE=graph-nodes
```

Adjust batch size:
```bash
make integrity-clean BATCH=500
```

### Monitor Database Health

Show table statistics:
```bash
make integrity-stats
```

Analyze table bloat:
```bash
make integrity-bloat
```

Run maintenance operations:
```bash
make maintenance
```

## CLI Tool Usage

The integrity tool can be run directly with more control:

```bash
# Inside a container with DATABASE_URL set
go run ./cmd/integrity check
go run ./cmd/integrity clean -type all -batch 1000
go run ./cmd/integrity clean -dry-run
go run ./cmd/integrity stats
go run ./cmd/integrity bloat
```

## Integrity Checks Explained

### 1. Orphan Posts
Posts that reference subreddits or users that don't exist in the database. This can happen if:
- Data was deleted manually
- Crawling was interrupted
- Database constraints were not enforced

**Fix**: Deletes posts with invalid foreign key references.

### 2. Orphan Comments
Comments that reference non-existent posts, users, or subreddits.

**Fix**: Deletes comments with invalid references. Comments are deleted before posts to maintain referential integrity.

### 3. Dangling Graph Links
Graph links (`graph_links` table) that point to nodes that don't exist in `graph_nodes`.

**Fix**: Removes links to missing nodes.

### 4. Orphan Graph Nodes
Nodes in `graph_nodes` that have no incoming or outgoing links. These are isolated nodes that don't contribute to the graph visualization.

**Fix**: Removes isolated nodes. Run after cleaning graph links to remove newly orphaned nodes.

### 5. Invalid Comment Parents
Comments with `parent_id` values that don't exist as posts or comments. This indicates broken comment threading.

**Fix**: Currently reports only. Consider nullifying the parent_id or deleting such comments based on your needs.

## Database Maintenance

### Routine Maintenance

The `scripts/maintenance.sql` file contains common maintenance operations:

1. **VACUUM**: Reclaims space from deleted/updated rows
2. **ANALYZE**: Updates statistics for query planner
3. **REINDEX**: Rebuilds indexes to remove bloat

Run the full maintenance script:
```bash
make maintenance
```

Or execute specific operations in psql:
```sql
VACUUM ANALYZE posts;
VACUUM ANALYZE comments;
REINDEX TABLE graph_nodes;
```

### When to Run Maintenance

**Daily** (automated via autovacuum):
- Autovacuum handles most maintenance automatically
- Monitor with `make integrity-stats`

**Weekly**:
- Run `make integrity-check` to identify issues
- Review bloat with `make integrity-bloat`
- Run targeted VACUUM on high-bloat tables

**Monthly**:
- Run `make integrity-clean` to remove orphaned data
- Full `make maintenance` during low-traffic periods
- Review database statistics and adjust autovacuum settings if needed

**As Needed**:
- After bulk data imports or deletions
- If query performance degrades
- After schema changes

## Backfill Operations

While this tool focuses on cleanup, backfilling missing data typically happens through the crawler:

1. **User backfill**: Crawl user activity for missing users
2. **Post backfill**: Re-crawl subreddits to fetch missing posts
3. **Comment backfill**: Fetch comments for posts with incomplete comment trees

To backfill:
```bash
# Queue subreddits for re-crawling
curl -X POST http://localhost:8000/api/crawl \
  -H "Content-Type: application/json" \
  -d '{"subreddit": "AskReddit"}'
```

The crawler will automatically:
- Create missing users when it encounters them
- Fetch new posts and comments
- Update existing records with latest data

## Monitoring and Alerts

### Key Metrics to Monitor

1. **Orphan counts**: Should be close to zero in healthy database
2. **Dead tuple percentage**: Tables with >10% dead tuples need vacuuming
3. **Table sizes**: Track growth over time
4. **Last vacuum/analyze times**: Should be recent (within hours/days)

### Setting Up Alerts

Use the integrity checks in monitoring:
```bash
# Exit code 0 if no issues, 1 if issues found
make integrity-check
```

Integrate with monitoring systems:
- Prometheus: Expose integrity metrics
- Cron: Schedule daily checks
- CI/CD: Run checks before/after deployments

## Troubleshooting

### High Orphan Counts

If you see high orphan counts:
1. Check if crawler is running properly
2. Review recent deletions or schema changes
3. Run cleanup during maintenance window
4. Investigate root cause before cleaning

### Database Bloat

If bloat is high (>20% dead tuples):
1. Check autovacuum settings
2. Run manual VACUUM ANALYZE
3. Consider VACUUM FULL during maintenance window (requires lock)
4. Increase `maintenance_work_mem` for faster vacuuming

### Performance Issues

If integrity checks are slow:
1. Run checks during low-traffic periods
2. Reduce batch size: `BATCH=100`
3. Run specific check types instead of all
4. Ensure indexes are up to date: `make maintenance`

## Best Practices

1. **Test in staging first**: Run cleanup operations on a staging database before production
2. **Backup before cleanup**: Always have a recent backup: `make backup-now`
3. **Monitor after cleanup**: Check application behavior after large cleanups
4. **Schedule during low traffic**: Run maintenance during off-peak hours
5. **Document issues**: Track orphan counts over time to identify patterns

## Advanced Configuration

### PostgreSQL Autovacuum Tuning

Edit `postgresql.conf`:
```conf
# Autovacuum settings
autovacuum = on
autovacuum_vacuum_scale_factor = 0.1  # Vacuum when 10% of table is dead
autovacuum_analyze_scale_factor = 0.05  # Analyze when 5% changed
autovacuum_naptime = 1min  # Check for work every minute
maintenance_work_mem = 256MB  # Memory for vacuum operations
```

### Custom Cleanup Scripts

Create custom scripts for specific scenarios:
```go
// Example: Clean old stale data
func CleanStaleData(svc *integrity.Service, days int) error {
    // Custom logic here
}
```

## Related Documentation

- [Setup Guide](../../docs/setup.md) - Initial database setup
- [Developer Guide](../../docs/developer-guide.md) - Development workflows
- [Backup Guide](../../Makefile) - Backup operations (see ##@ Backups section)

## Support

For issues or questions:
1. Check logs: `make logs-db`
2. Review table statistics: `make integrity-stats`
3. Open an issue on GitHub with integrity check output
