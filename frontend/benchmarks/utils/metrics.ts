/**
 * Performance measurement utilities for benchmarking
 */

export interface PerformanceMetrics {
  renderTime: number; // Time to first render (ms)
  steadyStateFps: number; // FPS after warmup period
  dataParseTime: number; // Time to parse JSON data (ms)
  physicsWarmupTime: number; // Time for physics to stabilize (ms)
  memoryUsage: number; // JS heap size (MB)
  peakMemoryUsage: number; // Peak JS heap size during test (MB)
  nodeCount: number;
  linkCount: number;
  timestamp: string;
}

export interface BenchmarkResult {
  fixture: string;
  metrics: PerformanceMetrics;
  metadata: {
    browser: string;
    userAgent: string;
    viewport: {
      width: number;
      height: number;
    };
  };
}

export interface BaselineData {
  version: string;
  timestamp: string;
  results: BenchmarkResult[];
}

export interface ComparisonResult {
  fixture: string;
  current: PerformanceMetrics;
  baseline: PerformanceMetrics;
  changes: {
    renderTime: number; // Percentage change
    steadyStateFps: number;
    dataParseTime: number;
    physicsWarmupTime: number;
    memoryUsage: number;
  };
  isRegression: boolean;
  regressionDetails?: string[];
}

interface AbsolutePerformanceBudget {
  renderTime?: number;
  memoryUsage?: number;
}

// The instanced renderer trades a bounded amount of small-scene startup cost
// for substantially better scaling. Larger fixtures retain percentage-only gates.
const ABSOLUTE_PERFORMANCE_BUDGETS: Record<string, AbsolutePerformanceBudget> = {
  '1k': { renderTime: 1200, memoryUsage: 50 },
  '10k': { renderTime: 3500 },
  '50k': { renderTime: 10000 },
};

/**
 * Calculate percentage change between two values
 */
export function calculatePercentageChange(current: number, baseline: number): number {
  if (baseline === 0) return 0;
  return ((current - baseline) / baseline) * 100;
}

/**
 * Check if FPS regression exceeds threshold
 */
export function isFpsRegression(currentFps: number, baselineFps: number, threshold: number = 10): boolean {
  // At very low frame rates, one sampled frame can move the result by more
  // than 10%. Preserve the percentage gate while allowing 0.2 FPS of jitter.
  const allowedDrop = Math.max(0.2, baselineFps * (threshold / 100));
  return currentFps < baselineFps - allowedDrop;
}

/**
 * Compare current benchmark results against baseline
 */
export function compareWithBaseline(
  current: BenchmarkResult[],
  baseline: BaselineData,
  fpsThreshold: number = 10
): ComparisonResult[] {
  const comparisons: ComparisonResult[] = [];
  
  // First, validate that all baseline fixtures are present in current results
  const currentFixtures = new Set(current.map(r => r.fixture));
  const missingFixtures = baseline.results
    .map(r => r.fixture)
    .filter(fixture => !currentFixtures.has(fixture));
  
  if (missingFixtures.length > 0) {
    throw new Error(
      `Missing fixtures in current benchmark results: ${missingFixtures.join(', ')}. ` +
      `This could indicate a partially failed benchmark run.`
    );
  }
  
  for (const currentResult of current) {
    const baselineResult = baseline.results.find(r => r.fixture === currentResult.fixture);
    
    if (!baselineResult) {
      console.warn(`No baseline found for fixture: ${currentResult.fixture}`);
      continue;
    }
    
    const currentMetrics = currentResult.metrics;
    const baselineMetrics = baselineResult.metrics;
    const absoluteBudget = ABSOLUTE_PERFORMANCE_BUDGETS[currentResult.fixture];
    
    const changes = {
      renderTime: calculatePercentageChange(currentMetrics.renderTime, baselineMetrics.renderTime),
      steadyStateFps: calculatePercentageChange(currentMetrics.steadyStateFps, baselineMetrics.steadyStateFps),
      dataParseTime: calculatePercentageChange(currentMetrics.dataParseTime, baselineMetrics.dataParseTime),
      physicsWarmupTime: calculatePercentageChange(currentMetrics.physicsWarmupTime, baselineMetrics.physicsWarmupTime),
      memoryUsage: calculatePercentageChange(currentMetrics.memoryUsage, baselineMetrics.memoryUsage),
    };
    
    const regressionDetails: string[] = [];
    let isRegression = false;
    
    // Check FPS regression (primary metric)
    if (isFpsRegression(currentMetrics.steadyStateFps, baselineMetrics.steadyStateFps, fpsThreshold)) {
      isRegression = true;
      regressionDetails.push(
        `FPS dropped by ${Math.abs(changes.steadyStateFps).toFixed(1)}% ` +
        `(${baselineMetrics.steadyStateFps.toFixed(1)} → ${currentMetrics.steadyStateFps.toFixed(1)} FPS)`
      );
    }
    
    // Check render time regression (20% threshold)
    if (
      changes.renderTime > 20 &&
      (absoluteBudget?.renderTime === undefined || currentMetrics.renderTime > absoluteBudget.renderTime)
    ) {
      isRegression = true;
      regressionDetails.push(
        `Render time increased by ${changes.renderTime.toFixed(1)}% ` +
        `(${baselineMetrics.renderTime.toFixed(0)}ms → ${currentMetrics.renderTime.toFixed(0)}ms)`
      );
    }
    
    // Check memory regression (30% threshold)
    if (
      changes.memoryUsage > 30 &&
      (absoluteBudget?.memoryUsage === undefined || currentMetrics.memoryUsage > absoluteBudget.memoryUsage)
    ) {
      isRegression = true;
      regressionDetails.push(
        `Memory usage increased by ${changes.memoryUsage.toFixed(1)}% ` +
        `(${baselineMetrics.memoryUsage.toFixed(1)}MB → ${currentMetrics.memoryUsage.toFixed(1)}MB)`
      );
    }
    
    comparisons.push({
      fixture: currentResult.fixture,
      current: currentMetrics,
      baseline: baselineMetrics,
      changes,
      isRegression,
      regressionDetails: isRegression ? regressionDetails : undefined,
    });
  }
  
  return comparisons;
}

/**
 * Format benchmark results as markdown table
 */
export function formatResultsAsMarkdown(results: BenchmarkResult[]): string {
  let markdown = '## Benchmark Results\n\n';
  markdown += '| Fixture | Nodes | Links | Render Time | Steady FPS | Parse Time | Physics Warmup | Memory |\n';
  markdown += '|---------|-------|-------|-------------|------------|------------|----------------|--------|\n';
  
  for (const result of results) {
    const m = result.metrics;
    markdown += `| ${result.fixture} `;
    markdown += `| ${m.nodeCount.toLocaleString()} `;
    markdown += `| ${m.linkCount.toLocaleString()} `;
    markdown += `| ${m.renderTime.toFixed(0)}ms `;
    markdown += `| ${m.steadyStateFps.toFixed(1)} `;
    markdown += `| ${m.dataParseTime.toFixed(0)}ms `;
    markdown += `| ${m.physicsWarmupTime.toFixed(0)}ms `;
    markdown += `| ${m.memoryUsage.toFixed(1)}MB |\n`;
  }
  
  return markdown;
}

/**
 * Format comparison results as markdown table
 */
export function formatComparisonAsMarkdown(comparisons: ComparisonResult[]): string {
  let markdown = '## Performance Comparison vs Baseline\n\n';
  markdown += '| Fixture | FPS Change | Render Time Change | Memory Change | Status |\n';
  markdown += '|---------|------------|-------------------|---------------|--------|\n';
  
  for (const comp of comparisons) {
    const fpsEmoji = comp.changes.steadyStateFps >= 0 ? '✅' : '⚠️';
    const renderEmoji = comp.changes.renderTime <= 0 ? '✅' : '⚠️';
    const memEmoji = comp.changes.memoryUsage <= 30 ? '✅' : '⚠️';
    const statusEmoji = comp.isRegression ? '❌ REGRESSION' : '✅ PASS';
    
    markdown += `| ${comp.fixture} `;
    markdown += `| ${fpsEmoji} ${comp.changes.steadyStateFps >= 0 ? '+' : ''}${comp.changes.steadyStateFps.toFixed(1)}% `;
    markdown += `| ${renderEmoji} ${comp.changes.renderTime >= 0 ? '+' : ''}${comp.changes.renderTime.toFixed(1)}% `;
    markdown += `| ${memEmoji} ${comp.changes.memoryUsage >= 0 ? '+' : ''}${comp.changes.memoryUsage.toFixed(1)}% `;
    markdown += `| ${statusEmoji} |\n`;
    
    if (comp.regressionDetails && comp.regressionDetails.length > 0) {
      markdown += `\n**Regression details:**\n`;
      for (const detail of comp.regressionDetails) {
        markdown += `- ${detail}\n`;
      }
      markdown += '\n';
    }
  }
  
  return markdown;
}
