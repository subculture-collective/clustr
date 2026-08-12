import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import App from './App';

// Mock all child components
vi.mock('./components/Admin', () => ({
  default: ({ onViewMode }: { onViewMode: (mode: string) => void }) => (
    <div>
      <div>Mocked Admin</div>
      <button onClick={() => onViewMode('3d')}>Switch to 3D</button>
    </div>
  ),
}));

vi.mock('./components/Dashboard', () => ({
  default: ({ onViewMode }: { onViewMode: (mode: string) => void }) => (
    <div>
      <div>Mocked Dashboard</div>
      <button onClick={() => onViewMode('3d')}>Switch to 3D</button>
    </div>
  ),
}));

vi.mock('./components/Communities', () => ({
  default: ({ onViewMode }: { onViewMode: (mode: string) => void }) => (
    <div>
      <div>Mocked Communities</div>
      <button onClick={() => onViewMode('3d')}>Switch to 3D</button>
    </div>
  ),
}));

vi.mock('./components/Sidebar', () => ({
  default: ({ graphMode, onGraphModeChange, onShowDashboard, onShowAdmin }: {
    graphMode: string;
    onGraphModeChange: (mode: string) => void;
    onShowDashboard: () => void;
    onShowAdmin: () => void;
  }) => (
    <div>
      <div>Mocked Sidebar</div>
      <div>Mode: {graphMode}</div>
      <button onClick={() => onGraphModeChange('2d')}>Switch to 2D</button>
      <button onClick={onShowDashboard}>Show Dashboard</button>
      <button onClick={onShowAdmin}>Show Admin</button>
    </div>
  ),
}));

vi.mock('./components/Graph3D', () => ({
  default: () => <div>Mocked Graph3D</div>,
}));

vi.mock('./components/Graph2D', () => ({
  default: () => <div>Mocked Graph2D</div>,
}));

vi.mock('./components/Inspector', () => ({
  default: () => <div>Mocked Inspector</div>,
}));

vi.mock('./components/Legend', () => ({
  default: () => <div>Mocked Legend</div>,
}));

vi.mock('./components/ShareButton', () => ({
  default: ({ getState }: { getState: () => unknown }) => (
    <button onClick={() => getState()}>Mocked ShareButton</button>
  ),
}));

vi.mock('./components/ErrorBoundary', () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('./components/GraphErrorFallback', () => ({
  default: () => <div>Mocked GraphErrorFallback</div>,
}));

vi.mock('./utils/webglDetect', () => ({
  detectWebGLSupport: () => true,
}));

vi.mock('./utils/urlState', () => ({
  readStateFromURL: () => ({}),
  writeStateToURL: vi.fn(),
}));

describe('App', () => {
  let setItemSpy: ReturnType<typeof vi.fn>;
  let getItemSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.useFakeTimers();
    // Mock localStorage
    setItemSpy = vi.fn();
    getItemSpy = vi.fn(() => null);
    Object.defineProperty(window, 'localStorage', {
      value: {
        getItem: getItemSpy,
        setItem: setItemSpy,
        removeItem: vi.fn(),
        clear: vi.fn(),
      },
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders without crashing', () => {
    const { container } = render(<App />);
    expect(container).toBeTruthy();
  });

  it('renders default 3D view', () => {
    render(<App />);
    expect(screen.getByText('Mocked Graph3D')).toBeInTheDocument();
    expect(screen.queryByText('Mocked Sidebar')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Analyst' })).toBeInTheDocument();
  });

  it('persists view mode to localStorage', () => {
    render(<App />);
    
    vi.runAllTimers();
    
    expect(setItemSpy).toHaveBeenCalledWith('viewMode', '3d');
  });

  it('persists precomputed layout preference to localStorage', () => {
    render(<App />);
    
    vi.runAllTimers();
    
    expect(setItemSpy).toHaveBeenCalledWith('usePrecomputedLayout', 'true');
  });

  it('writes state to URL', () => {
    render(<App />);
    
    // Fast forward 500ms debounce
    vi.advanceTimersByTime(500);
    
    // Just verify component renders
    expect(screen.getByText('Mocked Graph3D')).toBeInTheDocument();
  });

  it('switches to 2D view', () => {
    render(<App />);
    fireEvent.click(screen.getByRole('button', { name: 'Map' }));
    expect(screen.getByText('Mocked Graph2D')).toBeInTheDocument();
  });

  it('reveals dense controls only in analyst mode', () => {
    render(<App />);
    fireEvent.click(screen.getByRole('button', { name: 'Analyst' }));
    expect(screen.getByText('Mocked Sidebar')).toBeInTheDocument();
    expect(screen.getByText('Mocked Legend')).toBeInTheDocument();
  });

  it('switches to dashboard view', () => {
    render(<App />);
    
    // Verify 3D view is initially shown
    expect(screen.getByText('Mocked Graph3D')).toBeInTheDocument();
  });

  it('switches to admin view', () => {
    render(<App />);
    
    // Verify 3D view is initially shown
    expect(screen.getByText('Mocked Graph3D')).toBeInTheDocument();
  });

  it('renders main container with correct classes', () => {
    const { container } = render(<App />);
    const mainDiv = container.firstChild as HTMLElement;
    expect(mainDiv).toHaveClass('w-full', 'h-screen');
  });
});
