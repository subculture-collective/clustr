# Performance Benchmarks

Automated performance benchmark suite for Reddit Cluster Map, measuring rendering FPS, data load times, and physics simulation performance with regression detection.

## Overview

The benchmark suite tests application performance across different dataset sizes:
- **1k nodes**: ~1,000 nodes with ~2,500 links
- **10k nodes**: ~10,000 nodes with ~25,000 links  
- **50k nodes**: ~50,000 nodes with ~125,000 links
- **100k nodes**: ~100,000 nodes with ~250,000 links (optional, disabled by default for CI speed)

## Metrics Measured

For each dataset size, the benchmarks measure:

- **Render Time**: Time from navigation to first render complete (ms)
- **Steady-State FPS**: Frames per second after 5-second warmup period
- **Data Parse Time**: Time to parse and process JSON graph data (ms)
- **Physics Warmup Time**: Time for physics simulation to stabilize (ms)
- **Memory Usage**: JavaScript heap size during steady-state rendering (MB)

## Running Benchmarks Locally

### Prerequisites

```bash
# From the frontend directory
npm ci
npx playwright install chromium
```

### Generate Test Fixtures

Test fixtures are generated programmatically but are gitignored due to size. Generate them before running benchmarks:

```bash
npm run benchmark:generate-fixtures
```

This creates fixture files in `benchmarks/fixtures/`:
- `graph-1k.json`
- `graph-10k.json`
- `graph-50k.json`
- `graph-100k.json`

### Run Benchmarks

```bash
# Run all benchmarks
npm run benchmark

# Results are saved to:
# - benchmarks/results/benchmark-<timestamp>.json
# - benchmarks/results/benchmark-latest.json
```

Benchmarks run with the dev server automatically started. Expected runtime:
- 1k: ~10 seconds
- 10k: ~20 seconds
- 50k: ~40 seconds
- 100k: ~60 seconds (if enabled)

## Baseline Management

### Creating a Baseline

The first time you run benchmarks, establish a baseline for comparison:

```bash
# Run benchmarks
npm run benchmark

# Copy latest results as baseline
cp benchmarks/results/benchmark-latest.json benchmarks/results/baseline.json
```

### Comparing Against Baseline

```bash
# Run benchmarks
npm run benchmark

# Compare with baseline (fails on regression)
npm run benchmark:compare
```

The comparison script checks for:
- **FPS regression**: >10% drop in steady-state FPS
- **Render time regression**: >20% increase in initial render time
- **Memory regression**: >30% increase in memory usage

### Updating Baseline

After intentional changes that affect performance, update the baseline:

```bash
# Run benchmarks with new changes
npm run benchmark

# Review results to ensure they're acceptable
cat benchmarks/results/benchmark-latest.json

# Update baseline if satisfied
cp benchmarks/results/benchmark-latest.json benchmarks/results/baseline.json

# Commit the new baseline
git add benchmarks/results/baseline.json
git commit -m "chore: update performance baseline"
```

The committed `baseline.json` is the one CI compares against: pull request runs cannot read the `benchmark-baseline` artifact that main pushes upload. CI runs on two Gitea runners of different speeds (Kvant and Kuznya), so a copy of one run's results makes the other runner fail. The `gitea-2cpu-v2` baseline was built from the per-run tables that CI posted on pull requests between November 3 and 6, 2026: render time is the slower runner's 75th percentile, FPS and parse time its median, and memory the median across both runners. Check a new baseline by passing earlier runs to `npx tsx benchmarks/compare.ts <baseline> <run>`.

## CI Integration

Benchmarks run automatically in GitHub Actions on:
- Push to `main` branch
- Pull requests to `main`
- Manual workflow dispatch

### CI Workflow

1. **Run Benchmarks**: Executes benchmark suite on all fixture sizes
2. **Upload Results**: Stores results as artifacts (90-day retention)
3. **Compare with Baseline**: Downloads stored baseline and compares
4. **Report Results**: Adds comment to PR with benchmark table
5. **Fail on Regression**: Returns exit code 1 if performance degraded
6. **Store Baseline** (main only): Updates baseline artifact on main branch

### Viewing CI Results

**In Pull Requests:**
- Check the "Performance Benchmarks" status check
- View benchmark table in automated PR comment
- Click "Details" to see full job summary with comparison

**On Main Branch:**
- Baseline is automatically updated from latest results
- New baseline stored as artifact for future comparisons

### Job Summaries

GitHub Actions job summaries show:
- Comparison table with percentage changes
- Pass/fail status per fixture
- Detailed regression information
- Full results table with absolute values

## Interpreting Results

### Good Performance
```
✅ 1k nodes: 60 FPS, 150ms render time, 50MB memory
✅ 10k nodes: 45 FPS, 800ms render time, 150MB memory
✅ 50k nodes: 30 FPS, 3000ms render time, 400MB memory
```

### Warning Signs
```
⚠️  FPS < 20 for any dataset size
⚠️  Render time > 5000ms for 50k nodes
⚠️  Memory usage > 800MB for 50k nodes
```

### Regression Examples

**FPS Regression:**
```
❌ 10k: FPS dropped by 12.5% (45.0 → 39.4 FPS)
```

**Render Time Regression:**
```
❌ 10k: Render time increased by 25.3% (800ms → 1002ms)
```

**Memory Regression:**
```
❌ 50k: Memory usage increased by 35.7% (400.0MB → 542.8MB)
```

## Troubleshooting

### Benchmarks Timing Out

Increase timeout in `playwright.benchmark.config.ts`:

```typescript
timeout: 180 * 1000, // 3 minutes
```

### Inconsistent Results

Performance can vary based on:
- System load
- Browser state
- Background processes
- CPU throttling

For more consistent results:
- Close other applications
- Run multiple times and average
- Use CI for definitive measurements

### Missing Fixtures

```bash
# Regenerate fixtures if missing
npm run benchmark:generate-fixtures
```

### Baseline Not Found

```bash
# Create initial baseline
npm run benchmark
cp benchmarks/results/benchmark-latest.json benchmarks/results/baseline.json
```

## File Structure

```
benchmarks/
├── fixtures/               # Test data (gitignored)
│   ├── generateFixtures.ts # Fixture generator script
│   ├── graph-1k.json      # 1k node dataset
│   ├── graph-10k.json     # 10k node dataset
│   ├── graph-50k.json     # 50k node dataset
│   └── graph-100k.json    # 100k node dataset (optional)
├── results/               # Benchmark outputs (gitignored except baseline)
│   ├── baseline.json      # Stored baseline (committed)
│   ├── benchmark-latest.json
│   └── benchmark-*.json   # Timestamped results
├── utils/
│   └── metrics.ts         # Performance measurement utilities
├── compare.ts             # Regression detection script
└── performance.spec.ts    # Playwright benchmark tests
```

## Configuration

### Playwright Config

See `playwright.benchmark.config.ts`:
- Fixed viewport: 1280×720
- Sequential execution (workers: 1)
- No retries for deterministic results
- Chrome-only for consistent measurements
- Performance flags enabled

### Test Fixtures

Edit `fixtures/generateFixtures.ts` to adjust:
- Node counts: `FIXTURE_CONFIGS`
- Link density: `linkDensity` (default: 2.5)
- Node type distribution: ratios in `generateGraphData()`

### Regression Thresholds

Edit `compare.ts` to adjust failure thresholds:
- FPS regression: `FPS_REGRESSION_THRESHOLD` (default: 10%)
- Render time: 20% threshold
- Memory usage: 30% threshold

## Best Practices

1. **Run Before/After**: Always benchmark before and after performance changes
2. **Commit Baselines**: Track baseline.json in git for team consistency
3. **Review Trends**: Monitor benchmark history artifacts in GitHub
4. **Isolate Changes**: Run on clean branches to isolate performance impact
5. **Document Changes**: Note expected performance impact in PRs
6. **Update Baselines**: Update after confirmed performance improvements

## Related Documentation

- [Testing Guide](../TESTING.md) - Unit and E2E testing
- [Bundle Size](../BUNDLE_SIZE.md) - Bundle size monitoring
- [Issue #143](https://github.com/subculture-collective/reddit-cluster-map/issues/143) - Epic: Testing, CI/CD & Quality
- [Issue #138](https://github.com/subculture-collective/reddit-cluster-map/issues/138) - v2.0 Roadmap

## Support

For issues or questions about benchmarks:
1. Check this README
2. Review recent benchmark runs in Actions
3. Open an issue with benchmark results attached
