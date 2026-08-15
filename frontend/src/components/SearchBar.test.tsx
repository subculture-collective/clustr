import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SearchBar from './SearchBar';

describe('SearchBar', () => {
  const mockOnSelectNode = vi.fn();
  let fetchMock: ReturnType<typeof vi.fn>;
  const setupUser = () => userEvent.setup({ delay: null });
  const waitForDebounce = () => new Promise((resolve) => setTimeout(resolve, 175));

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 4 }), { status: 200 }));
    global.fetch = fetchMock;
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders search input with placeholder', () => {
    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    expect(input).toBeInTheDocument();
  });

  it('shows clear button when query is entered', async () => {
    const user = setupUser();
    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'test');
    
    // Clear button should appear
    expect(screen.getByLabelText('Clear search')).toBeInTheDocument();
  });

  it('performs search after debounce delay', async () => {
    const user = setupUser();
    
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        revision_id: 9,
        results: [
          {
            ID: 'subreddit_1',
            Name: 'askreddit',
            Val: '100',
            Type: { String: 'subreddit', Valid: true },
          },
        ],
      }),
    } as Response);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'ask');

    // Fast-forward past debounce delay
    await waitForDebounce();

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/search?node=ask'),
        expect.any(Object)
      );
    });
  });

  it('displays search results in dropdown', async () => {
    const user = setupUser();
    
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        revision_id: 9,
        results: [
          {
            ID: 'subreddit_1',
            Name: 'askreddit',
            Val: '100',
            Type: { String: 'subreddit', Valid: true },
          },
          {
            ID: 'user_1',
            Name: 'testuser',
            Val: '50',
            Type: { String: 'user', Valid: true },
          },
        ],
      }),
    } as Response);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'test');

    // Fast-forward past debounce
    await waitForDebounce();

    // Wait for results
    await waitFor(() => {
      expect(screen.getByText('askreddit')).toBeInTheDocument();
      expect(screen.getByText('testuser')).toBeInTheDocument();
    });
  });

  it('calls onSelectNode when result is clicked', async () => {
    const user = setupUser();
    
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        revision_id: 9,
        results: [
          {
            ID: 'subreddit_1',
            Name: 'askreddit',
            Val: '100',
            Type: { String: 'subreddit', Valid: true },
          },
        ],
      }),
    } as Response);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'ask');

    // Fast-forward past debounce
    await waitForDebounce();

    // Wait for result
    await waitFor(() => {
      expect(screen.getByText('askreddit')).toBeInTheDocument();
    });

    // Click the result
    const result = screen.getByText('askreddit');
    await user.click(result);

    expect(mockOnSelectNode).toHaveBeenCalledWith('subreddit_1');
  });

  it('allows keyboard navigation with arrow keys', async () => {
    const user = setupUser();
    
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        revision_id: 9,
        results: [
          {
            ID: 'subreddit_1',
            Name: 'askreddit',
            Val: '100',
            Type: { String: 'subreddit', Valid: true },
          },
          {
            ID: 'user_1',
            Name: 'testuser',
            Val: '50',
            Type: { String: 'user', Valid: true },
          },
        ],
      }),
    } as Response);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'test');

    // Fast-forward past debounce
    await waitForDebounce();

    // Wait for results
    await waitFor(() => {
      expect(screen.getByText('askreddit')).toBeInTheDocument();
    });

    // Press arrow down
    await user.keyboard('{ArrowDown}');
    
    // Press Enter to select
    await user.keyboard('{Enter}');

    expect(mockOnSelectNode).toHaveBeenCalledWith('user_1');
  });

  it('closes dropdown on Escape key', async () => {
    const user = setupUser();
    
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        revision_id: 9,
        results: [
          {
            ID: 'subreddit_1',
            Name: 'askreddit',
            Val: '100',
            Type: { String: 'subreddit', Valid: true },
          },
        ],
      }),
    } as Response);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'ask');

    // Fast-forward past debounce
    await waitForDebounce();

    // Wait for results
    await waitFor(() => {
      expect(screen.getByText('askreddit')).toBeInTheDocument();
    });

    // Press Escape
    await user.keyboard('{Escape}');

    // Dropdown should be closed
    await waitFor(() => {
      expect(screen.queryByText('askreddit')).not.toBeInTheDocument();
    });
  });

  it('shows "no results" message when search returns empty', async () => {
    const user = setupUser();
    
    fetchMock.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        revision_id: 9,
        results: [],
      }),
    } as Response);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'nonexistent');

    // Fast-forward past debounce
    await waitForDebounce();

    // Wait for no results message
    await waitFor(() => {
      expect(screen.getByText(/No results found/i)).toBeInTheDocument();
    });
  });

  it('shows loading indicator while searching', async () => {
    const user = setupUser();
    
    // Create a promise that we can control
    let resolveSearch: ((value: Response) => void) | undefined;
    const searchPromise = new Promise<Response>((resolve) => {
      resolveSearch = resolve;
    });

    fetchMock.mockReturnValueOnce(searchPromise as Promise<Response>);

    render(<SearchBar onSelectNode={mockOnSelectNode} />);
    
    const input = screen.getByPlaceholderText(/Search nodes/i);
    await user.type(input, 'test');

    // Fast-forward past debounce
    await waitForDebounce();

    // Wait for loading state
    await waitFor(() => {
      const spinner = input.parentElement?.querySelector('.animate-spin');
      expect(spinner).toBeInTheDocument();
    });

    // Resolve the search
    resolveSearch!({
      ok: true,
      json: async () => ({ revision_id: 9, results: [] }),
    } as Response);
  });
});
