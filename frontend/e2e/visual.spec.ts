/**
 * Visual Regression Tests for Graph Rendering
 * 
 * Tests screenshot consistency across:
 * - Different graph sizes (empty, small, large)
 * - Different view modes (3D, 2D)
 * - Different themes (light, dark)
 * - Zoomed states
 * 
 * Run with: npm run test:visual
 * Update baselines with: npm run test:visual:update
 */

import { test, expect, type Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';
import { dirname } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Helper to load fixture data
function loadFixture(fixtureName: string) {
  const fixturePath = path.join(__dirname, 'fixtures', `visual-${fixtureName}.json`);
  return JSON.parse(fs.readFileSync(fixturePath, 'utf-8'));
}

// Helper to mock the API response with fixture data
// MUST be called BEFORE page.goto()
async function mockGraphAPI(page: Page, fixtureName: string) {
  const fixtureData = loadFixture(fixtureName);
  const revision = 'visual-test-revision';
  const catalog = 'visual-test-catalog';
  
  await page.route(/\/api\/graph(?:\/|\?|$)/, async (route) => {
    const requestUrl = new URL(route.request().url());
    const body = requestUrl.pathname.endsWith('/manifest')
      ? { revision_id: revision, spatial_catalog_id: catalog }
      : { ...fixtureData, revision_id: revision, spatial_catalog_id: catalog };

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
}

// Helper to wait for graph to be rendered and stable
async function waitForGraphStable(page: Page, timeout: number = 5000, minimumVisibleNodes: number = 0) {
  if (minimumVisibleNodes > 0) {
    await page.waitForFunction(
      minimum => {
        const graph = document.querySelector('[data-visible-node-count]');
        return Number(graph?.getAttribute('data-visible-node-count') ?? 0) >= minimum;
      },
      minimumVisibleNodes,
      { timeout: 30000 },
    );
  }

  // Wait for either 3D or 2D graph container to be visible
  try {
    await page.waitForSelector('canvas, svg', { timeout: 10000 });
  } catch {
    // If no canvas/svg found, might be empty graph state
    console.log('No graph canvas/svg found - might be empty graph');
  }
  
  // Wait additional time for physics to settle and rendering to stabilize
  await page.waitForTimeout(timeout);
}

async function expectRenderedGraphPixels(page: Page) {
  await expect.poll(async () => {
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
  }, { timeout: 10000 }).toBeGreaterThan(20);
}

// Helper to set theme using localStorage init script
// MUST be called BEFORE page.goto()
async function setTheme(page: Page, theme: 'light' | 'dark') {
  await page.addInitScript((t) => {
    localStorage.setItem('themeMode', t);
  }, theme);
}

test.describe('Visual Regression Tests', () => {
  // Configure tests to run sequentially for stability
  test.describe.configure({ mode: 'serial' });

  test.describe('Empty Graph State', () => {
    test('empty graph - light theme - 3D view', async ({ page }) => {
      await setTheme(page, 'light');
      await mockGraphAPI(page, 'empty');
      
      await page.goto('/');
      await waitForGraphStable(page, 2000);
      
      // Take screenshot of the entire page
      await expect(page).toHaveScreenshot('empty-light-3d.png', {
        fullPage: false,
      });
    });

    test('empty graph - dark theme - 3D view', async ({ page }) => {
      await setTheme(page, 'dark');
      await mockGraphAPI(page, 'empty');
      
      await page.goto('/');
      await waitForGraphStable(page, 2000);
      
      await expect(page).toHaveScreenshot('empty-dark-3d.png', {
        fullPage: false,
      });
    });
  });

  test.describe('Small Graph (100 nodes)', () => {
    test('small graph - light theme - 3D view', async ({ page }) => {
      await setTheme(page, 'light');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      await waitForGraphStable(page, 5000, 1);
      await expectRenderedGraphPixels(page);
      
      await expect(page).toHaveScreenshot('small-light-3d.png', {
        fullPage: false,
      });
    });

    test('small graph - dark theme - 3D view', async ({ page }) => {
      await setTheme(page, 'dark');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      await waitForGraphStable(page, 5000, 1);
      await expectRenderedGraphPixels(page);
      
      await expect(page).toHaveScreenshot('small-dark-3d.png', {
        fullPage: false,
      });
    });

    test('small graph - light theme - 2D view', async ({ page }) => {
      await setTheme(page, 'light');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      await waitForGraphStable(page, 5000);
      
      // Switch to 2D view - look for view mode button
      const viewModeButton = page.getByRole('button', { name: /2D/i }).first();
      if (await viewModeButton.isVisible().catch(() => false)) {
        await viewModeButton.click();
        await waitForGraphStable(page, 3000);
      } else {
        // Try alternate selector
        await page.evaluate(() => {
          localStorage.setItem('viewMode', '2d');
        });
        await page.reload();
        await waitForGraphStable(page, 5000);
      }
      
      await expect(page).toHaveScreenshot('small-light-2d.png', {
        fullPage: false,
      });
    });

    test('small graph - dark theme - 2D view', async ({ page }) => {
      await setTheme(page, 'dark');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      
      // Set 2D view mode
      await page.evaluate(() => {
        localStorage.setItem('viewMode', '2d');
      });
      await page.reload();
      
      await waitForGraphStable(page, 5000);
      
      await expect(page).toHaveScreenshot('small-dark-2d.png', {
        fullPage: false,
      });
    });
  });

  test.describe('Large Graph (10k nodes)', () => {
    test('large graph - light theme - 3D view', async ({ page }) => {
      await setTheme(page, 'light');
      await mockGraphAPI(page, 'large');
      
      await page.goto('/');
      
      // Longer wait for large graphs to render
      await waitForGraphStable(page, 8000, 100);
      await expectRenderedGraphPixels(page);
      
      await expect(page).toHaveScreenshot('large-light-3d.png', {
        fullPage: false,
        timeout: 30000, // Longer timeout for large graphs
      });
    });

    test('large graph - dark theme - 3D view', async ({ page }) => {
      await setTheme(page, 'dark');
      await mockGraphAPI(page, 'large');
      
      await page.goto('/');
      await waitForGraphStable(page, 8000, 100);
      await expectRenderedGraphPixels(page);
      
      await expect(page).toHaveScreenshot('large-dark-3d.png', {
        fullPage: false,
        timeout: 30000,
      });
    });
  });

  test.describe('Zoomed Views', () => {
    test('small graph - zoomed in', async ({ page }) => {
      await setTheme(page, 'light');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      await waitForGraphStable(page, 5000, 1);
      await expectRenderedGraphPixels(page);
      
      // Simulate zoom by scrolling (wheel events)
      const canvas = page.locator('canvas').first();
      if (await canvas.isVisible().catch(() => false)) {
        await canvas.hover();
        // Zoom in with wheel events
        await canvas.evaluate((el) => {
          const event = new WheelEvent('wheel', {
            deltaY: -300,
            bubbles: true,
            cancelable: true,
          });
          el.dispatchEvent(event);
        });
        await page.waitForTimeout(2000); // Wait for zoom animation
      }
      
      await expect(page).toHaveScreenshot('small-light-3d-zoomed.png', {
        fullPage: false,
      });
    });
  });

  test.describe('Dashboard View', () => {
    test('small graph - dashboard view - light theme', async ({ page }) => {
      await setTheme(page, 'light');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      
      // Switch to dashboard view
      await page.evaluate(() => {
        localStorage.setItem('viewMode', 'dashboard');
      });
      await page.reload();
      
      await waitForGraphStable(page, 3000);
      
      await expect(page).toHaveScreenshot('small-light-dashboard.png', {
        fullPage: false,
      });
    });

    test('small graph - dashboard view - dark theme', async ({ page }) => {
      await setTheme(page, 'dark');
      await mockGraphAPI(page, 'small');
      
      await page.goto('/');
      
      // Switch to dashboard view
      await page.evaluate(() => {
        localStorage.setItem('viewMode', 'dashboard');
      });
      await page.reload();
      
      await waitForGraphStable(page, 3000);
      
      await expect(page).toHaveScreenshot('small-dark-dashboard.png', {
        fullPage: false,
      });
    });
  });
});
