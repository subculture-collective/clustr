import type { GraphData, GraphLink, GraphNode } from '../types/graph';

/** A stable, revision-pinned slice of the spatial graph. */
export interface SpatialScene {
  revision: string | null;
  nodes: GraphNode[];
  links: GraphLink[];
  nodeIds: readonly string[];
  positions: Float32Array;
  continuation?: string;
  source: 'manifest' | 'overview' | 'region' | 'legacy';
}

type SpatialGraphData = GraphData & { revision_id?: string | number; next_cursor?: string };

type Manifest = { revision_id?: string | number; revision?: string; current_revision?: string; id?: string };

function apiBase(): string {
  return (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '');
}

function revisionOf(value: Manifest): string | null {
  const revision = value.revision_id ?? value.revision ?? value.current_revision ?? value.id;
  return revision === undefined || revision === null ? null : String(revision);
}

function withRevision(path: string, revision: string | null): string {
  if (!revision) return path;
  const separator = path.includes('?') ? '&' : '?';
  return `${path}${separator}revision=${encodeURIComponent(revision)}`;
}

/**
 * Loads a scene without allowing an in-flight continuation to silently cross a
 * crawl revision.  Older backends simply fall back to the established graph
 * endpoint, retaining compatibility while not pretending that it is pinned.
 */
export class SpatialSceneClient {
  private revision: string | null = null;
  private manifestPromise: Promise<string | null> | null = null;
  private epoch = 0;

  public getRevision(): string | null { return this.revision; }

  public invalidate(): void { this.epoch++; this.revision = null; this.manifestPromise = null; }

  private async fetchJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
    const response = await fetch(`${apiBase()}${path}`, { signal, headers: { Accept: 'application/json' } });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return response.json() as Promise<T>;
  }

  private async pin(signal?: AbortSignal): Promise<string | null> {
    if (this.revision) return this.revision;
    if (!this.manifestPromise) {
      // The manifest pins the lifetime of this client, not one React effect.
      // Do not attach an individual effect's AbortSignal: StrictMode can abort
      // the first mount while the replacement mount still needs the same pin.
      this.manifestPromise = this.fetchJSON<Manifest>('/graph/manifest')
        .then(manifest => {
          this.revision = revisionOf(manifest);
          return this.revision;
        })
        .catch(error => {
          if ((error as Error).message === 'HTTP 404') return null;
          this.manifestPromise = null;
          throw error;
        });
    }
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    return this.manifestPromise;
  }

  private async load(path: string, source: SpatialScene['source'], signal?: AbortSignal): Promise<SpatialScene> {
    const epoch = ++this.epoch;
    const revision = await this.pin(signal);
    const data = await this.fetchJSON<SpatialGraphData>(withRevision(path, revision), signal);
    if (epoch !== this.epoch) throw new DOMException('Superseded spatial scene request', 'AbortError');
    const responseRevision = data.revision_id === undefined ? revision : String(data.revision_id);
    if (revision && responseRevision && responseRevision !== revision) throw new Error('Mixed graph revision response');
    const positions = new Float32Array(data.nodes.length * 3);
    const nodeIds = new Array<string>(data.nodes.length);
    for (let index = 0; index < data.nodes.length; index++) {
      const node = data.nodes[index];
      nodeIds[index] = node.id;
      positions[index * 3] = node.x ?? 0;
      positions[index * 3 + 1] = node.y ?? 0;
      positions[index * 3 + 2] = node.z ?? 0;
    }
    return { revision: responseRevision, nodes: data.nodes, links: data.links, nodeIds, positions, continuation: data.next_cursor, source };
  }

  public async overview(signal?: AbortSignal): Promise<SpatialScene> {
    try {
      // Far LOD is a navigational star chart, not the full route network. The
      // API orders routes by weight, so this retains only the strongest paths;
      // medium/near region loads reveal the denser local topology.
      return await this.load('/graph/overview?level=0&max_nodes=300&max_links=180&with_positions=true', 'overview', signal);
    } catch (error) {
      if ((error as Error).message !== 'HTTP 404') throw error;
      return this.load('/graph?max_nodes=300&max_links=180&with_positions=true', 'legacy', signal);
    }
  }

  public region(bounds: { xMin: number; xMax: number; yMin: number; yMax: number; zMin: number; zMax: number }, signal?: AbortSignal): Promise<SpatialScene> {
    const p = new URLSearchParams({ x_min: String(bounds.xMin), x_max: String(bounds.xMax), y_min: String(bounds.yMin), y_max: String(bounds.yMax), z_min: String(bounds.zMin), z_max: String(bounds.zMax), lod: 'medium', max_nodes: '5000', max_links: '20000' });
    return this.load(`/graph/region?${p}`, 'region', signal);
  }

  public community(stableId: string, lod: 'medium' | 'near' | 'inspect' = 'near', signal?: AbortSignal): Promise<SpatialScene> {
    return this.load(`/graph/community/${encodeURIComponent(stableId)}?lod=${lod}&max_nodes=5000&max_links=20000`, 'region', signal);
  }
}
