import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSpatialWorldClient } from '../contexts/spatialWorld';
import type { PublishedCommunity } from '../data/SpatialSceneClient';
import type { CommunityResult } from '../utils/communityDetection';
import { publishedCommunityColor, publishedCommunityNumericID } from '../utils/publishedCommunities';

type CommunitiesProps = {
  onViewMode?: (mode: '3d' | '2d') => void;
  onFocusNode?: (id: string) => void;
  onApplyCommunityColors?: (result: CommunityResult) => void;
};

function colorResult(communities: PublishedCommunity[]): CommunityResult {
  const published = communities.map(community => {
    const id = publishedCommunityNumericID(community.id);
    return {
      id,
      nodes: [community.id],
      size: community.size,
      color: publishedCommunityColor(community.id),
      label: community.label,
      topNodes: [{ id: community.id, name: community.label, degree: 0 }],
    };
  });
  return {
    communities: published,
    nodeCommunities: new Map(published.map(item => [item.nodes[0], item.id])),
    modularity: 0,
  };
}

export default function Communities({ onViewMode, onFocusNode, onApplyCommunityColors }: CommunitiesProps) {
  const client = useSpatialWorldClient();
  const [communities, setCommunities] = useState<PublishedCommunity[]>([]);
  const [revision, setRevision] = useState<string | number | null>(null);
  const [catalog, setCatalog] = useState<string | number | null>(null);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (nextCursor?: string, signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      const page = await client.communities({ level: 0, limit: 50, cursor: nextCursor }, signal);
      setRevision(page.revision_id);
      setCatalog(page.spatial_catalog_id);
      setCommunities(current => nextCursor ? [...current, ...page.communities] : page.communities);
      setCursor(page.next_cursor);
    } catch (caught) {
      if ((caught as Error).name !== 'AbortError') setError((caught as Error).message);
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(undefined, controller.signal);
    return () => controller.abort();
  }, [load]);

  const colors = useMemo(() => colorResult(communities), [communities]);
  useEffect(() => {
    if (communities.length) onApplyCommunityColors?.(colors);
  }, [colors, communities.length, onApplyCommunityColors]);

  if (loading && communities.length === 0) return <div className="flex min-h-[60vh] items-center justify-center" role="status">Reading published landmarks…</div>;
  if (error && communities.length === 0) return <div className="flex min-h-[60vh] items-center justify-center text-red-300" role="alert">Unable to load places: {error}</div>;

  return (
    <div className="w-full px-4 text-white sm:px-6">
      <div className="mx-auto max-w-7xl">
        <header className="mb-8 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="instrument-label mb-3">Places / Published structure</p>
            <h1 className="text-3xl font-bold">Community landmarks</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-gray-400">
              Stable places from revision {revision}, catalog {catalog}. These are the same landmarks shown in Universe and Map.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button onClick={() => onViewMode?.('3d')} className="instrument-button rounded-full border-white/15 px-4 text-xs">Universe</button>
            <button onClick={() => onViewMode?.('2d')} className="instrument-button rounded-full border-white/15 px-4 text-xs">Map</button>
          </div>
        </header>

        <p className="mb-4 font-mono text-xs text-gray-400">{communities.length} landmark{communities.length === 1 ? '' : 's'} loaded</p>
        <ol className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {communities.map(community => (
            <li key={community.id}>
              <button
                className="group flex min-h-32 w-full items-start gap-4 rounded-2xl border border-white/10 bg-gray-800 p-5 text-left hover:border-white/25 hover:bg-gray-700"
                onClick={() => { onFocusNode?.(community.id); onViewMode?.('3d'); }}
              >
                <span className="mt-1 h-4 w-4 shrink-0 rounded-full" style={{ background: publishedCommunityColor(community.id) }} aria-hidden="true" />
                <span className="min-w-0">
                  <span className="block truncate text-lg font-semibold">{community.label}</span>
                  <span className="mt-2 block text-sm text-gray-400">{community.size.toLocaleString()} resident entities</span>
                  <span className="mt-4 block font-mono text-[10px] text-gray-500">{community.id}</span>
                </span>
              </button>
            </li>
          ))}
        </ol>

        {cursor && (
          <div className="mt-6 flex justify-center">
            <button disabled={loading} onClick={() => void load(cursor)} className="instrument-button rounded-full border-white/15 px-5 text-xs">
              {loading ? 'Loading…' : 'Load more landmarks'}
            </button>
          </div>
        )}
        {error && <p className="mt-4 text-center text-sm text-red-300" role="alert">Unable to load more landmarks: {error}</p>}
      </div>
    </div>
  );
}
