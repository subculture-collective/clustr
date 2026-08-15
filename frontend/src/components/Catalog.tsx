import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { SpatialSceneClient, type CatalogFacets, type CatalogItem, type CatalogResponse, type CatalogSort, type CatalogTreeResponse, type CatalogType } from '../data/SpatialSceneClient';

type CatalogMode = 'search' | 'browse' | 'tree';

interface Props {
  onLocate: (id: string) => void;
}

const client = new SpatialSceneClient();

function initialState() {
  const params = new URLSearchParams(window.location.search);
  const routeMatch = window.location.pathname.match(/^\/catalog\/(?:clusters|subreddits|users|posts|comments)\/(.+)$/);
  const mode = params.get('catalog_mode');
  const type = params.get('catalog_type');
  const sort = params.get('catalog_sort');
	const seededQuery = params.get('catalog_q') ?? '';
  return {
    mode: mode === 'browse' || mode === 'tree' ? mode : 'search' as CatalogMode,
    query: seededQuery,
    type: type === 'subreddit' || type === 'user' || type === 'post' || type === 'comment' ? type : '' as CatalogType,
    sort: sort === 'relevance' || sort === 'activity' || sort === 'size' || sort === 'distinctiveness' || sort === 'newest' ? sort : (seededQuery ? 'relevance' : 'activity') as CatalogSort,
    selected: params.get('catalog_selected') ?? (routeMatch ? decodeURIComponent(routeMatch[1]) : ''),
    treeRoot: params.get('catalog_tree') ?? '',
		cursor: params.get('catalog_cursor') ?? '',
  };
}

function countLabel(value: number) { return new Intl.NumberFormat().format(value); }

export default function Catalog({ onLocate }: Props) {
  const seeded = useMemo(initialState, []);
  const [mode, setMode] = useState<CatalogMode>(seeded.mode);
  const [query, setQuery] = useState(seeded.query);
  const [submittedQuery, setSubmittedQuery] = useState(seeded.query);
  const [type, setType] = useState<CatalogType>(seeded.type);
  const [sort, setSort] = useState<CatalogSort>(seeded.sort);
  const [selected, setSelected] = useState(seeded.selected);
  const [treeRoot, setTreeRoot] = useState(seeded.treeRoot);
	const [cursor, setCursor] = useState(seeded.cursor);
  const [catalog, setCatalog] = useState<CatalogResponse>();
  const [facets, setFacets] = useState<CatalogFacets>();
  const [tree, setTree] = useState<CatalogTreeResponse>();
  const [selectedDetail, setSelectedDetail] = useState<CatalogItem>();
	const [mobilePanel, setMobilePanel] = useState<'results'|'filters'|'detail'>('results');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [sensitiveOpen, setSensitiveOpen] = useState<Record<string, boolean>>({});
	const selectedItem = catalog?.items.find(item => item.id === selected) ?? selectedDetail;
  const lastGood = useRef<CatalogResponse | undefined>(undefined);

  const syncURL = useCallback((next: Partial<ReturnType<typeof initialState>> = {}, push = false) => {
    const params = new URLSearchParams(window.location.search);
    const state = { mode, query: submittedQuery, type, sort, selected, treeRoot, cursor, ...next };
    params.set('view', 'catalog'); params.set('catalog_mode', state.mode); params.set('catalog_sort', state.sort);
    if (state.query) params.set('catalog_q', state.query); else params.delete('catalog_q');
    if (state.type) params.set('catalog_type', state.type); else params.delete('catalog_type');
    if (state.selected) params.set('catalog_selected', state.selected); else params.delete('catalog_selected');
    if (state.treeRoot) params.set('catalog_tree', state.treeRoot); else params.delete('catalog_tree');
		if (state.cursor) params.set('catalog_cursor', state.cursor); else params.delete('catalog_cursor');
    window.history[push ? 'pushState' : 'replaceState']({}, '', `${window.location.pathname}?${params}`);
  }, [mode, submittedQuery, type, sort, selected, treeRoot, cursor]);

  useEffect(() => { syncURL(); }, [mode, submittedQuery, type, sort, selected, treeRoot, syncURL]);
	useEffect(() => {
		const restore = () => {
			const state = initialState();
			setMode(state.mode); setQuery(state.query); setSubmittedQuery(state.query); setType(state.type);
			setSort(state.sort); setSelected(state.selected); setTreeRoot(state.treeRoot); setCursor(state.cursor);
		};
		window.addEventListener('popstate', restore);
		return () => window.removeEventListener('popstate', restore);
	}, []);
  useEffect(() => {
    const controller = new AbortController();
    client.catalogFacets(controller.signal).then(setFacets).catch(() => undefined);
    return () => controller.abort();
  }, []);

  const load = useCallback((cursor?: string, append = false) => {
    const controller = new AbortController(); setLoading(true); setError('');
    client.catalogPage({ q: mode === 'search' ? submittedQuery : '', type, sort, cursor }, controller.signal)
      .then(response => {
        const merged = append && lastGood.current ? { ...response, items: [...lastGood.current.items, ...response.items], returned_count: lastGood.current.items.length + response.items.length } : response;
        lastGood.current = merged; setCatalog(merged);
        if (merged.items[0]) setSelected(current => current || merged.items[0].id);
      })
      .catch(reason => { if (reason?.name !== 'AbortError') setError(String(reason)); })
      .finally(() => setLoading(false));
    return controller;
  }, [mode, submittedQuery, type, sort]);

  useEffect(() => { const controller = load(cursor || undefined); return () => controller.abort(); }, [load, cursor]);
	useEffect(() => {
		if (!selected || catalog?.items.some(item => item.id === selected)) { setSelectedDetail(undefined); return; }
		const controller = new AbortController();
		client.catalogPage({ entityId: selected }, controller.signal).then(response => setSelectedDetail(response.items[0])).catch(() => setSelectedDetail(undefined));
		return () => controller.abort();
	}, [selected, catalog]);
	const loadTree = useCallback((treeCursor?: string, append = false) => {
		const controller = new AbortController(); setLoading(true);
		client.catalogTree(treeRoot, treeCursor, controller.signal).then(response => setTree(current => append && current ? {
			...response, children: [...current.children, ...response.children], returned_count: current.children.length + response.children.length,
		} : response)).catch(reason => { if (reason?.name !== 'AbortError') setError(String(reason)); }).finally(() => setLoading(false));
		return controller;
	}, [treeRoot]);
	useEffect(() => {
		if (mode !== 'tree' || !treeRoot) { setTree(undefined); return; }
		const controller = loadTree();
		return () => controller.abort();
	}, [mode, treeRoot, loadTree]);

  function selectItem(item: CatalogItem) { setSelected(item.id); setMobilePanel('detail'); syncURL({ selected: item.id }, true); }
  function openTree(id: string) { setTreeRoot(id); setMode('tree'); setMobilePanel('results'); syncURL({ mode: 'tree', treeRoot: id }, true); }

  return <div className="catalog-shell mx-auto w-full max-w-[96rem] px-3 md:px-6">
    <header className="mb-5 flex flex-col gap-4 border-b border-white/10 pb-5 lg:flex-row lg:items-end lg:justify-between">
      <div><p className="instrument-label">Public archive / revision pinned</p><h1 className="mt-3 text-3xl font-semibold text-white">Catalog</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-[#9aaba8]">Search and traverse every published subreddit, person, post, and comment without entering the map.</p></div>
      <div className="flex rounded-full border border-white/10 p-1" role="tablist" aria-label="Catalog modes">
        {(['search','browse','tree'] as const).map(value => <button key={value} role="tab" aria-selected={mode===value} data-active={mode===value} className="instrument-button min-h-11 rounded-full px-5 text-xs capitalize" onClick={() => { setMode(value); setCursor(''); }}>{value}</button>)}
      </div>
    </header>

    <form className="mb-5 flex gap-2" onSubmit={event => { event.preventDefault(); setSelected(''); setSubmittedQuery(query.trim()); setMode('search'); setCursor(''); setMobilePanel('results'); }}>
      <label className="sr-only" htmlFor="catalog-search">Search the public Catalog</label>
      <input id="catalog-search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search communities, users, posts, and comments" className="min-h-12 min-w-0 flex-1 rounded-xl border border-white/15 bg-black/30 px-4 text-white placeholder:text-[#61706e]" />
      <button className="instrument-button min-h-12 rounded-xl bg-[#b6ff62] px-5 font-semibold text-black">Search</button>
    </form>
		<div className="mb-3 grid grid-cols-3 gap-2 md:hidden" aria-label="Catalog mobile panels">
			{(['filters','results','detail'] as const).map(panel => <button key={panel} className="instrument-button min-h-11 rounded-lg border border-white/10 text-xs capitalize" aria-pressed={mobilePanel===panel} onClick={() => setMobilePanel(panel)}>{panel}</button>)}
		</div>

    <div className="catalog-grid">
      <aside className="instrument-panel catalog-filters rounded-2xl p-4" aria-label="Catalog filters" data-mobile-panel="filters" data-mobile-active={mobilePanel==='filters'}>
        <p className="instrument-label">Signal filters</p>
        <fieldset className="mt-4 space-y-2"><legend className="sr-only">Entity type</legend>
          {(['','subreddit','user','post','comment'] as CatalogType[]).map(value => { const count = value ? facets?.types.find(f => f.type===value)?.count : facets?.exact_total; return <label key={value || 'all'} className="flex min-h-11 cursor-pointer items-center justify-between rounded-lg px-3 hover:bg-white/5"><span className="flex items-center gap-3"><input type="radio" name="catalog-type" checked={type===value} onChange={() => { setSelected(''); setType(value); setCursor(''); setMobilePanel('results'); }} /> <span className="capitalize">{value || 'all records'}</span></span>{count!==undefined && <span className="font-mono text-xs text-[#9aaba8]">{countLabel(count)}</span>}</label>; })}
        </fieldset>
        <label className="mt-5 block text-xs text-[#9aaba8]" htmlFor="catalog-sort">Sort</label>
        <select id="catalog-sort" value={sort} onChange={event => { setSort(event.target.value as CatalogSort); setCursor(''); }} className="mt-2 min-h-11 w-full rounded-lg border border-white/15 bg-[#0a0e10] px-3">
          <option value="relevance">Relevance</option><option value="activity">Activity</option><option value="size">Size</option><option value="distinctiveness">Distinctiveness</option><option value="newest">Newest</option>
        </select>
      </aside>

      <section className="instrument-panel catalog-results rounded-2xl" aria-label="Catalog results" aria-busy={loading} data-mobile-panel="results" data-mobile-active={mobilePanel==='results'}>
        <div className="flex min-h-14 items-center justify-between border-b border-white/10 px-4"><p className="font-mono text-xs text-[#9aaba8]">{catalog ? `${countLabel(catalog.total_count)} matches · ${countLabel(catalog.returned_count)} loaded` : 'Loading published snapshot'}</p>{(loading || catalog?.stale || catalog?.partial) && <span className="rounded-full border border-amber-300/30 px-2 py-1 font-mono text-[10px] text-amber-200">{loading ? 'REFRESHING' : catalog?.partial ? 'PARTIAL' : 'STALE'}</span>}</div>
        {error && <div role="alert" className="border-b border-rose-400/20 bg-rose-400/10 p-4 text-sm text-rose-100">Catalog refresh failed. Last good results remain visible. {error}</div>}
        {mode === 'tree' ? <TreeResults tree={tree} onOpen={openTree} onSelect={id => { setSelected(id); setMobilePanel('detail'); }} onNext={next => loadTree(next, true)} /> : <ol className="catalog-list" aria-label="Published entities">{catalog?.items.map(item => <li key={item.id} className="catalog-row"><button className="min-h-16 w-full px-4 py-3 text-left" aria-current={selected===item.id ? 'true' : undefined} onClick={() => selectItem(item)}><span className="flex items-center justify-between gap-3"><span className="min-w-0"><span className="instrument-label">{item.type}{item.sensitive ? ' · sensitive' : ''}</span><strong className="mt-1 block truncate text-sm text-white">{item.label}</strong></span><span className="font-mono text-xs text-[#9aaba8]">{countLabel(item.value)}</span></span>{item.snippet && <span className="mt-2 line-clamp-2 block text-xs leading-5 text-[#9aaba8]">{item.snippet}</span>}</button></li>)}</ol>}
        {catalog?.next_cursor && mode !== 'tree' && <div className="border-t border-white/10 p-3"><button disabled={loading} className="instrument-button min-h-11 w-full rounded-lg border border-white/10 text-xs" onClick={() => { setCursor(catalog.next_cursor ?? ''); syncURL({ cursor: catalog.next_cursor ?? '' }, true); }}>Next 50</button></div>}
      </section>

      <aside className="instrument-panel catalog-detail rounded-2xl p-5" aria-label="Entity detail" data-mobile-panel="detail" data-mobile-active={mobilePanel==='detail'}>
        {selectedItem ? <><p className="instrument-label">{selectedItem.type} / {selectedItem.id}</p><h2 className="mt-3 break-words text-xl font-semibold text-white">{selectedItem.title || selectedItem.label}</h2>
          <div className="mt-4 flex flex-wrap gap-2"><button className="instrument-button min-h-11 rounded-full border border-white/10 px-4 text-xs" onClick={() => onLocate(selectedItem.id)}>Locate in Universe</button><button className="instrument-button min-h-11 rounded-full border border-white/10 px-4 text-xs" onClick={() => openTree(selectedItem.id)}>Open tree</button><button className="instrument-button min-h-11 rounded-full border border-white/10 px-4 text-xs" onClick={() => navigator.clipboard.writeText(`${location.origin}/catalog/${selectedItem.type}s/${encodeURIComponent(selectedItem.id)}`)}>Copy link</button></div>
          {selectedItem.sensitive ? <div className="mt-5 rounded-xl border border-amber-300/30 bg-amber-300/5 p-4"><p className="font-mono text-xs text-amber-200">SENSITIVE CONTENT · COLLAPSED</p><p className="mt-2 text-xs leading-5 text-[#c3c8c5]">Source or administrative sensitivity flags apply. Metadata remains visible.</p><button className="instrument-button mt-3 min-h-11 rounded-lg border border-amber-200/20 px-4 text-xs" aria-expanded={!!sensitiveOpen[selectedItem.id]} onClick={() => setSensitiveOpen(open => ({...open,[selectedItem.id]:!open[selectedItem.id]}))}>{sensitiveOpen[selectedItem.id] ? 'Collapse text' : 'Show text'}</button></div> : null}
          {(!selectedItem.sensitive || sensitiveOpen[selectedItem.id]) && <div className="mt-5 space-y-4 text-sm leading-6 text-[#d6e0dd]">{selectedItem.description && <p className="whitespace-pre-wrap">{selectedItem.description}</p>}{selectedItem.body && <p className="whitespace-pre-wrap">{selectedItem.body}</p>}</div>}
          <dl className="mt-5 grid grid-cols-2 gap-3 border-t border-white/10 pt-4 text-xs"><div><dt className="instrument-label">Value</dt><dd className="mt-2 font-mono">{countLabel(selectedItem.value)}</dd></div><div><dt className="instrument-label">Confidence</dt><dd className="mt-2 font-mono">{Math.round(selectedItem.affinity_confidence*100)}%</dd></div>{selectedItem.community_id && <div className="col-span-2"><dt className="instrument-label">Cluster</dt><dd className="mt-2 break-all font-mono">{selectedItem.community_id}</dd></div>}</dl>
          {selectedItem.source_permalink && <a href={selectedItem.source_permalink} target="_blank" rel="ugc nofollow noopener noreferrer" className="mt-5 inline-flex min-h-11 items-center text-sm text-[#b6ff62] underline">View public source</a>}
        </> : <div className="flex min-h-48 items-center justify-center text-center text-sm text-[#9aaba8]">Choose a result to inspect its complete public record.</div>}
      </aside>
    </div>
  </div>;
}

function TreeResults({ tree, onOpen, onSelect, onNext }: { tree?: CatalogTreeResponse; onOpen: (id:string)=>void; onSelect:(id:string)=>void; onNext:(cursor:string)=>void }) {
  if (!tree) return <div className="p-6 text-sm text-[#9aaba8]">Choose an entity from Search or Browse, then open its tree.</div>;
  return <div><div className="border-b border-white/10 p-4"><p className="instrument-label">{tree.root.type} containment</p><h2 className="mt-2 font-semibold text-white">{tree.root.label}</h2></div><ol className="catalog-list">{tree.children.map(child => <li key={child.id} className="catalog-row flex items-center gap-2 p-2"><button className="min-h-12 min-w-0 flex-1 rounded-lg px-3 text-left hover:bg-white/5" onClick={() => onSelect(child.id)}><span className="instrument-label">{child.type}{child.sensitive?' · sensitive':''}</span><strong className="mt-1 block truncate text-sm">{child.label}</strong></button>{child.child_count>0 && <button className="instrument-button min-h-11 rounded-lg border border-white/10 px-3 text-xs" onClick={() => onOpen(child.id)}>{child.child_count} children</button>}</li>)}</ol>{tree.next_cursor && <div className="border-t border-white/10 p-3"><button className="instrument-button min-h-11 w-full rounded-lg border border-white/10 text-xs" onClick={() => onNext(tree.next_cursor ?? '')}>Load more children</button></div>}{tree.cross_linked_authors.length>0 && <div className="border-t border-white/10 p-4"><p className="instrument-label">Cross-linked authors</p><div className="mt-2 flex flex-wrap gap-2">{tree.cross_linked_authors.map(author => <button key={author} className="instrument-button min-h-11 rounded-lg border border-white/10 px-3 font-mono text-xs" onClick={() => onSelect(author)}>{author}</button>)}</div></div>}</div>;
}
