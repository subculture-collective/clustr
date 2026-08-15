import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Communities from './Communities';
import { publishedCommunityColor } from '../utils/publishedCommunities';

describe('Communities', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders published landmarks without recomputing a sampled graph', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 42, spatial_catalog_id: 8 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        revision_id: 42,
        spatial_catalog_id: 8,
        level: 0,
        communities: [{ id: 'c:alpha', label: 'Alpha', size: 120, x: 1, y: 2, z: 3 }],
      }), { status: 200 }));

    render(<Communities />);

    await waitFor(() => expect(screen.getByRole('button', { name: /Alpha/ })).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: /recompute/i })).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.some(call => String(call[0]).match(/\/graph\?/))).toBe(false);
  });

  it('derives colors deterministically from stable community identity', () => {
    expect(publishedCommunityColor('c:alpha')).toBe(publishedCommunityColor('c:alpha'));
    expect(publishedCommunityColor('c:alpha')).not.toBe(publishedCommunityColor('c:beta'));
  });
});
