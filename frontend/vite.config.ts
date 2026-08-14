import { defineConfig } from "vitest/config";
import { loadEnv } from "vite";
import react from "@vitejs/plugin-react-swc";
import { visualizer } from "rollup-plugin-visualizer";

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_PROXY_TARGET || "http://localhost:8000";
  return ({
  plugins: [
    react(),
    visualizer({
      filename: "./dist/stats.html",
      open: false,
      gzipSize: true,
      brotliSize: true,
    }),
  ],
  server: {
    proxy: {
      "/api": proxyTarget,
      "/subreddits": proxyTarget,
      "/users": proxyTarget,
      "/posts": proxyTarget,
      "/comments": proxyTarget,
      "/jobs": proxyTarget,
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    css: true,
    // Browser benchmarks are Playwright suites, not unit tests. Keeping them
    // out of Vitest prevents a full unit run from executing performance work.
    exclude: ['**/node_modules/**', '**/dist/**', '**/e2e/**', '**/benchmarks/**', '**/.{idea,git,cache,output,temp}/**'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/**/*.spec.{ts,tsx}',
        'src/test/**',
        'src/vite-env.d.ts',
        'src/main.tsx',
        'src/**/__typechecks__/**',
        // Type definition files
        'src/types/**/*.ts',
        // Exclude admin/dashboard components from coverage requirements
        'src/components/Admin.tsx',
        'src/components/Dashboard.tsx',
        'src/components/Communities.tsx',
        // Exclude complex utility that should be tested separately
        'src/utils/communityDetection.ts',
        'src/utils/apiErrors.ts',
        // Exclude Graph components that require extensive mocking of WebGL/Three.js
        'src/components/Graph3D.tsx',
        'src/components/Graph3DInstanced.tsx',
        'src/components/Graph2D.tsx',
        'src/components/CommunityMap.tsx',
        'src/App.tsx',
        // Browser workers are exercised by the Playwright performance suite.
        'src/workers/**',
        // Mock data files
        'src/__mocks__/**/*.ts',
      ],
      thresholds: {
        lines: 75,
        functions: 60,
        branches: 70,
        statements: 75,
      },
    },
  },
  });
});
