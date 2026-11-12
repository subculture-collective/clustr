# Data Integrity Quick Start

Quick reference for common data integrity operations.

## Daily Checks

```bash
# Check for integrity issues (run daily)
make integrity-check

# View database statistics
make integrity-stats
```

## Weekly Maintenance

```bash
# Analyze bloat
make integrity-bloat

# Clean up issues (dry-run first)
make integrity-clean-dry-run
make integrity-clean

# Run database maintenance
make maintenance
```

## Common Commands

### Check for Issues

```bash
make integrity-check
```

Output example:
```
=== Integrity Check Results ===

orphan_posts:                  ✓ OK
  Posts referencing non-existent subreddits or users

orphan_comments:               ⚠ ISSUES FOUND: 42
  Comments referencing non-existent posts, users, or subreddits

dangling_graph_links:          ✓ OK
  Graph links referencing non-existent nodes
```

### Clean Up Issues

```bash
# Preview what would be cleaned
make integrity-clean-dry-run

# Clean specific types
make integrity-clean TYPE=comments

# Clean all issues
make integrity-clean

# Adjust batch size for large datasets
make integrity-clean BATCH=500
```

### Monitor Database Health

```bash
# Table statistics
make integrity-stats

# Bloat analysis
make integrity-bloat

# Full maintenance
make maintenance
```

## Troubleshooting

### High Orphan Counts

If integrity checks show many orphaned records:

1. Check crawler status: `make logs-crawler`
2. Review recent changes to schema or data
3. Run cleanup during low-traffic period
4. Investigate root cause before deleting

### Performance Issues

If checks are slow:

1. Reduce batch size: `make integrity-clean BATCH=100`
2. Run during off-peak hours
3. Check database indexes: `make integrity-stats`
4. Consider running maintenance: `make maintenance`

## Integration with Monitoring

### Cron Job Example

Add to crontab for automated daily checks:

```bash
# Daily integrity check at 2 AM
0 2 * * * cd /path/to/backend && make integrity-check >> /var/log/integrity.log 2>&1

# Weekly cleanup at 3 AM on Sundays
0 3 * * 0 cd /path/to/backend && make integrity-clean >> /var/log/integrity-clean.log 2>&1
```

### Alert on Issues

```bash
#!/bin/bash
cd /path/to/backend
if ! make integrity-check > /tmp/integrity-check.log 2>&1; then
    # Send alert (email, Slack, etc.)
    cat /tmp/integrity-check.log | mail -s "Integrity Check Failed" admin@example.com
fi
```

## See Also

- [Full Documentation](DATA_INTEGRITY.md) - Comprehensive guide
- [Makefile](../../Makefile) - All available targets
- [Maintenance Script](../scripts/maintenance.sql) - SQL operations
