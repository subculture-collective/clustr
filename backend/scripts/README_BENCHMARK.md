# Graph Query Benchmark Script

## Overview

The `benchmark_graph_queries.sh` script measures the performance of the precalculated graph data queries used by the `/api/graph` endpoint.

## Usage

```bash
# From the backend directory
./scripts/benchmark_graph_queries.sh [DATABASE_URL]
```

If `DATABASE_URL` is not provided, the script will:
1. Try to use `$DATABASE_URL` from environment
2. Fall back to constructing URL from `.env` file variables

## Requirements

- `psql` (PostgreSQL client) must be installed
- `bc` (calculator) for averaging results
- Database must be populated with graph data (run precalculation first)

## What It Tests

The benchmark runs 5 tests covering the main query patterns:

1. **GetPrecalculatedGraphDataCappedAll**: Full query with top 20K nodes and up to 50K links
2. **GetPrecalculatedGraphDataCappedFiltered**: Type-filtered query (subreddit, user)
3. **Node Selection Only**: Just selecting top nodes without links
4. **Link Selection**: Finding links between a set of nodes
5. **Type-Filtered Nodes**: Selecting nodes of a specific type

Each test runs 5 iterations and reports:
- Individual run times
- Average execution time
- Index usage statistics
- Table statistics

## Example Output

```
Graph Query Performance Benchmark
==================================

Database: reddit_cluster
Node count: 45123
Link count: 123456

Running: Test 1: GetPrecalculatedGraphDataCappedAll
-------------------------------------------
Iterations: 5
  Run 1: 45.234ms
  Run 2: 43.567ms
  Run 3: 44.123ms
  Run 4: 43.890ms
  Run 5: 44.456ms
  Average: 44.254ms

[... more tests ...]

Index Usage Statistics:
 schemaname | tablename   | indexname                        | scans | tuples_read
------------+-------------+----------------------------------+-------+-------------
 public     | graph_nodes | idx_graph_nodes_val_numeric      | 25    | 100000
 public     | graph_links | idx_graph_links_source           | 15    | 75000
 ...
```

## Before/After Migration Testing

To test the impact of a new migration:

### 1. Baseline (Before Migration)

```bash
# Run benchmark before applying migration
./scripts/benchmark_graph_queries.sh > results_before.txt

# Save index stats
psql "$DATABASE_URL" -c "SELECT * FROM pg_stat_user_indexes WHERE tablename IN ('graph_nodes', 'graph_links');" > indexes_before.txt
```

### 2. Apply Migration

```bash
make migrate-up
```

### 3. After Migration

```bash
# Reset stats to get clean measurements
psql "$DATABASE_URL" -c "SELECT pg_stat_reset();"

# Run benchmark after migration
./scripts/benchmark_graph_queries.sh > results_after.txt

# Compare results
diff results_before.txt results_after.txt
```

## Interpreting Results

### Good Signs
- **Lower execution times**: Queries complete faster
- **Index scans**: Queries use indexes rather than sequential scans
- **Consistent times**: Little variance between runs (indicates stable performance)

### Warning Signs
- **Higher execution times**: Query performance degraded
- **Sequential scans**: Postgres is not using indexes
- **High variance**: Times vary widely (may indicate resource contention)

### Typical Performance (for reference)

With proper indexes on a dataset of ~50K nodes and ~150K links:

| Query | Expected Time |
|-------|---------------|
| Full capped (20K/50K) | 40-100ms |
| Filtered (subreddit+user) | 30-80ms |
| Node selection only | 10-30ms |
| Link selection | 20-60ms |
| Type-filtered nodes | 15-40ms |

Times will vary based on:
- Dataset size
- Hardware (CPU, disk I/O)
- Database configuration
- Concurrent load

## Integration with CI/CD

You can use this script in CI to detect performance regressions:

```bash
# In your CI pipeline
./scripts/benchmark_graph_queries.sh | tee benchmark_results.txt

# Extract average times and fail if they exceed thresholds
# (example - adapt to your needs)
avg_time=$(grep "Average:" benchmark_results.txt | head -1 | grep -oP '\d+\.\d+')
if (( $(echo "$avg_time > 200" | bc -l) )); then
  echo "ERROR: Query too slow (${avg_time}ms > 200ms threshold)"
  exit 1
fi
```

## Troubleshooting

### "Cannot connect to database"

Check your database connection:
```bash
psql "$DATABASE_URL" -c "SELECT 1"
```

### "No successful runs"

The query may have failed. Run manually to see the error:
```bash
psql "$DATABASE_URL" -c "EXPLAIN ANALYZE <your-query>"
```

### Slow queries

If queries are consistently slow:
1. Check if indexes exist: `\d graph_nodes` in psql
2. Run `VACUUM ANALYZE graph_nodes; VACUUM ANALYZE graph_links;`
3. Check for table bloat or missing indexes
4. Review query execution plans

## Related Documentation

- [Performance Documentation](../../docs/perf.md)
- [Graph Performance Migration](../../docs/graph-performance-migration.md)
- [Developer Guide](../../docs/developer-guide.md)
