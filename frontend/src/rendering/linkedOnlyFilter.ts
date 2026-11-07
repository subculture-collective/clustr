export interface LinkedOnlyNode {
    id: string;
    type?: string;
}

export interface LinkedOnlyLink {
    source: string;
    target: string;
}

/**
 * "Linked only" hides nodes without a visible link so dense entity scenes
 * stay readable. Community landmarks are exempt: the overview carries a
 * bounded link budget (180 links for 300 landmarks on the production world),
 * so most landmarks are isolated by design and hiding them empties the map.
 * Scenes that carry no links at all (region scenes are requested with
 * max_links=0) are returned unchanged, because every node would vanish.
 */
export function filterLinkedOnly<N extends LinkedOnlyNode>(
    nodes: N[],
    links: LinkedOnlyLink[],
    sceneHasLinks: boolean,
): N[] {
    if (!sceneHasLinks) return nodes;
    const linked = new Set<string>();
    for (const link of links) {
        linked.add(link.source);
        linked.add(link.target);
    }
    return nodes.filter(node => node.type === 'community' || linked.has(node.id));
}
