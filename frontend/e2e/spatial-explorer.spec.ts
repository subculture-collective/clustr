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
    body: JSON.stringify({ revision_id: 'rev-42', spatial_catalog_id: 'catalog-8', bounds: { min_x: -150, max_x: 150, min_y: -150, max_y: 160, min_z: -100, max_z: 180 } }),
  }));
  await page.route('**/api/graph/overview**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(world),
  }));
  await page.route('**/api/graph/community/**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ ...world, spatial_catalog_id: 'catalog-8' }),
  }));
  await page.route('**/api/graph/telemetry**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      revision_id: 'rev-42',
      spatial_catalog_id: 'catalog-8',
      totals: { entities: 7_066_559, links: 4_693_401, communities: 4, by_type: { subreddit: 100, user: 1_000, post: 50_000, comment: 7_015_459 } },
      top_subreddits: [{ id: 'subreddit_alpha', name: 'Alpha', subscribers: 500_000, activity_count: 100, unique_users: 40 }],
      top_users: [{ id: 'user_reader', name: 'Reader', posts: 2, comments: 8, activity_count: 10, distinct_communities: 3 }],
    }),
  }));
  await page.route('**/api/graph/communities**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      revision_id: 'rev-42',
      spatial_catalog_id: 'catalog-8',
      level: 0,
      communities: [
        ...world.nodes.map(node => ({ id: node.id, label: node.name, size: node.val, x: node.x, y: node.y, z: node.z })),
        { id: 'c:new:d8909f50aeb05ee6', label: 'HistoryMemes · Minecraft · Overwatch', size: 328, x: 80, y: 90, z: 30 },
      ],
    }),
  }));
  await page.route('**/api/graph/catalog/facets**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ revision_id: 'rev-42', spatial_catalog_id: 'catalog-8', exact_total: 2, types: [{ type: 'subreddit', count: 1 }, { type: 'post', count: 1 }], partial: false }),
  }));
  await page.route('**/api/graph/catalog/tree/**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ revision_id: 'rev-42', spatial_catalog_id: 'catalog-8', root: { id: 'post_sensitive', type: 'post', label: 'Sensitive field report' }, children: [], returned_count: 0, cross_linked_authors: ['user_reader'], partial: false, stale: false }),
  }));
  await page.route('**/api/graph/catalog?**', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ revision_id: 'rev-42', spatial_catalog_id: 'catalog-8', returned_count: 2, total_count: 2, applied_filters: {}, partial: false, stale: false, items: [
      { id: 'subreddit_alpha', type: 'subreddit', label: 'Alpha', description: 'A public community.', value: 100, sensitive: false, updated_at: '2026-08-15', metrics: {}, distinctiveness: .7, affinity_confidence: .8 },
      { id: 'post_sensitive', type: 'post', label: 'Sensitive field report', body: 'collapsed text', value: 50, sensitive: true, updated_at: '2026-08-15', metrics: {}, distinctiveness: .6, affinity_confidence: .7 },
    ] }),
  }));
  await page.route('**/api/nodes/**', route => {
    const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() || '');
    const node = world.nodes.find(candidate => candidate.id === id) || world.nodes[0];
    return route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ revision_id: 'rev-42', spatial_catalog_id: 'catalog-8', id: node.id, name: node.name, type: 'community', val: String(node.val), degree: 1, neighbors: [{ id: 'c:beta', name: 'Beta', val: '95', type: 'community', degree: 1 }] }),
    });
  });
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
  await expect(page.locator('[role="application"]')).toHaveAttribute('data-visible-label-count', '4');
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

test('uses one published world across Map, Data, Places, and keyboard inspection', async ({ page }) => {
  const legacyGraphRequests: string[] = [];
  page.on('request', request => {
    const url = new URL(request.url());
    if (url.pathname === '/api/graph') legacyGraphRequests.push(request.url());
  });
  await mockSpatialWorld(page);
  await page.goto('/');

  const universe = page.getByRole('application', { name: /interactive community universe/i });
  await expect(universe).toHaveAttribute('data-revision', 'rev-42');
  await universe.focus();
  await universe.press('Enter');
  await expect(page.getByRole('complementary', { name: 'Node Inspector Panel' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Alpha' })).toBeVisible();

  await page.getByRole('button', { name: 'Map', exact: true }).click();
  await expect(page.getByRole('application', { name: /published community map/i })).toBeVisible();
  await expect(page.locator('[data-revision="rev-42"]')).toBeVisible();

  await page.getByRole('button', { name: 'Data', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Universe telemetry' })).toBeVisible();
  await expect(page.getByText('7.1M')).toBeVisible();

  await page.getByRole('button', { name: 'Places', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Community landmarks' })).toBeVisible();
  await expect(page.getByRole('button', { name: /Alpha/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /recompute/i })).toHaveCount(0);

  expect(legacyGraphRequests).toEqual([]);
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

  await page.getByRole('button', { name: 'Places', exact: true }).click();
  const places = page.locator('.content-view');
  await expect(page.getByRole('heading', { name: 'Community landmarks' })).toBeVisible();
  expect(await places.evaluate(element => element.scrollWidth)).toBeLessThanOrEqual(
    await places.evaluate(element => element.clientWidth),
  );

  const artifactDirectory = process.env.CLUSTR_VISUAL_DIR;
  if (artifactDirectory) await page.screenshot({ path: `${artifactDirectory}/clustr-observatory-mobile.png`, fullPage: true });
});

test('keeps Catalog filters, results, detail, and sensitive disclosure coordinated on mobile', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockSpatialWorld(page);
  await page.goto('/?view=catalog');

  await expect(page.getByRole('heading', { name: 'Catalog' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Catalog results' })).toBeVisible();
  await expect(page.getByRole('complementary', { name: 'Catalog filters' })).toBeHidden();
  await page.getByRole('button', { name: /Sensitive field report/ }).click();
  await expect(page.getByRole('complementary', { name: 'Entity detail' })).toBeVisible();
  await expect(page.getByText('SENSITIVE CONTENT · COLLAPSED')).toBeVisible();
  await expect(page.getByText('collapsed text')).toBeHidden();
  await page.getByRole('button', { name: 'filters' }).click();
  await expect(page.getByRole('complementary', { name: 'Catalog filters' })).toBeVisible();
  await expect(page.getByRole('complementary', { name: 'Entity detail' })).toBeHidden();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

  await page.setViewportSize({ width: 844, height: 390 });
  await expect(page.getByRole('complementary', { name: 'Catalog filters' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Catalog results' })).toBeVisible();
  await expect(page.getByRole('complementary', { name: 'Entity detail' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(844);
});
