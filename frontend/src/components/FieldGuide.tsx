import { useEffect, useRef } from 'react';

interface FieldGuideProps { open: boolean; onClose: () => void }

const visualTerms = [
  ['Landmarks', 'Large luminous bodies are stable community territories. Their position is preserved between published worlds so the universe remains learnable.'],
  ['Routes', 'Lines express relationships. Repeated activity and subreddit overlap are stored as weighted routes; authorship, publishing, comments, and replies are projected from each object’s canonical references.'],
  ['Scale', 'Distance changes meaning. Far away you see community landmarks; zooming streams the camera’s bounded 3D region. Posts appear at inspect scale, and selecting a post or object reveals its labeled comments and relationships.'],
  ['Color', 'Acid green marks subreddits, cyan marks people, amber marks posts, and rose marks comments. Landmark color may instead identify a community.'],
];

const pipeline = [
  ['01', 'Collect', 'Recurring workers gather subreddit metadata, posts, comments, authors, and discovered communities.'],
  ['02', 'Project', 'Source records become typed weighted routes plus canonical author, parent, post, and community references.'],
  ['03', 'Place', 'The full catalog gives every subreddit, person, post, and comment a semantic label and reproducible 3D coordinate.'],
  ['04', 'Publish', 'Validation checks complete source coverage, labels, coordinates, bounds, and links before an immutable revision atomically references the catalog.'],
  ['05', 'Stream + render', 'The browser pins that revision, loads only the overview or camera region, and draws the bounded scene with GPU-instanced Three.js and collision-managed labels.'],
];

const calculations = [
  ['Relationship weight', 'Repeated evidence accumulates instead of producing duplicate lines. Shared activity strengthens overlap routes; authorship, publishing, comments, and replies remain directed when projected into the scene.'],
  ['Community structure', 'A deterministic weighted community calculation finds strongly connected territories. The launch world uses its bounded top-level partition; complete multi-level coverage is the next refinement.'],
  ['Stable identity', 'A new community inherits a prior landmark ID when the strongest membership match is mutual. Existing landmark coordinates are warm-started so familiar territories do not jump arbitrarily.'],
  ['Three-axis layout', 'A bounded force pass places the strongest connected core in three axes. The full catalog warm-starts old objects, anchors people and content to their semantic homes, and deterministically seeds anything new.'],
  ['Publication checks', 'Promotion requires exact per-type source counts at the watermark, nonempty labels, finite noncollapsed XYZ bounds, valid endpoints, and nonempty community landmarks. A failed build leaves the visible world unchanged.'],
];

export default function FieldGuide({ open, onClose }: FieldGuideProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key !== 'Tab' || !dialog) return;
      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>('button, [href], input, [tabindex]:not([tabindex="-1"])')).filter(element => !element.hasAttribute('disabled'));
      if (!focusable.length) return;
      const first = focusable[0]; const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener('keydown', handleKey);
    return () => { document.removeEventListener('keydown', handleKey); previous?.focus(); };
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div ref={dialogRef} className="fixed inset-0 z-[90] overflow-y-auto bg-[#030506]/88 p-3 backdrop-blur-xl md:p-8" role="dialog" aria-modal="true" aria-labelledby="field-guide-title">
      <div className="instrument-panel mx-auto min-h-full max-w-6xl rounded-[1.5rem] p-6 md:min-h-0 md:p-10">
        <header className="flex items-start justify-between gap-6 border-b border-white/10 pb-8">
          <div>
            <p className="instrument-label mb-4">Field guide / Clustr universe</p>
            <h2 id="field-guide-title" className="max-w-3xl text-3xl font-semibold tracking-[-.04em] text-white md:text-5xl">A living map of communities and the people moving between them.</h2>
            <p className="mt-5 max-w-2xl text-sm leading-7 text-[#9aaba8] md:text-base">This is not a literal astronomy simulation. It is a semantic spatial model: proximity, routes, scale, and hierarchy turn relationships in Reddit activity into a world you can learn and revisit.</p>
          </div>
          <button autoFocus onClick={onClose} className="instrument-button h-11 w-11 shrink-0 rounded-full" aria-label="Close field guide">×</button>
        </header>

        <div className="grid gap-10 py-9 lg:grid-cols-[.85fr_1.15fr]">
          <section aria-labelledby="reading-title">
            <p className="instrument-label mb-3">What you are looking at</p>
            <h3 id="reading-title" className="text-2xl font-semibold tracking-tight">How to read the universe</h3>
            <div className="mt-6 divide-y divide-white/10 border-y border-white/10">
              {visualTerms.map(([term, description], index) => <div key={term} className="grid grid-cols-[2rem_1fr] gap-4 py-5"><span className="font-mono text-xs text-[#b6ff62]">0{index + 1}</span><div><h4 className="font-semibold text-white">{term}</h4><p className="mt-2 text-sm leading-6 text-[#9aaba8]">{description}</p></div></div>)}
            </div>
          </section>
          <section aria-labelledby="pipeline-title">
            <p className="instrument-label mb-3">How it is done</p>
            <h3 id="pipeline-title" className="text-2xl font-semibold tracking-tight">From crawl to constellation</h3>
            <ol className="mt-6 space-y-3">
              {pipeline.map(([number, title, description]) => <li key={number} className="grid grid-cols-[2.5rem_1fr] gap-4 rounded-xl border border-white/10 bg-white/[.025] p-4"><span className="font-mono text-xs text-[#b6ff62]">{number}</span><div><h4 className="font-semibold text-white">{title}</h4><p className="mt-1 text-sm leading-6 text-[#9aaba8]">{description}</p></div></li>)}
            </ol>
          </section>
        </div>
        <section className="border-t border-white/10 py-9" aria-labelledby="calculation-title">
          <div className="grid gap-8 lg:grid-cols-[.55fr_1.45fr]">
            <div><p className="instrument-label mb-3">Calculation model</p><h3 id="calculation-title" className="text-2xl font-semibold tracking-tight">Why objects end up where they do</h3><p className="mt-4 text-sm leading-6 text-[#9aaba8]">The picture is calculated from repeated evidence, not arranged by hand. The goal is a stable, useful geography—not a claim that there is one mathematically perfect map.</p></div>
            <div className="grid gap-px overflow-hidden rounded-xl border border-white/10 bg-white/10 sm:grid-cols-2">
              {calculations.map(([title, description], index) => <article key={title} className={`bg-[#090d0f] p-5 ${index === calculations.length - 1 ? 'sm:col-span-2' : ''}`}><p className="font-mono text-[10px] tracking-[.18em] text-[#b6ff62]">CALC / 0{index + 1}</p><h4 className="mt-3 font-semibold text-white">{title}</h4><p className="mt-2 text-sm leading-6 text-[#9aaba8]">{description}</p></article>)}
            </div>
          </div>
        </section>
        <footer className="flex flex-col gap-4 border-t border-white/10 pt-6 text-sm text-[#9aaba8] sm:flex-row sm:items-center sm:justify-between"><p>Drag to orbit · scroll to travel · click a landmark to approach · press / to search</p><button onClick={onClose} className="instrument-button rounded-full border-white/15 px-5 text-white">Enter the universe <span aria-hidden="true">→</span></button></footer>
      </div>
    </div>
  );
}
