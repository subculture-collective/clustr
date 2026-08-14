import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Dashboard from './Dashboard';

describe('Dashboard', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows catalog totals separately from the analysis sample', async () => {
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({
        nodes: [{ id: 'subreddit_1', name: 'AskReddit', type: 'subreddit' }],
        links: [],
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        node_count: 7_066_559,
        link_count: 4_693_401,
      }), { status: 200 }));

    render(<Dashboard />);

    await waitFor(() => expect(screen.getByText('7.1M')).toBeInTheDocument());
    expect(screen.getByText('4.7M')).toBeInTheDocument();
    expect(screen.getByText('1 sampled for analysis')).toBeInTheDocument();
    expect(screen.getByText('Catalog Entities')).toBeInTheDocument();
    expect(screen.getByText('Sampled Nodes by Type')).toBeInTheDocument();
  });
});
