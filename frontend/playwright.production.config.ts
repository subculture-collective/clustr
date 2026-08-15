import { defineConfig, devices } from '@playwright/test';

const productionPort = process.env.PLAYWRIGHT_PORT || '4173';
const productionURL = `http://127.0.0.1:${productionPort}`;

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['html'], ['list']] : 'list',
  use: {
    baseURL: productionURL,
    trace: 'on-first-retry',
    viewport: { width: 1280, height: 720 },
  },
  projects: [{
    name: 'chromium-production',
    use: {
      ...devices['Desktop Chrome'],
      launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH },
    },
  }],
  webServer: {
    command: `npm run build && npm run preview -- --host 127.0.0.1 --port ${productionPort}`,
    url: productionURL,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
