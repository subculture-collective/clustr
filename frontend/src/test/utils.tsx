import { type ReactElement } from 'react';
import { render, type RenderOptions } from '@testing-library/react';
import { ThemeProvider } from '../contexts/ThemeContext';

/**
 * Custom render function that wraps components with ThemeProvider
 */
export function renderWithTheme(
  ui: ReactElement,
  options?: Omit<RenderOptions, 'wrapper'>
) {
  return render(ui, {
    wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    ...options,
  });
}

// This is a test-only module; re-exporting RTL is intentional and does not
// participate in Fast Refresh boundaries.
// eslint-disable-next-line react-refresh/only-export-components
export * from '@testing-library/react';
