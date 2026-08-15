import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Catalog from './Catalog';

const jsonResponse = (value: unknown) => Promise.resolve(new Response(JSON.stringify(value), {
  status: 200,
  headers: { 'Content-Type': 'application/json' },
}));

describe('Catalog', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/?view=catalog');
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/graph/manifest')) return jsonResponse({ revision_id: 7, spatial_catalog_id: 9 });
      if (url.includes('/graph/catalog/facets')) return jsonResponse({ revision_id: 7, spatial_catalog_id: 9, exact_total: 1, types: [{ type: 'post', count: 1 }], partial: false });
      if (url.includes('/graph/catalog/tree/')) return jsonResponse({
        revision_id: 7, spatial_catalog_id: 9, root: { id: 'post_sensitive', type: 'post', label: 'Sensitive field report' },
        children: [], returned_count: 0, cross_linked_authors: ['user_1'], partial: false, stale: false,
      });
      if (url.includes('/graph/catalog')) return jsonResponse({
        revision_id: 7, spatial_catalog_id: 9, returned_count: 1, total_count: 1,
        items: [{ id: 'post_sensitive', type: 'post', label: 'Sensitive field report', title: 'Sensitive field report', body: 'collapsed body', value: 12, sensitive: true, sensitivity_sources: ['source'], updated_at: '2026-08-15T00:00:00Z', metrics: {}, distinctiveness: .7, affinity_confidence: .8, source_permalink: 'https://www.reddit.com/example' }],
        applied_filters: {}, partial: false, stale: false,
      });
      return Promise.resolve(new Response('not found', { status: 404 }));
    }));
  });

  afterEach(() => vi.unstubAllGlobals());

  it('loads a pinned snapshot, collapses sensitive text, and exposes semantic tree controls', async () => {
    const user = userEvent.setup();
    render(<Catalog onLocate={vi.fn()} />);

    expect(await screen.findByRole('heading', { name: 'Sensitive field report' })).toBeInTheDocument();
    expect(screen.getByText('SENSITIVE CONTENT · COLLAPSED')).toBeInTheDocument();
    expect(screen.queryByText('collapsed body')).not.toBeInTheDocument();
    const source = screen.getByRole('link', { name: 'View public source' });
    expect(source).toHaveAttribute('rel', expect.stringContaining('nofollow'));

    await user.click(screen.getByRole('button', { name: 'Show text' }));
    expect(screen.getByText('collapsed body')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Open tree' }));
    expect(await screen.findByText('post containment')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'user_1' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'filters' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'filters' })).toHaveAttribute('aria-pressed', 'true'));
  });
});
