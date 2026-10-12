import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ShareButton from './ShareButton';

// Mock generateShareURL
vi.mock('../utils/urlState', () => ({
  generateShareURL: vi.fn(() => 'https://example.com/graph?view=3d'),
}));

describe('ShareButton', () => {
  const mockGetState = vi.fn(() => ({
    viewMode: '3d' as const,
    filters: { subreddit: true, user: true, post: false, comment: false },
  }));

  beforeEach(() => {
    // Mock clipboard API with spy
    const writeTextSpy = vi.fn(() => Promise.resolve());
    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: writeTextSpy,
      },
      writable: true,
      configurable: true,
    });
  });

  it('renders share button', () => {
    render(<ShareButton getState={mockGetState} />);
    expect(screen.getByText(/Share Link/)).toBeInTheDocument();
  });

  it('calls getState when sharing', async () => {
    const user = userEvent.setup();
    const getState = vi.fn(() => ({
      viewMode: '3d' as const,
    }));

    render(<ShareButton getState={getState} />);

    const button = screen.getByText(/Share Link/);
    await user.click(button);

    expect(getState).toHaveBeenCalled();
  });

  it('renders with appropriate styling', () => {
    render(<ShareButton getState={mockGetState} />);
    const button = screen.getByText(/Share Link/);
    expect(button).toHaveClass('px-3', 'py-2', 'rounded-sm');
  });

  it('has accessible title attribute', () => {
    render(<ShareButton getState={mockGetState} />);
    const button = screen.getByText(/Share Link/);
    expect(button).toHaveAttribute('title', 'Copy shareable link to clipboard');
  });
});

