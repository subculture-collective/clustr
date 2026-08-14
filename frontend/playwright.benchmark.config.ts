import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for performance benchmarks
 * 
 * This config is optimized for consistent, reproducible performance measurements.
 */
export default defineConfig({
  testDir: './benchmarks',
  testMatch: '**/*.spec.ts',
  
  // Run benchmarks sequentially for consistent results
  fullyParallel: false,
  workers: 1,
  
  // Don't retry benchmarks - we want deterministic results
  retries: 0,
  
  // Longer timeout for large dataset tests
  timeout: 120 * 1000,
  
  // Only use JSON reporter for CI, console for local
  reporter: process.env.CI ? 'json' : 'list',
  
  use: {
    baseURL: 'http://localhost:5173',
    
    // Fixed viewport for consistent rendering measurements
    viewport: { width: 1280, height: 720 },
    
    // Disable video/screenshot to reduce overhead
    video: 'off',
    screenshot: 'off',
    
    // Disable tracing for benchmarks
    trace: 'off',
    
    // Enable performance metrics in Chrome
    launchOptions: {
      args: [
        '--enable-precise-memory-info', // Enable performance.memory API
        '--disable-gpu-vsync', // Disable VSync for more accurate FPS measurement
      ],
    },
  },

  projects: [
    {
      name: 'chromium',
      use: { 
        ...devices['Desktop Chrome'],
        // Additional Chrome flags for benchmarking
        launchOptions: {
          args: [
            '--enable-precise-memory-info',
            '--disable-gpu-vsync',
            '--js-flags=--expose-gc', // Allow garbage collection control
          ],
        },
      },
    },
  ],

  /* Run local dev server before starting benchmarks */
  webServer: {
    command: 'VITE_MAX_RENDER_NODES=100000 VITE_MAX_RENDER_LINKS=250000 npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 120 * 1000,
  },
});
