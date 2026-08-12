import { expect, afterEach, beforeAll } from 'vitest';
import { cleanup } from '@testing-library/react';
import * as matchers from '@testing-library/jest-dom/matchers';
import { toHaveNoViolations } from 'jest-axe';

// Node 26 exposes an incomplete experimental localStorage global which can
// shadow jsdom's implementation. Install a deterministic browser-compatible
// storage before test hooks and component modules execute.
const storageValues = new Map<string, string>();
const testStorage: Storage = {
  get length() { return storageValues.size; },
  clear: () => storageValues.clear(),
  getItem: key => storageValues.get(String(key)) ?? null,
  key: index => Array.from(storageValues.keys())[index] ?? null,
  removeItem: key => { storageValues.delete(String(key)); },
  setItem: (key, value) => { storageValues.set(String(key), String(value)); },
};
Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: testStorage });

// Extend Vitest's expect with jest-dom matchers
expect.extend(matchers);

// Extend Vitest's expect with jest-axe matchers
expect.extend(toHaveNoViolations);

// Mock window.matchMedia for theme detection
beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {}, // deprecated
      removeListener: () => {}, // deprecated
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => true,
    }),
  });
});

// Cleanup after each test case (e.g. clearing jsdom)
afterEach(() => {
  cleanup();
});
