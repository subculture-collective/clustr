/**
 * Performance benchmarks for Reddit Cluster Map
 * 
 * Measures rendering performance, FPS, memory usage, and physics simulation
 * across different dataset sizes (1k, 10k, 50k, 100k nodes).
 * 
 * Run with: npm run benchmark
 * Results are stored in benchmarks/results/
 */

import { test, expect, type Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';
import { dirname } from 'path';
import type { BenchmarkResult, PerformanceMetrics } from './utils/metrics';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const FIXTURES = ['1k', '10k', '50k'];
const WARMUP_TIME_MS = 5000; // Time to wait for physics stabilization
const FPS_MEASUREMENT_DURATION_MS = 3000; // Duration to measure FPS

// Helper to load fixture data
function loadFixture(fixtureName: string) {
  const fixturePath = path.join(__dirname, 'fixtures', `graph-${fixtureName}.json`);
  return JSON.parse(fs.readFileSync(fixturePath, 'utf-8'));
}

// Helper to measure FPS using requestAnimationFrame
async function measureFPS(page: Page, durationMs: number): Promise<number> {
  return await page.evaluate(async (duration: number) => {
    return new Promise<number>((resolve) => {
      let frameCount = 0;
      const startTime = performance.now();
      
      function countFrame() {
        frameCount++;
        const elapsed = performance.now() - startTime;
        
        if (elapsed < duration) {
          requestAnimationFrame(countFrame);
        } else {
          const fps = (frameCount / elapsed) * 1000;
          resolve(fps);
        }
      }
      
      requestAnimationFrame(countFrame);
    });
  }, durationMs);
}

// Helper to get memory metrics
async function getMemoryMetrics(page: Page) {
  return await page.evaluate(() => {
    const performanceWithMemory = performance as Performance & {
      memory?: { usedJSHeapSize: number; totalJSHeapSize: number };
    };
    if (performanceWithMemory.memory) {
      return {
        usedJSHeapSize: performanceWithMemory.memory.usedJSHeapSize / 1024 / 1024, // Convert to MB
        totalJSHeapSize: performanceWithMemory.memory.totalJSHeapSize / 1024 / 1024,
      };
    }
    return { usedJSHeapSize: 0, totalJSHeapSize: 0 };
  });
}

async function countRenderedGraphPixels(page: Page): Promise<number> {
  return page.locator('[role="application"] canvas').evaluate(async (element) => {
    const canvas = element as HTMLCanvasElement;
    const gl = canvas.getContext('webgl2') ?? canvas.getContext('webgl');
    if (!gl) return 0;
    return new Promise<number>(resolve => requestAnimationFrame(() => {
      const pixels = new Uint8Array(canvas.width * canvas.height * 4);
      gl.readPixels(0, 0, canvas.width, canvas.height, gl.RGBA, gl.UNSIGNED_BYTE, pixels);
      const background = [pixels[0], pixels[1], pixels[2]];
      let signalPixels = 0;
      for (let index = 0; index < pixels.length; index += 4) {
        const difference = Math.abs(pixels[index] - background[0])
          + Math.abs(pixels[index + 1] - background[1])
          + Math.abs(pixels[index + 2] - background[2]);
        if (difference > 30) signalPixels++;
      }
      resolve(signalPixels);
    }));
  });
}

test.describe('Performance Benchmarks', () => {
  // Run benchmarks sequentially to avoid resource contention
  test.describe.configure({ mode: 'serial' });
  
  const results: BenchmarkResult[] = [];
  
  for (const fixture of FIXTURES) {
    test(`benchmark ${fixture} nodes`, async ({ page, browserName }) => {
      console.log(`\n🔬 Running benchmark for ${fixture} fixture...`);
      
      const fixtureData = loadFixture(fixture);
      const nodeCount = fixtureData.nodes.length;
      const linkCount = fixtureData.links.length;
      const linkedNodeCount = new Set(
        fixtureData.links.flatMap((link: { source: string; target: string }) => [link.source, link.target]),
      ).size;
      
      console.log(`   Nodes: ${nodeCount.toLocaleString()}, Links: ${linkCount.toLocaleString()}`);
      
      // Mark the start time
      const benchmarkStart = Date.now();
      
      // Intercept API calls and return fixture data
      let manifestRequests = 0;
      let graphRequests = 0;
      await page.route(/\/api\/graph(?:\/|\?|$)/, async (route) => {
        const requestUrl = new URL(route.request().url());
        const revision = 'benchmark-revision';
        const catalog = 'benchmark-catalog';
        const isManifest = requestUrl.pathname.endsWith('/manifest');
        if (isManifest) manifestRequests++;
        else graphRequests++;
        const body = isManifest
          ? { revision_id: revision, spatial_catalog_id: catalog }
          : { ...fixtureData, revision_id: revision, spatial_catalog_id: catalog };

        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(body),
        });
      });
      
      // Add performance marks for measurement as early as possible in the page lifecycle
      await page.addInitScript(() => {
        localStorage.setItem('enableAdaptiveLOD', 'false');
        performance.mark('navigation-start');
      });

      // Navigate to the app
      await page.goto('/?adaptiveLOD=0&f_subreddit=1&f_user=1&f_post=1&f_comment=1');
      
      // Wait for the page to load
      await page.waitForLoadState('networkidle');
      expect(manifestRequests).toBeGreaterThan(0);
      expect(graphRequests).toBeGreaterThan(0);
      const graph = page.locator('[data-visible-node-count]');
      await expect(graph).toHaveAttribute('data-visible-node-count', String(linkedNodeCount), {
        timeout: 30000,
      });
      // Rendering visibility is a setup condition, while renderTime below is the
      // measured budget. Large fixtures need extra headroom on two-core CI runners.
      await expect.poll(() => countRenderedGraphPixels(page), { timeout: 30000 }).toBeGreaterThan(20);
      
      // Measure time until UI is ready (not just JSON parse)
      const uiReadyStartTime = Date.now();
      await page.waitForFunction(
        () => {
          // Check if graph data is loaded
          const body = document.body.textContent || '';
          return !body.includes('Loading') || document.querySelector('canvas') !== null;
        },
        { timeout: 30000 }
      );
      const uiReadyTime = Date.now() - uiReadyStartTime;
      
      // Mark render complete
      await page.evaluate(() => {
        performance.mark('render-complete');
      });
      
      // Measure render time
      const renderTimeResult = await page.evaluate(() => {
        const measure = performance.measure('render-time', 'navigation-start', 'render-complete');
        return measure.duration;
      });
      
      console.log(`   ✓ UI ready in ${uiReadyTime}ms`);
      console.log(`   ✓ Initial render in ${renderTimeResult.toFixed(0)}ms`);
      
      // Wait for physics warmup
      console.log(`   ⏳ Waiting ${WARMUP_TIME_MS}ms for physics warmup...`);
      const physicsWarmupStart = Date.now();
      await page.waitForTimeout(WARMUP_TIME_MS);
      const physicsWarmupTime = Date.now() - physicsWarmupStart;
      
      // Measure steady-state FPS
      console.log(`   📊 Measuring FPS over ${FPS_MEASUREMENT_DURATION_MS}ms...`);
      const fps = await measureFPS(page, FPS_MEASUREMENT_DURATION_MS);
      
      // Get peak memory usage after warmup
      const peakMemory = await getMemoryMetrics(page);
      
      console.log(`   ✓ Steady-state FPS: ${fps.toFixed(1)}`);
      console.log(`   ✓ Memory usage: ${peakMemory.usedJSHeapSize.toFixed(1)}MB`);
      
      const metrics: PerformanceMetrics = {
        renderTime: renderTimeResult,
        steadyStateFps: fps,
        dataParseTime: uiReadyTime, // Time until UI is ready (includes parse + initial render)
        physicsWarmupTime,
        memoryUsage: peakMemory.usedJSHeapSize,
        peakMemoryUsage: peakMemory.usedJSHeapSize,
        nodeCount,
        linkCount,
        timestamp: new Date().toISOString(),
      };
      
      const result: BenchmarkResult = {
        fixture,
        metrics,
        metadata: {
          browser: browserName,
          userAgent: await page.evaluate(() => navigator.userAgent),
          viewport: {
            width: 1280,
            height: 720,
          },
        },
      };
      
      results.push(result);
      
      // Assert reasonable performance bounds
      // Only check for catastrophic failures (< 1 FPS) or extremely long render times
      // Regression detection is handled by the comparison script, not test assertions
      if (fps < 1) {
        console.warn(`   ⚠️  WARNING: Very low FPS detected (${fps.toFixed(1)})`);
      }
      expect(renderTimeResult).toBeLessThan(60000); // Max 60s initial render
      
      console.log(`   ✅ Benchmark complete (${Date.now() - benchmarkStart}ms total)\n`);
    });
  }
  
  // Save results after all benchmarks complete
  test.afterAll(async () => {
    const outputDir = path.join(__dirname, 'results');
    fs.mkdirSync(outputDir, { recursive: true });
    
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const outputPath = path.join(outputDir, `benchmark-${timestamp}.json`);
    
    const output = {
      version: process.env.npm_package_version || '0.1.0',
      timestamp: new Date().toISOString(),
      results,
    };
    
    fs.writeFileSync(outputPath, JSON.stringify(output, null, 2));
    console.log(`\n📝 Results saved to: ${outputPath}`);
    
    // Also save as "latest" for easy comparison
    const latestPath = path.join(outputDir, 'benchmark-latest.json');
    fs.writeFileSync(latestPath, JSON.stringify(output, null, 2));
    console.log(`📝 Latest results saved to: ${latestPath}\n`);
  });
});
