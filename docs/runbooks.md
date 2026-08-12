# Operational Runbooks

This document provides step-by-step procedures for common operational tasks and troubleshooting scenarios.

## Table of Contents

- [Backup and Restore](#backup-and-restore)
- [Production Database Clone](runbooks/production-database-clone.md)
- [Almaz and Kvant Launch](runbooks/almaz-kvant-launch.md)
- [Graph Precalculation](#graph-precalculation)
- [Database Maintenance](#database-maintenance)
- [Crawler Operations](#crawler-operations)
- [Performance Tuning](#performance-tuning)
- [Incident Response](#incident-response)
- [Common Issues](#common-issues)

## Backup and Restore

### Creating a Manual Backup

**When to use:** Before major migrations, schema changes, or as part of disaster recovery planning.

**Steps:**

1. Ensure services are running:
   ```bash
   cd backend
   docker compose ps
   ```

2. Create a backup:
   ```bash
   make backup-now
   ```
   
   This will:
   - Connect to the database
   - Run `pg_dump` to create a SQL dump
   - Save to `pgbackups` volume with timestamp
   - Keep only the last 7 backups

3. Verify backup was created:
   ```bash
   make backups-ls
   ```

4. Download backup to local machine (optional):
   ```bash
   make backups-download-latest
   ```
   
   Backup will be saved to `backend/backups/` directory.

**Expected Output:**
```
Running backup via precalculate service...
Backup completed: backups/reddit_cluster_20251030_123045.sql (size=15728640 bytes)
```

**Troubleshooting:**

- **"database not reachable"**: Check if database service is running with `docker compose ps db`
- **"Permission denied"**: Ensure the precalculate container has access to the pgbackups volume
- **Backup file size 0**: Database may be empty or connection failed; check logs with `make logs-db`

---

### Restoring from Backup

**When to use:** After data corruption, accidental deletion, or to roll back changes.

**⚠️ Warning:** This will **replace all data** in the target database. Ensure you have a current backup before proceeding.

**Steps:**

1. **Stop services** that write to the database:
   ```bash
   cd backend
   docker compose stop crawler api precalculate
   ```

2. **List available backups:**
   ```bash
   make backups-ls
   ```
   
   Note the filename you want to restore (e.g., `reddit_cluster_20251030_120000.sql`)

3. **Restore using docker exec:**
   
   ```bash
   # Method 1: Restore from backup volume
   # First, extract the backup file to a local path
   docker run --rm -v backend_pgbackups:/backups -v $(pwd):/out alpine \
     cp /backups/reddit_cluster_20251030_120000.sql /out/restore.sql
   
   # Then restore it
   cat restore.sql | docker compose exec -T db psql -U postgres -d reddit_cluster
   ```
   
   Or if you have a local backup file:
   ```bash
   cat backups/reddit_cluster_20251030_120000.sql | \
     docker compose exec -T db psql -U postgres -d reddit_cluster
   ```

4. **Verify restoration:**
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster -c "SELECT COUNT(*) FROM subreddits;"
   docker compose exec db psql -U postgres -d reddit_cluster -c "SELECT COUNT(*) FROM posts;"
   ```

5. **Restart services:**
   ```bash
   docker compose start crawler api precalculate
   ```

6. **Run migrations** (if restoring to a different version):
   ```bash
   make migrate-up-local
   ```

7. **Trigger graph regeneration:**
   ```bash
   docker compose run --rm precalculate /app/precalculate
   ```

**Alternative: Full Database Restore (Drop and Recreate)**

For a clean restore when there are schema conflicts:

```bash
# 1. Stop all services
docker compose down

# 2. Remove database volume (destructive!)
# Note: Volume name format is typically <directory>_<volume_name>
# Verify with: docker volume ls | grep postgres_data
docker volume rm backend_postgres_data

# 3. Start database
docker compose up -d db

# 4. Wait for database to initialize
sleep 10

# 5. Restore from backup
cat backups/reddit_cluster_20251030_120000.sql | \
  docker compose exec -T db psql -U postgres -d reddit_cluster

# 6. Start all services
docker compose up -d
```

---

### Automated Backup Schedule

**Backup service** runs automatically every 24 hours (configurable via `BACKUP_INTERVAL`).

**Check backup service status:**
```bash
docker compose logs backup | tail -20
```

**Modify backup interval:**

Edit `backend/docker-compose.yml`:
```yaml
backup:
  environment:
    - BACKUP_INTERVAL=12h  # Changed from 24h
```

Restart service:
```bash
docker compose up -d backup
```

**Backup retention:**

By default, `backup.sh` keeps the last 7 backups. To adjust:

Edit `backend/scripts/backup.sh`:
```bash
# Change this line (portable version without -r flag):
ls -t backups/reddit_cluster_*.sql 2>/dev/null | tail -n +8 | xargs rm -- 2>/dev/null || true

# To keep last 14 backups:
ls -t backups/reddit_cluster_*.sql 2>/dev/null | tail -n +15 | xargs rm -- 2>/dev/null || true
```

> **Note:** The `-r` flag for `xargs` is GNU-specific and not available on macOS/BSD. The portable version omits `-r` and redirects errors to `/dev/null` or uses `|| true` to avoid errors when no files match.

Or use the tidy command:
```bash
# Keep last 14 backups
make backups-tidy KEEP=14
```

---

### Backup Volume Snapshot

**When to use:** To capture the entire backup history or migrate to another server.

**Create snapshot:**
```bash
make backups-snapshot-volume
```

This creates a compressed tarball of all backups in the volume.

**Extract snapshot:**
```bash
# On target server
docker run --rm -v backend_pgbackups:/out -v $(pwd):/backup alpine sh -c \
  "cd /out && tar -xzf /backup/pgdata_TIMESTAMP.tar.gz"
```

---

## Graph Precalculation

### Manual Precalculation

**When to use:** After bulk data imports, to refresh stale graphs, or for testing.

**Steps:**

1. **Trigger one-time precalculation:**
   ```bash
   cd backend
   docker compose run --rm precalculate /app/precalculate
   ```

2. **Monitor progress:**
   ```bash
   docker compose logs -f precalculate
   ```
   
   Look for progress indicators:
   - "Generated X subreddit nodes"
   - "Generated Y user nodes"
   - "Created Z links"
   - "Precalculation completed in Xs"

3. **Verify results:**
   ```bash
   curl http://localhost:8000/api/graph?max_nodes=10&max_links=10 | jq '.nodes | length'
   ```
   
   Should return a number > 0 if graph was generated successfully.

**Expected Duration:**
- Small dataset (< 1000 subreddits): 1-5 minutes
- Medium dataset (1000-10000 subreddits): 5-30 minutes  
- Large dataset (> 10000 subreddits): 30+ minutes

**Troubleshooting:**

- **"No subreddits found"**: Database is empty; run `make seed` to add sample data
- **"timeout exceeded"**: Increase `DB_STATEMENT_TIMEOUT_MS` in `.env`
- **Memory issues**: Reduce batch sizes (`GRAPH_NODE_BATCH_SIZE`, `GRAPH_LINK_BATCH_SIZE`)
- **Still running after 1 hour**: Check for database locks with `docker compose exec db psql -U postgres -d reddit_cluster -c "SELECT * FROM pg_stat_activity WHERE state != 'idle';"`

---

### Clearing and Regenerating Graph

**When to use:** After changing graph generation settings or to fix corrupt graph data.

**Steps:**

1. **Set environment variable:**
   ```bash
   # In backend/.env
   PRECALC_CLEAR_ON_START=true
   ```

2. **Run precalculation:**
   ```bash
   cd backend
   docker compose run --rm precalculate /app/precalculate
   ```
   
   This will:
   - DELETE all existing graph nodes
   - DELETE all existing graph links
   - Regenerate from scratch

3. **Revert setting** (recommended):
   ```bash
   # In backend/.env
   PRECALC_CLEAR_ON_START=false
   ```

**Alternative: Manual clearing:**

```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
TRUNCATE TABLE graph_links;
TRUNCATE TABLE graph_nodes;
EOF
```

Then run precalculation normally.

---

### Optimizing Graph Performance

**When to use:** Graph API responses are slow (> 2 seconds) or graphs are too large.

**Reduce graph size:**

Edit `backend/.env`:
```bash
# Disable detailed content graph
DETAILED_GRAPH=false

# Or reduce content limits
POSTS_PER_SUB_IN_GRAPH=5      # Down from 10
COMMENTS_PER_POST_IN_GRAPH=25  # Down from 50
MAX_AUTHOR_CONTENT_LINKS=1     # Down from 3
```

Regenerate graph:
```bash
cd backend
PRECALC_CLEAR_ON_START=true docker compose run --rm precalculate /app/precalculate
```

**Increase batch sizes for faster generation:**

```bash
# In backend/.env
GRAPH_NODE_BATCH_SIZE=2000     # Up from 1000
GRAPH_LINK_BATCH_SIZE=5000     # Up from 2000
```

**Reduce progress logging overhead:**

```bash
# In backend/.env
GRAPH_PROGRESS_INTERVAL=50000  # Up from 10000
```

**Check index status:**

```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
-- Ensure indexes exist
\di graph_*

-- Check index usage
SELECT schemaname, tablename, indexname, idx_scan
FROM pg_stat_user_indexes
WHERE schemaname = 'public' AND tablename LIKE 'graph_%'
ORDER BY idx_scan;
EOF
```

If indexes are missing or not being used, run migrations:
```bash
make migrate-up-local
```

---

### Scheduled Precalculation

**Default schedule:** Hourly (via precalculate service in docker-compose.yml)

**Disable automatic precalculation:**

Option 1 - Stop the precalculate service:
```bash
docker compose stop precalculate
```

Option 2 - Disable API background job:
```bash
# In backend/.env
DISABLE_API_GRAPH_JOB=true
```

**Check last precalculation time:**

```bash
docker compose logs precalculate | grep "completed in"
```

**Force immediate precalculation:**

```bash
docker compose restart precalculate
```

---

## Database Maintenance

### Integrity Checks

**When to use:** Regularly (monthly) or after detecting data anomalies.

**Run comprehensive integrity check:**
```bash
cd backend
make integrity-check
```

This checks for:
- Orphaned posts (references non-existent subreddit/author)
- Orphaned comments (references non-existent post/author)
- Invalid graph links (source/target nodes don't exist)
- Duplicate entries

**View results:**
Output will show counts of issues found in each category.

---

### Cleaning Invalid Data

**When to use:** After integrity check reports issues.

**Dry run** (see what would be deleted):
```bash
make integrity-clean-dry-run
```

**Clean all issues:**
```bash
make integrity-clean TYPE=all
```

**Clean specific categories:**
```bash
# Clean only orphaned posts
make integrity-clean TYPE=posts

# Clean only orphaned comments
make integrity-clean TYPE=comments

# Clean only invalid graph links
make integrity-clean TYPE=graph-links

# Clean only invalid graph nodes
make integrity-clean TYPE=graph-nodes
```

**Adjust batch size** (default: 1000):
```bash
make integrity-clean TYPE=all BATCH=5000
```

**After cleaning, regenerate graph:**
```bash
docker compose run --rm precalculate /app/precalculate
```

---

### Database Statistics

**View current database stats:**
```bash
make integrity-stats
```

Shows:
- Total counts for each table
- Database size
- Table sizes
- Index sizes

**Analyze table bloat:**
```bash
make integrity-bloat
```

Identifies tables with excessive bloat that may benefit from VACUUM FULL.

---

### VACUUM Operations

**When to use:** After large deletions or when bloat analysis shows issues.

**Standard VACUUM** (safe, non-blocking):
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c "VACUUM ANALYZE;"
```

**VACUUM FULL** (requires exclusive lock, use during maintenance window):
```bash
# Stop services to prevent new connections
docker compose stop api crawler

# Run VACUUM FULL
docker compose exec db psql -U postgres -d reddit_cluster -c "VACUUM FULL ANALYZE;"

# Restart services
docker compose start api crawler
```

**VACUUM specific table:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c "VACUUM ANALYZE posts;"
```

---

### Reindexing

**When to use:** After corruption, performance degradation, or PostgreSQL upgrades.

**Reindex all tables:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c "REINDEX DATABASE reddit_cluster;"
```

**Reindex specific table:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c "REINDEX TABLE graph_nodes;"
```

**Reindex concurrently** (PostgreSQL 12+, doesn't block writes):
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c "REINDEX INDEX CONCURRENTLY idx_graph_nodes_type;"
```

---

### Routine Maintenance Script

**When to use:** Monthly or as part of regular maintenance schedule.

**Run maintenance script:**
```bash
cd backend
make maintenance
```

This script (in `backend/scripts/maintenance.sql`) runs:
- VACUUM ANALYZE on all tables
- Updates table statistics
- Rebuilds problematic indexes if needed

**Schedule via cron:**
```bash
# Add to crontab
0 2 * * 0 cd /path/to/backend && make maintenance >> /var/log/reddit-cluster-maintenance.log 2>&1
```

---

## Crawler Operations

### Starting a New Crawl

**Enqueue single subreddit:**
```bash
cd backend
make test-crawl SUB=golang
```

**Enqueue multiple subreddits:**
```bash
make seed
```

This seeds several popular subreddits:
- AskReddit
- programming
- golang
- webdev
- dataisbeautiful

**Enqueue via API:**
```bash
curl -X POST http://localhost:8000/api/crawl \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${ADMIN_API_TOKEN}" \
  -d '{"subreddit": "rust"}'
```

---

### Monitoring Crawl Progress

**Check job status:**
```bash
curl http://localhost:8000/jobs | jq '.[] | {subreddit: .subreddit_name, status: .status, priority: .priority}'
```

**Watch crawler logs:**
```bash
cd backend
make logs-crawler
```

**Filter for specific subreddit:**
```bash
make logs-crawler | grep "golang"
```

**Count jobs by status:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c \
  "SELECT status, COUNT(*) FROM crawl_jobs GROUP BY status;"
```

---

### Handling Stuck Crawl Jobs

**Symptom:** Jobs remain in "crawling" status for > 15 minutes (default `RESET_CRAWLING_AFTER_MIN`).

**Automatic reset:** Crawler automatically resets stuck jobs. Check logs:
```bash
make logs-crawler | grep "Resetting stuck job"
```

**Manual reset:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
UPDATE crawl_jobs 
SET status = 'pending', 
    started_at = NULL 
WHERE status = 'crawling' 
  AND started_at < NOW() - INTERVAL '15 minutes';
EOF
```

**Restart crawler to process reset jobs:**
```bash
docker compose restart crawler
```

---

### Adjusting Crawl Priority

**When to use:** To prioritize specific subreddits over others.

**Update priority** (higher number = higher priority):
```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
UPDATE crawl_jobs 
SET priority = 100 
WHERE subreddit_name = 'golang' AND status = 'pending';
EOF
```

**Default priority:** 1 (normal)  
**High priority:** 10-100  
**Urgent priority:** 100+

---

### Pausing and Resuming Crawls

**Pause crawler:**
```bash
docker compose stop crawler
```

**Resume crawler:**
```bash
docker compose start crawler
```

**Jobs remain in database and will be processed when crawler restarts.**

---

### Clearing Failed Jobs

**View failed jobs:**
```bash
curl http://localhost:8000/jobs | jq '.[] | select(.status == "failed")'
```

**Delete failed jobs:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c \
  "DELETE FROM crawl_jobs WHERE status = 'failed';"
```

**Retry failed jobs:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster -c \
  "UPDATE crawl_jobs SET status = 'pending', error_message = NULL WHERE status = 'failed';"
```

---

## Performance Tuning

### Identifying Slow Queries

**Enable slow query logging:**

```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
ALTER DATABASE reddit_cluster SET log_min_duration_statement = 1000;
EOF
```

This logs queries taking > 1 second.

**View slow queries in logs:**
```bash
make logs-db | grep "duration:"
```

**Check current running queries:**
```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND now() - query_start > interval '2 seconds';
EOF
```

---

### Running Performance Benchmarks

**Benchmark graph queries:**
```bash
cd backend
make benchmark-graph
```

This measures:
- Graph node queries with different limits
- Graph link queries
- Index usage
- Execution times

**View benchmark results:**
Output includes average execution times over 5 runs.

**Run with EXPLAIN ANALYZE:**
```bash
psql $DATABASE_URL -f backend/scripts/explain_analyze_queries.sql
```

See [docs/perf.md](./perf.md) for detailed performance analysis.

---

### Optimizing Graph API Response Time

**Current performance:** Target < 1 second for max_nodes=20000

**Tuning options:**

1. **Increase cache TTL** (default: 60 seconds):
   - Modify `backend/internal/api/handlers/graph.go`
   - Change `cacheTTL` constant

2. **Reduce default limits:**
   ```bash
   # Frontend .env
   VITE_MAX_RENDER_NODES=15000  # Down from 20000
   VITE_MAX_RENDER_LINKS=40000  # Down from 50000
   ```

3. **Database connection pooling:**
   ```bash
   # In DATABASE_URL
   ?pool_max_conns=20&pool_min_conns=5
   ```

4. **Check query plan:**
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster <<EOF
   EXPLAIN (ANALYZE, BUFFERS) 
   SELECT id, name, val, type 
   FROM graph_nodes 
   ORDER BY CAST(val AS NUMERIC) DESC 
   LIMIT 20000;
   EOF
   ```

Look for:
- Sequential scans (bad) vs index scans (good)
- High buffer usage
- Expensive sorts

---

### Scaling Considerations

**Horizontal scaling - Multiple crawlers:**
```yaml
# In docker-compose.yml
crawler:
  deploy:
    replicas: 3
```

Crawlers coordinate via database job queue.

**Horizontal scaling - API servers:**
- Use external Redis for caching (not in-memory)
- Deploy behind load balancer
- Share `ADMIN_API_TOKEN` across instances

**Vertical scaling - Database:**
- Increase PostgreSQL memory settings
- Add more CPU cores
- Use faster SSD storage

**Read replicas:**
- Configure PostgreSQL streaming replication
- Point read-only queries to replicas
- Keep write operations on primary

---

## Incident Response

### API Server Not Responding

**Symptoms:** 
- 503 errors
- Timeouts
- Health check fails

**Diagnosis:**

1. Check service status:
   ```bash
   docker compose ps api
   ```

2. Check logs for errors:
   ```bash
   make logs-api | tail -100
   ```

3. Check health endpoint:
   ```bash
   curl -v http://localhost:8000/health
   ```

**Resolution:**

1. If service is stopped:
   ```bash
   docker compose start api
   ```

2. If service is unhealthy:
   ```bash
   docker compose restart api
   ```

3. If repeated failures:
   ```bash
   # Check database connectivity
   docker compose exec api ping db
   
   # Rebuild and restart
   docker compose up -d --build api
   ```

4. If still failing, check disk space:
   ```bash
   df -h
   docker system df
   ```

---

### Database Connection Pool Exhausted

**Symptoms:**
- "too many connections" errors
- Slow API responses
- Crawler timing out

**Diagnosis:**

```bash
docker compose exec db psql -U postgres -d reddit_cluster <<EOF
SELECT count(*), state 
FROM pg_stat_activity 
GROUP BY state;
EOF
```

**Resolution:**

1. Terminate idle connections:
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster <<EOF
   SELECT pg_terminate_backend(pid) 
   FROM pg_stat_activity 
   WHERE state = 'idle' 
     AND state_change < now() - interval '10 minutes';
   EOF
   ```

2. Increase connection limit temporarily:
   ```bash
   docker compose exec db psql -U postgres <<EOF
   ALTER SYSTEM SET max_connections = 200;
   EOF
   
   docker compose restart db
   ```

3. Find connection leaks in application logs:
   ```bash
   make logs-api | grep "connection"
   make logs-crawler | grep "connection"
   ```

---

### Out of Disk Space

**Symptoms:**
- Database write errors
- "No space left on device"
- Services failing to start

**Diagnosis:**

```bash
df -h
docker system df
```

**Resolution:**

1. Clean Docker resources:
   ```bash
   docker system prune -a --volumes
   ```
   ⚠️ This removes unused volumes, containers, and images.

2. Remove old backups:
   ```bash
   make backups-tidy KEEP=3  # Keep only last 3
   ```

3. Vacuum database:
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster -c "VACUUM FULL;"
   ```

4. Delete old crawl job records:
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster <<EOF
   DELETE FROM crawl_jobs 
   WHERE completed_at < NOW() - INTERVAL '30 days';
   EOF
   ```

---

### High Memory Usage

**Symptoms:**
- OOM killer terminating containers
- Slow performance
- Swapping to disk

**Diagnosis:**

```bash
docker stats
```

**Resolution:**

1. Reduce batch sizes:
   ```bash
   # In backend/.env
   GRAPH_NODE_BATCH_SIZE=500
   GRAPH_LINK_BATCH_SIZE=1000
   ```

2. Add memory limits to docker-compose.yml:
   ```yaml
   api:
     mem_limit: 1g
     memswap_limit: 1g
   ```

3. Optimize queries:
   - Use pagination
   - Limit result sets
   - Add appropriate indexes

4. Increase swap space on host.

---

### Rate Limit Issues

**Symptoms:**
- HTTP 429 responses
- "rate limit exceeded" in logs

**Diagnosis:**

```bash
# Check rate limit metrics
curl http://localhost:8000/metrics | grep rate_limit

# View logs
make logs-api | grep "rate limit"
```

**Resolution:**

1. For legitimate high traffic, increase limits:
   ```bash
   # In backend/.env
   RATE_LIMIT_GLOBAL=200      # Up from 100
   RATE_LIMIT_PER_IP=20       # Up from 10
   ```

2. For abuse, identify offending IPs:
   ```bash
   make logs-api | grep "429" | awk '{print $1}' | sort | uniq -c | sort -rn
   ```

3. Block at network level if needed.

---

## Common Issues

### Reddit OAuth Token Expired

**Symptoms:**
- 401 Unauthorized errors from Reddit
- "invalid_grant" in crawler logs

**Resolution:**

1. Refresh credentials in `.env`:
   ```bash
   cd backend
   nano .env
   # Update REDDIT_CLIENT_ID and REDDIT_CLIENT_SECRET
   ```

2. Restart crawler:
   ```bash
   docker compose restart crawler
   ```

See [docs/oauth-token-management.md](./oauth-token-management.md) for details.

---

### Migrations Out of Sync

**Symptoms:**
- "relation does not exist" errors
- "column does not exist" errors

**Resolution:**

```bash
cd backend

# Check current migration version
migrate -path migrations -database "$DATABASE_URL" version

# Run missing migrations
make migrate-up-local

# If dirty state
migrate -path migrations -database "$DATABASE_URL" force VERSION_NUMBER
make migrate-up-local
```

---

### Graph Shows No Data

**Checklist:**

1. Has precalculation run?
   ```bash
   docker compose logs precalculate | grep "completed"
   ```

2. Does database have data?
   ```bash
   docker compose exec db psql -U postgres -d reddit_cluster -c \
     "SELECT COUNT(*) FROM graph_nodes;"
   ```

3. Is API returning data?
   ```bash
   curl http://localhost:8000/api/graph?max_nodes=10 | jq '.nodes | length'
   ```

4. Are frontend env variables correct?
   ```bash
   cat frontend/.env | grep VITE_API_URL
   ```

**Fix:** Run precalculation manually:
```bash
docker compose run --rm precalculate /app/precalculate
```

---

### Frontend Build Errors

**Symptoms:**
- TypeScript errors
- ESLint errors
- Build fails in Docker

**Resolution:**

1. Clean and rebuild:
   ```bash
   cd frontend
   rm -rf node_modules dist
   npm ci
   npm run build
   ```

2. Fix TypeScript errors:
   ```bash
   npx tsc --noEmit
   ```

3. Fix ESLint errors:
   ```bash
   npm run lint -- --fix
   ```

4. Rebuild Docker image:
   ```bash
   cd backend
   docker compose build reddit_frontend
   docker compose up -d reddit_frontend
   ```

---

### Circuit Breaker Tripped

**Symptoms:**
- "circuit breaker open" in logs
- Reddit API calls failing immediately

**Explanation:** Circuit breaker protects against cascading failures by temporarily stopping requests to failing services.

**Resolution:**

Circuit breaker resets automatically after cooldown period. To force reset:

```bash
docker compose restart crawler
```

Monitor recovery:
```bash
make logs-crawler | grep "circuit"
```

See [docs/CRAWLER_RESILIENCE.md](./CRAWLER_RESILIENCE.md) for configuration details.

---

## Best Practices

### Regular Maintenance Schedule

**Daily:**
- Check service health: `docker compose ps`
- Review error logs: `make logs-all | grep -i error`
- Monitor disk usage: `df -h`

**Weekly:**
- Review backup status: `make backups-ls`
- Check crawler job queue: `curl http://localhost:8000/jobs | jq 'length'`
- Review monitoring dashboards in Grafana

**Monthly:**
- Run integrity checks: `make integrity-check`
- Database VACUUM: `docker compose exec db psql -U postgres -d reddit_cluster -c "VACUUM ANALYZE;"`
- Review and clean old data
- Test backup restoration procedure
- Update dependencies

**Quarterly:**
- Full backup and test restore
- Review and update documentation
- Performance benchmarking
- Security audit

---

### Monitoring Checklist

**Key Metrics to Watch:**

1. **API Response Time** (target: p95 < 1s)
   - Check in Grafana or: `curl http://localhost:8000/metrics | grep http_request_duration`

2. **Crawler Success Rate** (target: > 90%)
   - Check: `curl http://localhost:8000/jobs | jq '[.[] | .status] | group_by(.) | map({status: .[0], count: length})'`

3. **Database Connection Count** (target: < 80% of max)
   - Check: `docker compose exec db psql -U postgres -c "SELECT count(*) FROM pg_stat_activity;"`

4. **Disk Usage** (target: < 80%)
   - Check: `df -h`

5. **Error Rates** (target: < 1%)
   - Check logs: `make logs-api | grep -c ERROR`

**Set up alerts for:**
- API error rate > 5%
- Crawler error rate > 10%
- Disk usage > 85%
- Database connections > 90% of max
- Graph precalculation failures

---

## Getting Help

**For operational issues:**
1. Check this runbook first
2. Search existing GitHub issues
3. Review logs for error messages
4. Check monitoring dashboards

**For new issues:**
1. Gather diagnostic information:
   ```bash
   docker compose ps
   docker compose logs --tail=100 > logs.txt
   docker system df
   ```

2. Open GitHub issue with:
   - Clear description of problem
   - Steps to reproduce
   - Relevant logs
   - Environment details (OS, Docker version, etc.)

**Emergency contacts:**
- GitHub Issues: https://github.com/subculture-collective/reddit-cluster-map/issues
- Maintainers: See CONTRIBUTING.md
