import type { GraphData, GraphLink, GraphNode } from '../types/graph';
import type { NodeDetails } from '../types/ui';

/** A stable, revision-pinned slice of the spatial graph. */
export interface SpatialScene {
  revision: string | null;
  catalog: string | null;
  nodes: GraphNode[];
  links: GraphLink[];
  nodeIds: readonly string[];
  positions: Float32Array;
  continuation?: string;
  source: 'manifest' | 'overview' | 'region' | 'legacy';
}

type SpatialGraphData = GraphData & { revision_id?: string | number; spatial_catalog_id?: string | number; next_cursor?: string };

type Manifest = { revision_id?: string | number; revision?: string; current_revision?: string; id?: string; spatial_catalog_id?: string | number };

export interface SpatialTelemetry {
  revision_id: string | number;
  spatial_catalog_id: string | number;
  totals: {
    entities: number;
    links: number;
    communities: number;
    by_type: Record<'subreddit' | 'user' | 'post' | 'comment', number>;
  };
  top_subreddits: Array<{
    id: string;
    name: string;
    subscribers: number;
    activity_count: number;
    unique_users: number;
  }>;
  top_users: Array<{
    id: string;
    name: string;
    posts: number;
    comments: number;
    activity_count: number;
    distinct_communities: number;
  }>;
}

export interface PublishedCommunity {
  id: string;
  label: string;
  size: number;
  x: number;
  y: number;
  z: number;
}

export interface PublishedCommunities {
  revision_id: string | number;
  spatial_catalog_id: string | number;
  level: number;
  communities: PublishedCommunity[];
  next_cursor?: string;
}

export interface SpatialSearchResponse {
  query: string;
  count: number;
  revision_id: string | number;
  results: Array<{
    ID: string;
    Name: string;
    Val: string;
    Type: { String: string; Valid: boolean } | null;
    PosX?: { Float64: number; Valid: boolean } | null;
    PosY?: { Float64: number; Valid: boolean } | null;
    PosZ?: { Float64: number; Valid: boolean } | null;
  }>;
}

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
  private catalog: string | null = null;
  private manifestPromise: Promise<string | null> | null = null;
  private epoch = 0;

  public getRevision(): string | null { return this.revision; }

  public invalidate(): void { this.epoch++; this.revision = null; this.catalog = null; this.manifestPromise = null; }

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
          this.catalog = manifest.spatial_catalog_id === undefined ? null : String(manifest.spatial_catalog_id);
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
    const responseCatalog = data.spatial_catalog_id === undefined ? this.catalog : String(data.spatial_catalog_id);
    if (revision && responseRevision && responseRevision !== revision) throw new Error('Mixed graph revision response');
    if (this.catalog && responseCatalog && responseCatalog !== this.catalog) throw new Error('Mixed spatial catalog response');
    const positions = new Float32Array(data.nodes.length * 3);
    const nodeIds = new Array<string>(data.nodes.length);
    for (let index = 0; index < data.nodes.length; index++) {
      const node = data.nodes[index];
      nodeIds[index] = node.id;
      positions[index * 3] = node.x ?? 0;
      positions[index * 3 + 1] = node.y ?? 0;
      positions[index * 3 + 2] = node.z ?? 0;
    }
    return { revision: responseRevision, catalog: responseCatalog, nodes: data.nodes, links: data.links, nodeIds, positions, continuation: data.next_cursor, source };
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

  public async telemetry(topLimit = 20, signal?: AbortSignal): Promise<SpatialTelemetry> {
    const revision = await this.pin(signal);
    return this.fetchJSON<SpatialTelemetry>(
      withRevision(`/graph/telemetry?top_limit=${Math.max(1, Math.min(100, Math.trunc(topLimit)))}`, revision),
      signal,
    );
  }

  public async communities(
    options: { level?: number; limit?: number; cursor?: string } = {},
    signal?: AbortSignal,
  ): Promise<PublishedCommunities> {
    const revision = await this.pin(signal);
    const parameters = new URLSearchParams({
      level: String(options.level ?? 0),
      limit: String(Math.max(1, Math.min(200, Math.trunc(options.limit ?? 50)))),
    });
    if (options.cursor) parameters.set('cursor', options.cursor);
    return this.fetchJSON<PublishedCommunities>(
      withRevision(`/graph/communities?${parameters}`, revision),
      signal,
    );
  }

  public async nodeDetails(entityID: string, neighborLimit = 20, signal?: AbortSignal): Promise<NodeDetails> {
    const revision = await this.pin(signal);
    const data = await this.fetchJSON<NodeDetails>(
      withRevision(`/nodes/${encodeURIComponent(entityID)}?neighbor_limit=${Math.max(1, Math.min(100, Math.trunc(neighborLimit)))}`, revision),
      signal,
    );
    if (revision && data.revision_id !== undefined && String(data.revision_id) !== revision) {
      throw new Error('Mixed graph revision response');
    }
    if (this.catalog && data.spatial_catalog_id !== undefined && String(data.spatial_catalog_id) !== this.catalog) {
      throw new Error('Mixed spatial catalog response');
    }
    return data;
  }

  public async search(query: string, limit = 10, signal?: AbortSignal): Promise<SpatialSearchResponse> {
    const revision = await this.pin(signal);
    const parameters = new URLSearchParams({ node: query, limit: String(Math.max(1, Math.min(500, Math.trunc(limit)))) });
    const data = await this.fetchJSON<SpatialSearchResponse>(withRevision(`/search?${parameters}`, revision), signal);
    if (revision && String(data.revision_id) !== revision) throw new Error('Mixed graph revision response');
    return data;
  }

  public region(bounds: { xMin: number; xMax: number; yMin: number; yMax: number; zMin: number; zMax: number }, signal?: AbortSignal): Promise<SpatialScene> {
    // Camera stops must stay within the region latency/integration budget.
    // Continuations can fill additional pages; the first meaningful local
    // scene is deliberately smaller than the renderer's resident capacity.
    const p = new URLSearchParams({ x_min: String(bounds.xMin), x_max: String(bounds.xMax), y_min: String(bounds.yMin), y_max: String(bounds.yMax), z_min: String(bounds.zMin), z_max: String(bounds.zMax), lod: 'medium', max_nodes: '1000', max_links: '0' });
    return this.load(`/graph/region?${p}`, 'region', signal);
  }

  public community(stableId: string, lod: 'medium' | 'near' | 'inspect' = 'near', signal?: AbortSignal): Promise<SpatialScene> {
    return this.load(`/graph/community/${encodeURIComponent(stableId)}?lod=${lod}&max_nodes=5000&max_links=20000`, 'region', signal);
  }

  public entity(entityId: string, signal?: AbortSignal): Promise<SpatialScene> {
    return this.load(`/graph/entity/${encodeURIComponent(entityId)}?max_nodes=5000&max_links=20000`, 'region', signal);
  }
}
