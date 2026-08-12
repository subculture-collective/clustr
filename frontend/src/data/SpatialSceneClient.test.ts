import { describe, expect, it, vi } from 'vitest';
import { SpatialSceneClient } from './SpatialSceneClient';

describe('SpatialSceneClient', () => {
  it('pins the manifest revision onto overview requests', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision: 'crawl-42' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ nodes: [], links: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const client = new SpatialSceneClient();

    const scene = await client.overview();

    expect(scene.revision).toBe('crawl-42');
    expect(String(fetchMock.mock.calls[1][0])).toContain('revision=crawl-42');
  });

  it('uses the legacy graph Adapter when overview is unavailable', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('', { status: 404 }))
      .mockResolvedValueOnce(new Response('', { status: 404 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ nodes: [], links: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const scene = await new SpatialSceneClient().overview();

    expect(scene.source).toBe('legacy');
  });

  it('uses the bounded revision region contract', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, nodes: [], links: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await new SpatialSceneClient().region({
      xMin: -1,
      xMax: 1,
      yMin: -2,
      yMax: 2,
      zMin: -3,
      zMax: 3,
    });

    const request = String(fetchMock.mock.calls[1][0]);
    expect(request).toContain('/graph/region?');
    expect(request).toContain('max_nodes=1000');
    expect(request).toContain('max_links=5000');
    expect(request).toContain('revision=9');
  });

  it('bounds selected community neighborhoods', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, nodes: [], links: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await new SpatialSceneClient().community('c:new:one', 'near');

    const request = String(fetchMock.mock.calls[1][0]);
    expect(request).toContain('/graph/community/c%3Anew%3Aone?');
    expect(request).toContain('max_nodes=5000');
    expect(request).toContain('max_links=20000');
    expect(request).toContain('revision=9');
  });

  it('single-flights concurrent manifest pinning and never sends an unpinned slice', async () => {
    let resolveManifest!: (response: Response) => void;
    const manifest = new Promise<Response>(resolve => { resolveManifest = resolve; });
    const fetchMock = vi.fn()
      .mockReturnValueOnce(manifest)
      .mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ revision_id: 9, nodes: [], links: [] }), { status: 200 })));
    vi.stubGlobal('fetch', fetchMock);
    const client = new SpatialSceneClient();

    const first = client.overview().catch(error => error);
    const second = client.overview();
    resolveManifest(new Response(JSON.stringify({ revision_id: 9 }), { status: 200 }));

    await second;
    await first;
    const graphRequests = fetchMock.mock.calls
      .map(call => String(call[0]))
      .filter(path => path.includes('/graph/overview'));
    expect(fetchMock.mock.calls.filter(call => String(call[0]).includes('/graph/manifest'))).toHaveLength(1);
    expect(graphRequests.length).toBeGreaterThan(0);
    expect(graphRequests.every(path => path.includes('revision=9'))).toBe(true);
  });

  it('loads a bounded full-corpus entity neighborhood', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 4 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 4, nodes: [], links: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const scene = await new SpatialSceneClient().entity('comment_xyz');

    expect(scene.catalog).toBe('4');
    expect(String(fetchMock.mock.calls[1][0])).toContain('/graph/entity/comment_xyz?');
    expect(String(fetchMock.mock.calls[1][0])).toContain('revision=9');
  });

  it('rejects a slice from a different spatial catalog', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 4 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ revision_id: 9, spatial_catalog_id: 5, nodes: [], links: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(new SpatialSceneClient().overview()).rejects.toThrow('Mixed spatial catalog response');
  });
});
