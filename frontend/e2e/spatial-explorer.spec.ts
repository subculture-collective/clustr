import { expect, test, type Page } from '@playwright/test';

const world = {
  revision_id: 'rev-42',
  nodes: [
    { id: 'c:alpha', name: 'Alpha', type: 'community', val: 120, x: -120, y: -60, z: -80 },
    { id: 'c:beta', name: 'Beta', type: 'community', val: 95, x: 140, y: -20, z: 40 },
    { id: 'c:gamma', name: 'Gamma', type: 'community', val: 80, x: 10, y: 150, z: 130 },
    { id: 'c:delta', name: 'Delta', type: 'community', val: 65, x: -30, y: -140, z: 170 },
  ],
  links: [
    { source: 'c:alpha', target: 'c:beta', relation: 'route', weight: 12 },
    { source: 'c:beta', target: 'c:gamma', relation: 'route', weight: 8 },
    { source: 'c:gamma', target: 'c:delta', relation: 'route', weight: 6 },
  ],
};

async function mockSpatialWorld(page: Page) {
  await page.route('**/api/graph/manifest', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ revision_id: 'rev-42', bounds: { min_x: -150, max_x: 150, min_y: -150, max_y: 160, min_z: -100, max_z: 180 } }),
  }));
  await page.route('**/api/graph/overview**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(world),
  }));
}

test('renders a meaningful revision-pinned universe and exposes analyst mode', async ({ page }) => {
  const browserErrors: string[] = [];
  page.on('console', message => { if (message.type() === 'error') browserErrors.push(message.text()); });
  page.on('pageerror', error => browserErrors.push(error.message));
  await mockSpatialWorld(page);
  await page.goto('/');

  await expect(page.getByRole('button', { name: 'Universe', exact: true })).toBeVisible();
  const canvas = page.locator('[role="application"] canvas').first();
  await expect(canvas).toBeVisible();
  await expect(page.locator('[role="application"]')).toHaveAttribute('data-visible-node-count', '4');
  await expect.poll(async () => canvas.evaluate(element => element.toDataURL().length)).toBeGreaterThan(1000);
  await expect.poll(async () => canvas.evaluate(element => new Promise<number>(resolve => requestAnimationFrame(() => {
    const gl = element.getContext('webgl2') || element.getContext('webgl');
    if (!gl) return resolve(0);
    const pixels = new Uint8Array(element.width * element.height * 4);
    gl.readPixels(0, 0, element.width, element.height, gl.RGBA, gl.UNSIGNED_BYTE, pixels);
    let luminousSignalPixels = 0;
    for (let index = 0; index < pixels.length; index += 16) {
      if (pixels[index + 1] > 100 && pixels[index + 1] > pixels[index] * 1.25 && pixels[index + 1] > pixels[index + 2] * 1.25) luminousSignalPixels++;
    }
    resolve(luminousSignalPixels);
  })))).toBeGreaterThan(20);
  await expect(page.getByText('Error loading graph')).toHaveCount(0);

  const sceneBounds = await canvas.boundingBox();
  expect(sceneBounds?.width).toBeGreaterThanOrEqual(1200);
  expect(sceneBounds?.height).toBeGreaterThanOrEqual(700);

  const artifactDirectory = process.env.CLUSTR_VISUAL_DIR;
  if (artifactDirectory) {
    await page.waitForTimeout(500); // allow the local SDF font atlas to finish its first sync for the review artifact
    await page.screenshot({ path: `${artifactDirectory}/clustr-observatory-explore.png`, fullPage: true });
  }

  await page.getByRole('button', { name: /Field guide/i }).click();
  await expect(page.getByRole('dialog', { name: /living map of communities/i })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'From crawl to constellation' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Why objects end up where they do' })).toBeVisible();
  if (artifactDirectory) await page.screenshot({ path: `${artifactDirectory}/clustr-field-guide.png`, fullPage: true });
  await page.getByRole('button', { name: 'Enter the universe' }).click();

  await page.getByRole('button', { name: 'Analyst' }).click();
  await expect(page.getByRole('complementary', { name: 'Graph controls sidebar' })).toBeVisible();
  expect(browserErrors).toEqual([]);

  if (artifactDirectory) await page.screenshot({ path: `${artifactDirectory}/clustr-observatory-desktop.png`, fullPage: true });
});

test('keeps the universe and primary travel controls usable on mobile', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockSpatialWorld(page);
  await page.goto('/');

  const canvas = page.locator('[role="application"] canvas').first();
  await expect(canvas).toBeVisible();
  const sceneBounds = await canvas.boundingBox();
  expect(sceneBounds?.width).toBeGreaterThanOrEqual(390);
  expect(sceneBounds?.height).toBeGreaterThanOrEqual(820);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await expect(page.getByRole('navigation', { name: 'Primary views' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Universe', exact: true })).toBeVisible();

  const artifactDirectory = process.env.CLUSTR_VISUAL_DIR;
  if (artifactDirectory) await page.screenshot({ path: `${artifactDirectory}/clustr-observatory-mobile.png`, fullPage: true });
});
