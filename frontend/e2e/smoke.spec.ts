import { test, expect } from '@playwright/test';

test.describe('Smoke Tests', () => {
  test('homepage loads successfully', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // Keep the delivery smoke aligned with the public product title rather
    // than an implementation-era Reddit name.
    await expect(page).toHaveTitle(/clustr/i);
    // A continuously interactive graph can keep requests in flight, so
    // network-idle is not a valid readiness signal. Assert the user-facing
    // scene contract instead.
    await expect(page.getByRole('application', { name: /interactive community universe/i })).toBeVisible();
  });

  test('app renders main UI elements', async ({ page }) => {
    await page.goto('/');
    
    // Wait for React to render
    await page.waitForSelector('body');
    
    // Basic smoke test - just verify the page loaded without crashing
    const body = await page.locator('body');
    await expect(body).toBeVisible();
  });

  test('handles navigation', async ({ page }) => {
    await page.goto('/');
    
    // Wait for page to be ready
    await page.waitForLoadState('domcontentloaded');
    
    // Verify we can interact with the page
    const body = page.locator('body');
    await expect(body).toBeVisible();
  });
});
