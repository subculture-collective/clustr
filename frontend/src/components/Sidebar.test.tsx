import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Sidebar from './Sidebar';

describe('Sidebar', () => {
  const mockFilters = {
    subreddit: true,
    user: true,
    post: false,
    comment: false,
  };

  const mockPhysics = {
    chargeStrength: -220,
    linkDistance: 120,
    velocityDecay: 0.88,
    cooldownTicks: 80,
    collisionRadius: 3,
  };

  const defaultProps = {
    filters: mockFilters,
    onFiltersChange: vi.fn(),
    linkOpacity: 0.35,
    onLinkOpacityChange: vi.fn(),
    nodeRelSize: 5,
    onNodeRelSizeChange: vi.fn(),
    physics: mockPhysics,
    onPhysicsChange: vi.fn(),
    subredditSize: 'subscribers' as const,
    onSubredditSizeChange: vi.fn(),
    onFocusNode: vi.fn(),
  };

  beforeEach(() => {
    // Mock fetch for admin services
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ crawler_enabled: true, precalc_enabled: true }),
      } as Response)
    );
    // Clear localStorage
    localStorage.clear();
  });

  it('renders sidebar in expanded state by default', () => {
    render(<Sidebar {...defaultProps} />);
    
    expect(screen.getByRole('heading', { name: 'World controls' })).toBeInTheDocument();
  });

  it('renders view mode buttons', () => {
    render(<Sidebar {...defaultProps} graphMode="3d" onGraphModeChange={vi.fn()} />);
    
    expect(screen.getByText('3D')).toBeInTheDocument();
    expect(screen.getByText('2D')).toBeInTheDocument();
  });

  it('renders dashboard button', () => {
    render(<Sidebar {...defaultProps} onShowDashboard={vi.fn()} />);
    
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
  });

  it('renders communities button', () => {
    render(<Sidebar {...defaultProps} onShowCommunities={vi.fn()} />);
    
    expect(screen.getByText('Communities')).toBeInTheDocument();
  });

  it('renders admin button', () => {
    render(<Sidebar {...defaultProps} onShowAdmin={vi.fn()} />);
    
    expect(screen.getByRole('button', { name: 'Admin' })).toBeInTheDocument();
  });

  it('calls onGraphModeChange when 3D button clicked', async () => {
    const user = userEvent.setup();
    const onGraphModeChange = vi.fn();
    
    render(<Sidebar {...defaultProps} graphMode="2d" onGraphModeChange={onGraphModeChange} />);
    
    const button3D = screen.getByText('3D');
    await user.click(button3D);
    
    expect(onGraphModeChange).toHaveBeenCalledWith('3d');
  });

  it('calls onGraphModeChange when 2D button clicked', async () => {
    const user = userEvent.setup();
    const onGraphModeChange = vi.fn();
    
    render(<Sidebar {...defaultProps} graphMode="3d" onGraphModeChange={onGraphModeChange} />);
    
    const button2D = screen.getByText('2D');
    await user.click(button2D);
    
    expect(onGraphModeChange).toHaveBeenCalledWith('2d');
  });

  it('calls onShowDashboard when Dashboard button clicked', async () => {
    const user = userEvent.setup();
    const onShowDashboard = vi.fn();
    
    render(<Sidebar {...defaultProps} onShowDashboard={onShowDashboard} />);
    
    const button = screen.getByText('Dashboard');
    await user.click(button);
    
    expect(onShowDashboard).toHaveBeenCalled();
  });

  it('calls onShowCommunities when Communities button clicked', async () => {
    const user = userEvent.setup();
    const onShowCommunities = vi.fn();
    
    render(<Sidebar {...defaultProps} onShowCommunities={onShowCommunities} />);
    
    const button = screen.getByText('Communities');
    await user.click(button);
    
    expect(onShowCommunities).toHaveBeenCalled();
  });

  it('calls onShowAdmin when Admin button clicked', async () => {
    const user = userEvent.setup();
    const onShowAdmin = vi.fn();
    
    render(<Sidebar {...defaultProps} onShowAdmin={onShowAdmin} />);
    
    const button = screen.getByRole('button', { name: 'Admin' });
    await user.click(button);
    
    expect(onShowAdmin).toHaveBeenCalled();
  });

  it('highlights active view mode', () => {
    render(<Sidebar {...defaultProps} graphMode="3d" onGraphModeChange={vi.fn()} />);
    
    const button3D = screen.getByText('3D');
    expect(button3D).toHaveClass('bg-blue-600');
  });

  it('shows community colors toggle when callback provided', () => {
    render(
      <Sidebar
        {...defaultProps}
        useCommunityColors={false}
        onToggleCommunityColors={vi.fn()}
      />
    );
    
    expect(screen.getByText('Use community colors')).toBeInTheDocument();
  });

  it('shows precomputed layout toggle when callback provided', () => {
    render(
      <Sidebar
        {...defaultProps}
        usePrecomputedLayout={true}
        onTogglePrecomputedLayout={vi.fn()}
      />
    );
    
    expect(screen.getByText('Use precomputed layout')).toBeInTheDocument();
  });

  it('calls onToggleCommunityColors when checkbox changed', async () => {
    const user = userEvent.setup();
    const onToggleCommunityColors = vi.fn();
    
    render(
      <Sidebar
        {...defaultProps}
        useCommunityColors={false}
        onToggleCommunityColors={onToggleCommunityColors}
      />
    );
    
    const checkbox = screen.getByRole('checkbox', { name: /community colors/i });
    await user.click(checkbox);
    
    expect(onToggleCommunityColors).toHaveBeenCalledWith(true);
  });

  it('calls onTogglePrecomputedLayout when checkbox changed', async () => {
    const user = userEvent.setup();
    const onTogglePrecomputedLayout = vi.fn();
    
    render(
      <Sidebar
        {...defaultProps}
        usePrecomputedLayout={true}
        onTogglePrecomputedLayout={onTogglePrecomputedLayout}
      />
    );
    
    const checkbox = screen.getByRole('checkbox', { name: /precomputed layout/i });
    await user.click(checkbox);
    
    expect(onTogglePrecomputedLayout).toHaveBeenCalledWith(false);
  });

  it('collapses when collapse button clicked', async () => {
    const user = userEvent.setup();
    
    render(<Sidebar {...defaultProps} />);
    
    const collapseButton = screen.getByRole('button', { name: /collapse sidebar/i });
    await user.click(collapseButton);
    
    await waitFor(() => {
      expect(collapseButton).toHaveAttribute('aria-expanded', 'false');
      expect(screen.getByRole('button', { name: /expand sidebar/i })).toBeInTheDocument();
    });
  });

  it('expands when expand button clicked', async () => {
    const user = userEvent.setup();
    
    // Set localStorage to collapsed state
    localStorage.setItem('sidebar-collapsed', 'true');
    
    render(<Sidebar {...defaultProps} />);
    
    const expandButton = screen.getByRole('button', { name: /expand sidebar/i });
    await user.click(expandButton);
    
    await waitFor(() => {
      expect(expandButton).toHaveAttribute('aria-expanded', 'true');
      expect(screen.getByRole('heading', { name: 'World controls' })).toBeInTheDocument();
    });
  });

  it('persists collapsed state in localStorage', async () => {
    const user = userEvent.setup();
    
    render(<Sidebar {...defaultProps} />);
    
    const collapseButton = screen.getByRole('button', { name: /collapse sidebar/i });
    await user.click(collapseButton);
    
    await waitFor(() => {
      expect(localStorage.getItem('sidebar-collapsed')).toBe('true');
    });
  });

  it('renders without optional props', () => {
    const { container } = render(<Sidebar {...defaultProps} />);
    expect(container).toBeTruthy();
  });

  it('renders organized sections', () => {
    render(<Sidebar {...defaultProps} />);
    
    expect(screen.getByText('View')).toBeInTheDocument();
    expect(screen.getByText('Filters')).toBeInTheDocument();
    expect(screen.getByText('Physics')).toBeInTheDocument();
    expect(screen.getByText('Display')).toBeInTheDocument();
    expect(screen.getByText('Data')).toBeInTheDocument();
  });

  it('toggles sidebar when Ctrl+B is pressed', async () => {
    const user = userEvent.setup();
    
    render(<Sidebar {...defaultProps} />);
    
    // Sidebar should be expanded initially
    const sidebarToggle = screen.getByRole('button', { name: /collapse sidebar/i });
    expect(sidebarToggle).toHaveAttribute('aria-expanded', 'true');
    
    // Press Ctrl+B to collapse
    await user.keyboard('{Control>}b{/Control}');
    
    await waitFor(() => {
      expect(sidebarToggle).toHaveAttribute('aria-expanded', 'false');
    });
    
    // Press Ctrl+B again to expand
    await user.keyboard('{Control>}b{/Control}');
    
    await waitFor(() => {
      expect(sidebarToggle).toHaveAttribute('aria-expanded', 'true');
    });
  });

  it('ignores Ctrl+B when focus is in an input field', async () => {
    const user = userEvent.setup();
    
    render(<Sidebar {...defaultProps} />);
    
    // Focus the search input
    const searchInput = screen.getByPlaceholderText('Focus node by id/name');
    await user.click(searchInput);
    
    // Press Ctrl+B while focused in input
    await user.keyboard('{Control>}b{/Control}');
    
    // Sidebar should still be expanded (Ctrl+B was ignored)
    expect(screen.getByRole('button', { name: /collapse sidebar/i })).toHaveAttribute('aria-expanded', 'true');
  });
});
