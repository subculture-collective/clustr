export interface GraphNode {
    id: string;
    name: string;
    val?: number;
    type?: 'subreddit' | 'user' | 'post' | 'comment' | string;
    // Optional precomputed layout positions (when provided by backend)
    x?: number;
    y?: number;
    z?: number;
    display_name?: string;
    evidence_label?: string;
    macro_group_id?: string;
    primary_color?: string;
    secondary_color?: string;
    bridge?: boolean;
    distinctiveness?: number;
    affinity_confidence?: number;
}

export interface GraphLink {
    source: string;
    target: string;
    weight?: number;
    affinity?: number;
    observed_evidence?: number;
    signal_composition?: Record<string, unknown>;
}

export interface GraphData {
    nodes: GraphNode[];
    links: GraphLink[];
}

export interface PaginationInfo {
    next_cursor?: string;
    has_more: boolean;
    page_size?: number;
}

export interface PaginatedGraphData {
    nodes: GraphNode[];
    links: GraphLink[];
    pagination?: PaginationInfo;
}
