import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Inspector from './Inspector';

// Mock fetch globally
global.fetch = vi.fn();

describe('Inspector', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(global.fetch).mockRejectedValue(new Error('API call failed'));
  });

  it('renders nothing when no selection', () => {
    const { container } = render(
      <Inspector selected={undefined} onClear={vi.fn()} onFocus={vi.fn()} />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders nothing when selection has no connections and loading completes', async () => {
    const { container } = render(
      <Inspector
        selected={{ id: 'test-1', degree: 0 }}
        onClear={vi.fn()}
        onFocus={vi.fn()}
      />
    );
    
    // After fetch fails with no connections, should hide
    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it('renders selected node with degree', async () => {
    render(
      <Inspector
        selected={{ id: 'test-1', name: 'Test Node', type: 'subreddit', degree: 5 }}
        onClear={vi.fn()}
        onFocus={vi.fn()}
      />
    );

    expect(screen.getByText('Node Inspector')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getAllByText('test-1')[0]).toBeInTheDocument();
    });
    expect(screen.getByText('Test Node')).toBeInTheDocument();
    expect(screen.getByText('subreddit')).toBeInTheDocument();
  });

  it('renders neighbors list', async () => {
    const neighbors = [
      { id: 'neighbor-1', name: 'Neighbor 1', type: 'user' },
      { id: 'neighbor-2', name: 'Neighbor 2', type: 'post' },
    ];

    render(
      <Inspector
        selected={{ id: 'test-1', degree: 2, neighbors }}
        onClear={vi.fn()}
        onFocus={vi.fn()}
      />
    );

    // Wait for loading to complete and check neighbors tab
    await waitFor(() => {
      expect(screen.getByText(/Connections \(2\)/)).toBeInTheDocument();
    });
  });

  it('calls onClear when close button clicked', async () => {
    const user = userEvent.setup();
    const onClear = vi.fn();

    render(
      <Inspector
        selected={{ id: 'test-1', degree: 1 }}
        onClear={onClear}
        onFocus={vi.fn()}
      />
    );

    const closeButton = screen.getByLabelText('Close inspector');
    await user.click(closeButton);

    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it('calls onFocus when neighbor clicked', async () => {
    const user = userEvent.setup();
    const onFocus = vi.fn();
    const neighbors = [{ id: 'neighbor-1', name: 'Neighbor 1', type: 'user' }];

    render(
      <Inspector
        selected={{ id: 'test-1', degree: 1, neighbors }}
        onClear={vi.fn()}
        onFocus={onFocus}
      />
    );

    // Click on Connections tab first
    await waitFor(() => {
      const connectionsTab = screen.getByText(/Connections \(1\)/);
      expect(connectionsTab).toBeInTheDocument();
    });
    
    const connectionsTab = screen.getByText(/Connections \(1\)/);
    await user.click(connectionsTab);

    // Now click the neighbor button
    const neighborButton = screen.getByText(/Neighbor 1/);
    await user.click(neighborButton);

    expect(onFocus).toHaveBeenCalledWith('neighbor-1');
  });

  it('renders without name or type', async () => {
    render(
      <Inspector
        selected={{ id: 'test-1', degree: 3 }}
        onClear={vi.fn()}
        onFocus={vi.fn()}
      />
    );

    await waitFor(() => {
      // Get all elements with test-1, should find it in both name and ID sections
      const elements = screen.getAllByText('test-1');
      expect(elements.length).toBeGreaterThan(0);
    });
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('switches between tabs', async () => {
    const user = userEvent.setup();
    
    render(
      <Inspector
        selected={{ id: 'test-1', name: 'Test Node', degree: 5 }}
        onClear={vi.fn()}
        onFocus={vi.fn()}
      />
    );

    // Check Overview tab is active by default
    expect(screen.getByText('Overview')).toBeInTheDocument();

    // Click Statistics tab
    const statsTab = screen.getByText('Statistics');
    await user.click(statsTab);

    // Should show statistics content
    await waitFor(() => {
      expect(screen.getByText('Connection Statistics')).toBeInTheDocument();
    });
  });

  it('fetches and displays node details', async () => {
    const mockNodeDetails = {
      id: 'subreddit_123',
      name: 'AskReddit',
      val: '1000',
      type: 'subreddit',
      degree: 10,
      neighbors: [
        { id: 'user_1', name: 'User1', val: '50', type: 'user', degree: 5 }
      ],
      stats: {
        subscribers: 50000000,
        title: 'Ask Reddit...',
        description: 'r/AskReddit is the place...'
      }
    };

    vi.mocked(global.fetch)
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 4 }), { status: 200 }))
      .mockResolvedValueOnce({
      ok: true,
      json: async () => mockNodeDetails
    } as Response);

    render(
      <Inspector
        selected={{ id: 'subreddit_123', degree: 1 }}
        onClear={vi.fn()}
        onFocus={vi.fn()}
      />
    );

    // Should show loading state initially
    expect(screen.getByRole('status')).toBeInTheDocument();

    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByText('AskReddit')).toBeInTheDocument();
    });

    expect(screen.getByText('Subreddit Info')).toBeInTheDocument();
    expect(screen.getByText('50,000,000')).toBeInTheDocument();
  });

  it('does not refetch when the same selection is passed as a new object', async () => {
    vi.mocked(global.fetch)
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 4 }), { status: 200 }))
      .mockResolvedValue({
        ok: true,
        json: async () => ({ revision_id: 9, spatial_catalog_id: 4, id: 'community_1', name: 'Place One', type: 'community', degree: 1, neighbors: [] }),
      } as Response);
    const props = { onClear: vi.fn(), onFocus: vi.fn() };
    const { rerender } = render(<Inspector {...props} selected={{ id: 'community_1', degree: 1 }} />);
    await waitFor(() => expect(screen.getByText('Place One')).toBeInTheDocument());

    rerender(<Inspector {...props} selected={{ id: 'community_1', degree: 1 }} />);

    expect(vi.mocked(global.fetch).mock.calls.filter(call => String(call[0]).includes('/nodes/'))).toHaveLength(1);
  });
});
