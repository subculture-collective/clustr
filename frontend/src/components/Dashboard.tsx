import { useEffect, useState } from 'react';
import { useSpatialWorldClient } from '../contexts/spatialWorld';
import type { SpatialTelemetry } from '../data/SpatialSceneClient';

type DashboardProps = {
  onViewMode?: (mode: '3d' | '2d') => void;
  onFocusNode?: (id: string) => void;
};

function formatNumber(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toLocaleString();
}

export default function Dashboard({ onViewMode, onFocusNode }: DashboardProps) {
  const client = useSpatialWorldClient();
  const [telemetry, setTelemetry] = useState<SpatialTelemetry | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    client.telemetry(20, controller.signal)
      .then(setTelemetry)
      .catch((caught: Error) => {
        if (caught.name !== 'AbortError') setError(caught.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [client]);

  if (loading) return <div className="flex min-h-[60vh] items-center justify-center" role="status">Reading the published world…</div>;
  if (error) return <div className="flex min-h-[60vh] items-center justify-center text-red-300" role="alert">Unable to load telemetry: {error}</div>;
  if (!telemetry) return null;

  const { totals } = telemetry;
  const summaryCards = [
    { label: 'Catalog Entities', display: formatNumber(totals.entities), title: totals.entities.toLocaleString() },
    { label: 'Catalog Links', display: formatNumber(totals.links), title: totals.links.toLocaleString() },
    { label: 'Community Landmarks', display: formatNumber(totals.communities), title: totals.communities.toLocaleString() },
    { label: 'Graph Revision', display: String(telemetry.revision_id), title: String(telemetry.revision_id) },
  ];
  return (
    <div className="w-full px-4 text-white sm:px-6">
      <div className="mx-auto max-w-7xl">
        <header className="mb-8 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="instrument-label mb-3">Data / Published world</p>
            <h1 className="text-3xl font-bold">Universe telemetry</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-gray-400">
              Exact measurements from revision {telemetry.revision_id}, catalog {telemetry.spatial_catalog_id}. Refresh the page to adopt a newer publication.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button onClick={() => onViewMode?.('3d')} className="instrument-button rounded-full border-white/15 px-4 text-xs">Universe</button>
            <button onClick={() => onViewMode?.('2d')} className="instrument-button rounded-full border-white/15 px-4 text-xs">Map</button>
          </div>
        </header>

        <section aria-label="Published totals" className="mb-8 grid grid-cols-2 gap-3 lg:grid-cols-4">
          {summaryCards.map(card => (
            <article key={card.label} className="min-w-0 rounded-2xl bg-gray-800 p-5">
              <p className="text-sm text-gray-400">{card.label}</p>
              <p className="mt-2 truncate text-2xl font-bold sm:text-3xl" title={card.title}>{card.display}</p>
            </article>
          ))}
        </section>

        <section className="mb-8 rounded-2xl bg-gray-800 p-5">
          <h2 className="text-xl font-semibold">Published Entities by Type</h2>
          <div className="mt-5 grid grid-cols-2 gap-4 md:grid-cols-4">
            {Object.entries(totals.by_type).map(([type, count]) => (
              <div key={type}>
                <p className="capitalize text-gray-400">{type}</p>
                <p className="mt-1 text-xl font-semibold" title={count.toLocaleString()}>{formatNumber(count)}</p>
              </div>
            ))}
          </div>
        </section>

        <div className="grid gap-6 lg:grid-cols-2">
          <section className="rounded-2xl bg-gray-800 p-5">
            <h2 className="text-xl font-semibold">Top Subreddits</h2>
            <ol className="mt-4 divide-y divide-white/10">
              {telemetry.top_subreddits.map((item, index) => (
                <li key={item.id}>
                  <button className="flex min-h-14 w-full items-center justify-between gap-4 rounded-lg px-2 text-left hover:bg-white/5" onClick={() => { onFocusNode?.(item.id); onViewMode?.('3d'); }}>
                    <span className="min-w-0"><span className="mr-3 font-mono text-gray-500">{String(index + 1).padStart(2, '0')}</span><span className="truncate font-medium">{item.name}</span></span>
                    <span className="shrink-0 text-right"><span className="block font-semibold">{formatNumber(item.subscribers)}</span><span className="text-xs text-gray-400">subscribers</span></span>
                  </button>
                </li>
              ))}
            </ol>
          </section>

          <section className="rounded-2xl bg-gray-800 p-5">
            <h2 className="text-xl font-semibold">Most Active People</h2>
            <ol className="mt-4 divide-y divide-white/10">
              {telemetry.top_users.map((item, index) => (
                <li key={item.id}>
                  <button className="flex min-h-14 w-full items-center justify-between gap-4 rounded-lg px-2 text-left hover:bg-white/5" onClick={() => { onFocusNode?.(item.id); onViewMode?.('3d'); }}>
                    <span className="min-w-0"><span className="mr-3 font-mono text-gray-500">{String(index + 1).padStart(2, '0')}</span><span className="truncate font-medium">{item.name}</span></span>
                    <span className="shrink-0 text-right"><span className="block font-semibold">{formatNumber(item.activity_count)}</span><span className="text-xs text-gray-400">posts + comments</span></span>
                  </button>
                </li>
              ))}
            </ol>
          </section>
        </div>
      </div>
    </div>
  );
}
