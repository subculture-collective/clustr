import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  readStateFromURL,
  writeStateToURL,
  generateShareURL,
  type AppState,
} from './urlState';

describe('urlState', () => {
  let originalLocation: Location;
  const mutableWindow = window as unknown as {
    location?: Location;
    history?: History;
  };

  beforeEach(() => {
    // Save original location
    originalLocation = window.location;
    // Use delete to allow reassignment
    delete mutableWindow.location;
  });

  afterEach(() => {
    // Restore original location
    window.location = originalLocation;
  });

  describe('readStateFromURL', () => {
    it('returns empty object when no params', () => {
      // Set window.location
      window.location = { search: '' } as Location;
      
      const state = readStateFromURL();
      expect(state).toEqual({});
    });

    it('reads view mode from URL', () => {
      window.location = { search: '?view=2d' } as Location;
      
      const state = readStateFromURL();
      expect(state.viewMode).toBe('2d');
    });

    it('reads filters from URL', () => {
      window.location = { search: '?f_subreddit=1&f_user=0&f_post=1&f_comment=0' } as Location;
      
      const state = readStateFromURL();
      expect(state.filters).toEqual({
        subreddit: true,
        user: false,
        post: true,
        comment: false,
      });
    });

    it('reads degree thresholds from URL', () => {
      window.location = { search: '?minDegree=5&maxDegree=100' } as Location;
      
      const state = readStateFromURL();
      expect(state.minDegree).toBe(5);
      expect(state.maxDegree).toBe(100);
    });

    it('reads 3D camera position from URL', () => {
      window.location = { search: '?cam3d_x=100.5&cam3d_y=200.3&cam3d_z=300.7' } as Location;
      
      const state = readStateFromURL();
      expect(state.camera3d).toEqual({ x: 100.5, y: 200.3, z: 300.7 });
    });

    it('reads 2D camera position from URL', () => {
      window.location = { search: '?cam2d_x=50.2&cam2d_y=75.8&cam2d_zoom=1.5' } as Location;
      
      const state = readStateFromURL();
      expect(state.camera2d).toEqual({ x: 50.2, y: 75.8, zoom: 1.5 });
    });

    it('reads community colors setting from URL', () => {
      window.location = { search: '?communityColors=1' } as Location;
      
      const state = readStateFromURL();
      expect(state.useCommunityColors).toBe(true);
    });

    it('reads precomputed layout setting from URL', () => {
      window.location = { search: '?precomputedLayout=0' } as Location;
      
      const state = readStateFromURL();
      expect(state.usePrecomputedLayout).toBe(false);
    });

    it('reads size attenuation setting from URL', () => {
      window.location = { search: '?sizeAttenuation=1' } as Location;
      
      const state = readStateFromURL();
      expect(state.sizeAttenuation).toBe(true);
    });

    it('reads size attenuation disabled from URL', () => {
      window.location = { search: '?sizeAttenuation=0' } as Location;
      
      const state = readStateFromURL();
      expect(state.sizeAttenuation).toBe(false);
    });
  });

  describe('writeStateToURL', () => {
    let originalHistory: History;

    beforeEach(() => {
      originalHistory = window.history;
    });

    afterEach(() => {
      window.history = originalHistory;
    });

    it('writes view mode to URL', () => {
      const mockReplaceState = vi.fn();
      delete mutableWindow.history;
      mutableWindow.history = { replaceState: mockReplaceState } as unknown as History;
      window.location = { search: '', pathname: '/test' } as Location;

      const state: AppState = { viewMode: '3d' };
      writeStateToURL(state);

      expect(mockReplaceState).toHaveBeenCalled();
      const url = mockReplaceState.mock.calls[0][2];
      expect(url).toContain('view=3d');
    });

    it('writes filters to URL', () => {
      const mockReplaceState = vi.fn();
      delete mutableWindow.history;
      mutableWindow.history = { replaceState: mockReplaceState } as unknown as History;
      window.location = { search: '', pathname: '/test' } as Location;

      const state: AppState = {
        filters: {
          subreddit: true,
          user: false,
          post: true,
          comment: false,
        },
      };
      writeStateToURL(state);

      const url = mockReplaceState.mock.calls[0][2];
      expect(url).toContain('f_subreddit=1');
      expect(url).toContain('f_user=0');
      expect(url).toContain('f_post=1');
      expect(url).toContain('f_comment=0');
    });

    it('writes camera positions to URL', () => {
      const mockReplaceState = vi.fn();
      delete mutableWindow.history;
      mutableWindow.history = { replaceState: mockReplaceState } as unknown as History;
      window.location = { search: '', pathname: '/test' } as Location;

      const state: AppState = {
        camera3d: { x: 100, y: 200, z: 300 },
        camera2d: { x: 50, y: 75, zoom: 1.5 },
      };
      writeStateToURL(state);

      const url = mockReplaceState.mock.calls[0][2];
      expect(url).toContain('cam3d_x=100.00');
      expect(url).toContain('cam2d_zoom=1.50');
    });

    it('writes size attenuation to URL', () => {
      const mockReplaceState = vi.fn();
      delete mutableWindow.history;
      mutableWindow.history = { replaceState: mockReplaceState } as unknown as History;
      window.location = { search: '', pathname: '/test' } as Location;

      const state: AppState = { sizeAttenuation: true };
      writeStateToURL(state);

      const url = mockReplaceState.mock.calls[0][2];
      expect(url).toContain('sizeAttenuation=1');
    });

    it('writes size attenuation disabled to URL', () => {
      const mockReplaceState = vi.fn();
      delete mutableWindow.history;
      mutableWindow.history = { replaceState: mockReplaceState } as unknown as History;
      window.location = { search: '', pathname: '/test' } as Location;

      const state: AppState = { sizeAttenuation: false };
      writeStateToURL(state);

      const url = mockReplaceState.mock.calls[0][2];
      expect(url).toContain('sizeAttenuation=0');
    });
  });

  describe('generateShareURL', () => {
    it('generates full URL with state', () => {
      window.location = { origin: 'https://example.com', pathname: '/graph' } as Location;

      const state: AppState = {
        viewMode: '3d',
        filters: { subreddit: true, user: true, post: false, comment: false },
      };

      const url = generateShareURL(state);
      expect(url).toContain('https://example.com/graph');
      expect(url).toContain('view=3d');
      expect(url).toContain('f_subreddit=1');
    });

    it('returns empty string in non-browser environment', () => {
      const originalWindow = global.window;
      // @ts-expect-error - testing undefined window
      delete global.window;

      const url = generateShareURL({});
      expect(url).toBe('');

      global.window = originalWindow;
    });
  });
});
