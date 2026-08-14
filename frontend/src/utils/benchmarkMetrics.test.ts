import { describe, expect, it } from 'vitest';

import {
  type BaselineData,
  type BenchmarkResult,
  compareWithBaseline,
} from '../../benchmarks/utils/metrics';

function result(
  fixture: string,
  renderTime: number,
  steadyStateFps: number,
  memoryUsage: number,
): BenchmarkResult {
  return {
    fixture,
    metrics: {
      renderTime,
      steadyStateFps,
      dataParseTime: 10,
      physicsWarmupTime: 20,
      memoryUsage,
      peakMemoryUsage: memoryUsage,
      nodeCount: 1000,
      linkCount: 2000,
      timestamp: '2026-08-14T00:00:00Z',
    },
    metadata: {
      browser: 'chromium',
      userAgent: 'test',
      viewport: { width: 1280, height: 720 },
    },
  };
}

function baseline(entry: BenchmarkResult): BaselineData {
  return {
    version: 'test',
    timestamp: '2026-08-14T00:00:00Z',
    results: [entry],
  };
}

describe('benchmark regression policy', () => {
  it('allows 1k percentage changes within the absolute budgets', () => {
    const previous = result('1k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('1k', 900, 60, 40)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(false);
  });

  it('rejects 1k render time above its absolute budget', () => {
    const previous = result('1k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('1k', 1201, 60, 40)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(true);
    expect(comparison.regressionDetails?.[0]).toContain('Render time increased');
  });

  it('rejects 1k memory usage above its absolute budget', () => {
    const previous = result('1k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('1k', 900, 60, 51)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(true);
    expect(comparison.regressionDetails?.[0]).toContain('Memory usage increased');
  });

  it('allows 10k startup changes within the absolute render budget', () => {
    const previous = result('10k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('10k', 1400, 60, 25)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(false);
  });

  it('rejects 10k startup above the absolute render budget', () => {
    const previous = result('10k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('10k', 3501, 60, 25)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(true);
    expect(comparison.regressionDetails?.[0]).toContain('Render time increased');
  });

  it('keeps percentage gates for fixtures without absolute budgets', () => {
    const previous = result('100k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('100k', 700, 60, 40)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(true);
    expect(comparison.regressionDetails).toHaveLength(2);
  });

  it('always rejects an FPS regression', () => {
    const previous = result('1k', 500, 60, 25);
    const [comparison] = compareWithBaseline(
      [result('1k', 500, 50, 25)],
      baseline(previous),
    );

    expect(comparison.isRegression).toBe(true);
    expect(comparison.regressionDetails?.[0]).toContain('FPS dropped');
  });
});
