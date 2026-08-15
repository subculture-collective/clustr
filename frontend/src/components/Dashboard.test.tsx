import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Dashboard from './Dashboard';

describe('Dashboard', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows exact published-world telemetry without downloading a graph sample', async () => {
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({
        revision_id: 42,
        spatial_catalog_id: 8,
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        revision_id: 42,
        spatial_catalog_id: 8,
        totals: {
          entities: 7_066_559,
          links: 4_693_401,
          communities: 31,
          by_type: { subreddit: 1200, user: 6000, post: 59000, comment: 7000359 },
        },
        top_subreddits: [{ id: 'subreddit_1', name: 'AskReddit', subscribers: 50_000_000, activity_count: 45, unique_users: 12 }],
        top_users: [{ id: 'user_1', name: 'reader', posts: 2, comments: 8, activity_count: 10, distinct_communities: 3 }],
      }), { status: 200 }));

    render(<Dashboard />);

    await waitFor(() => expect(screen.getByText('7.1M')).toBeInTheDocument());
    expect(screen.getByText('4.7M')).toBeInTheDocument();
    expect(screen.getByText('Catalog Entities')).toBeInTheDocument();
    expect(screen.getByText('Published Entities by Type')).toBeInTheDocument();
    expect(screen.queryByText(/sample/i)).not.toBeInTheDocument();
    expect(vi.mocked(globalThis.fetch).mock.calls.some(call => String(call[0]).match(/\/graph\?/))).toBe(false);
  });
});
