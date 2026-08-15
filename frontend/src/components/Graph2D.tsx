import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSpatialWorldClient } from '../contexts/spatialWorld';
import type { SpatialScene } from '../data/SpatialSceneClient';
import type { TypeFilters } from '../types/ui';
import { publishedCommunityColor } from '../utils/publishedCommunities';

type Graph2DProps = {
  filters: TypeFilters;
  minDegree?: number;
  maxDegree?: number;
  linkOpacity: number;
  nodeRelSize: number;
  physics: {
    chargeStrength: number;
    linkDistance: number;
    velocityDecay: number;
    cooldownTicks: number;
    collisionRadius: number;
  };
  subredditSize: 'subscribers' | 'activeUsers' | 'contentActivity' | 'interSubLinks';
  focusNodeId?: string;
  showLabels?: boolean;
  selectedId?: string;
  onNodeSelect?: (id?: string) => void;
  communityResult?: {
    nodeCommunities: Map<string, number>;
    communities: Array<{ id: number; color: string }>;
  } | null;
  usePrecomputedLayout?: boolean;
  initialCamera?: { x: number; y: number; zoom: number };
  onCameraChange?: (camera: { x: number; y: number; zoom: number }) => void;
};

type ViewBox = { x: number; y: number; width: number; height: number };

function frameScene(scene: SpatialScene): ViewBox {
  if (scene.nodes.length === 0) return { x: -100, y: -100, width: 200, height: 200 };
  const xs = scene.nodes.map(node => node.x ?? 0);
  const ys = scene.nodes.map(node => node.y ?? 0);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  const width = Math.max(40, maxX - minX);
  const height = Math.max(40, maxY - minY);
  const padding = Math.max(width, height) * 0.12 + 12;
  return { x: minX - padding, y: minY - padding, width: width + padding * 2, height: height + padding * 2 };
}

export default function Graph2D({
  linkOpacity,
  nodeRelSize,
  focusNodeId,
  showLabels = true,
  selectedId,
  onNodeSelect,
}: Graph2DProps) {
  const client = useSpatialWorldClient();
  const [scene, setScene] = useState<SpatialScene | null>(null);
  const [viewBox, setViewBox] = useState<ViewBox>({ x: -100, y: -100, width: 200, height: 200 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      const next = await client.overview(signal);
      setScene(next);
      setViewBox(frameScene(next));
    } catch (caught) {
      if ((caught as Error).name !== 'AbortError') setError((caught as Error).message);
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const byID = useMemo(() => new Map(scene?.nodes.map(node => [node.id, node]) ?? []), [scene]);
  const labels = useMemo(() => [...(scene?.nodes ?? [])]
    .sort((left, right) => (right.val ?? 0) - (left.val ?? 0) || left.id.localeCompare(right.id))
    .slice(0, 40), [scene]);

  useEffect(() => {
    if (!focusNodeId || !scene) return;
    const node = byID.get(focusNodeId);
    if (!node) return;
    const width = Math.max(30, viewBox.width / 3);
    const height = Math.max(30, viewBox.height / 3);
    setViewBox({ x: (node.x ?? 0) - width / 2, y: (node.y ?? 0) - height / 2, width, height });
  // Intentionally frame once for a new focus identity, not for viewBox changes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusNodeId, scene, byID]);

  if (loading && !scene) return <div className="flex h-full items-center justify-center" role="status">Projecting the published overview…</div>;

  return (
    <div className="relative h-screen w-full bg-[#030506]" data-revision={scene?.revision ?? 'legacy'}>
      <div className="instrument-panel absolute left-3 top-36 z-10 flex items-center gap-2 rounded-full p-1.5 text-white md:left-5 md:top-24">
        <button
          className="instrument-button rounded-full px-3 text-[10px]"
          onClick={() => {
            onNodeSelect?.(undefined);
            if (scene) setViewBox(frameScene(scene));
          }}
        >
          World view
        </button>
        <span className="px-2 font-mono text-[10px] text-gray-400">Published 2D projection</span>
      </div>
      {error && <div className="absolute left-3 top-48 z-20 rounded-lg bg-red-950/90 px-4 py-3 text-sm text-red-100" role="alert">Unable to load Map: {error}</div>}
      <svg
        className="h-full w-full touch-pan-x touch-pan-y"
        viewBox={`${viewBox.x} ${viewBox.y} ${viewBox.width} ${viewBox.height}`}
        role="application"
        aria-label="Published community map. Use Tab to move between landmarks and Enter to inspect."
      >
        <g aria-hidden="true">
          {scene?.links.map(link => {
            const source = byID.get(String(link.source));
            const target = byID.get(String(link.target));
            if (!source || !target) return null;
            return <line key={`${link.source}-${link.target}`} x1={source.x} y1={source.y} x2={target.x} y2={target.y} stroke="#a7c3bd" strokeOpacity={linkOpacity} strokeWidth={Math.max(0.35, viewBox.width / 1600)} />;
          })}
        </g>
        {scene?.nodes.map(node => {
          const selected = selectedId === node.id;
          const radius = Math.max(2.5, Math.pow(Math.max(1, node.val ?? 1), 0.25)) * Math.max(0.7, nodeRelSize / 5);
          return (
            <g key={node.id} transform={`translate(${node.x ?? 0} ${node.y ?? 0})`}>
              <circle
                r={radius}
                fill={publishedCommunityColor(node.id)}
                stroke={selected ? '#ffffff' : '#030506'}
                strokeWidth={selected ? 2 : 0.8}
                tabIndex={0}
                role="button"
                aria-label={`${node.name || node.id}, ${Number(node.val ?? 0).toLocaleString()} entities`}
                onClick={() => onNodeSelect?.(node.id)}
                onKeyDown={event => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    onNodeSelect?.(node.id);
                  }
                }}
              />
              <title>{node.name || node.id}</title>
            </g>
          );
        })}
        {showLabels && labels.map(node => (
          <text key={`label-${node.id}`} x={node.x ?? 0} y={(node.y ?? 0) - 7} textAnchor="middle" fill="#eef7f4" stroke="#030506" strokeWidth="1.5" paintOrder="stroke" fontSize={Math.max(4, viewBox.width / 120)} pointerEvents="none">
            {node.name || node.id}
          </text>
        ))}
      </svg>
    </div>
  );
}
