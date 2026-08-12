import { defineConfig, devices } from '@playwright/test';

/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['html'], ['list']] : 'html',
  
  // Screenshot comparison settings for visual regression tests
  expect: {
    toHaveScreenshot: {
      // 1% pixel difference tolerance to handle minor rendering variations
      maxDiffPixelRatio: 0.01,
      // Avoid flakiness from animations
      animations: 'disabled',
      // Consistent anti-aliasing across environments
      threshold: 0.2,
    },
  },
  
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
    // Fixed viewport for consistent screenshots
    viewport: { width: 1280, height: 720 },
    // Disable video for faster tests, screenshots are captured by test
    video: 'off',
  },

  projects: [
    {
      name: 'chromium',
      use: { 
        ...devices['Desktop Chrome'],
        // Ensure consistent rendering for visual tests
        launchOptions: {
          executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
          args: [
            '--disable-web-security',
            '--disable-gpu-vsync',
            '--force-color-profile=srgb',
          ],
        },
      },
    },
  ],

  /* Run your local dev server before starting the tests */
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 120 * 1000,
  },
});
