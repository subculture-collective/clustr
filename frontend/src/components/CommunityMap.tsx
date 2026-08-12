import * as d3 from "d3";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { GraphData } from "../types/graph";
import type { CommunityResult } from "../utils/communityDetection";
import { detectCommunities } from "../utils/communityDetection";

type Props = {
  communityResult?: CommunityResult | null;
  onBack?: () => void;
  onFocusNode?: (id: string) => void;
};

type D3Node = {
  id: string;
  name: string;
  type: "community" | "node";
  size: number; // visual radius
  color: string;
  originalId?: string; // when type === 'node'
  density?: number; // for communities
  memberCount?: number; // for communities
};

type D3Link = { source: string; target: string; weight: number };

// Helper function to calculate community density
function calculateCommunityDensity(
  communityNodes: string[],
  links: { source: string; target: string }[]
): number {
  const memberSet = new Set(communityNodes);
  let internalEdges = 0;
  for (const l of links) {
    if (memberSet.has(l.source) && memberSet.has(l.target)) {
      internalEdges++;
    }
  }
  const size = communityNodes.length;
  const possibleEdges = (size * (size - 1)) / 2;
  return possibleEdges > 0 ? internalEdges / possibleEdges : 0;
}

// Helper function to calculate label font size
function calculateLabelFontSize(nodeSize: number): number {
  const baseSize = 10;
  const sizeBonus = Math.min(8, Math.sqrt(nodeSize) * 1.2);
  return baseSize + sizeBonus;
}

export default function CommunityMap({
  communityResult,
  onBack,
  onFocusNode,
}: Props) {
  const [graph, setGraph] = useState<GraphData | null>(null);
  const [comm, setComm] = useState<CommunityResult | null>(
    communityResult ?? null
  );
  const [expanded, setExpanded] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const zoomTransformRef = useRef<d3.ZoomTransform | null>(null);
  const isFirstRenderRef = useRef(true);

  // React to prop changes
  useEffect(() => {
    if (communityResult) setComm(communityResult);
  }, [communityResult]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const base = (import.meta.env?.VITE_API_URL || "/api").replace(/\/$/, "");
      const url = `${base}/graph?max_nodes=50000&max_links=100000`;
      const r = await fetch(url);
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      const data = (await r.json()) as GraphData;
      setGraph(data);
      if (!communityResult) {
        const res = detectCommunities(data);
        setComm(res);
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [communityResult]);

  useEffect(() => {
    void load();
  }, [load]);

  // Build aggregated dataset for D3 depending on expansion state
  const aggregated = useMemo(() => {
    if (!graph || !comm) return null;
    const nodes: D3Node[] = [];
    const links: D3Link[] = [];

    if (expanded === null) {
      // Community-level supernodes
      for (const c of comm.communities) {
        const density = calculateCommunityDensity(c.nodes, graph.links);

        nodes.push({
          id: `community_${c.id}`,
          name: c.label || `Community ${c.id}`,
          type: "community",
          size: Math.max(4, Math.sqrt(c.size) * 2),
          color: c.color,
          density,
          memberCount: c.size,
        });
      }
      // Aggregate inter-community weights
      const w = new Map<string, number>();
      for (const l of graph.links) {
        const a = comm.nodeCommunities.get(l.source);
        const b = comm.nodeCommunities.get(l.target);
        if (a === undefined || b === undefined || a === b) continue;
        const key = a < b ? `${a}-${b}` : `${b}-${a}`;
        w.set(key, (w.get(key) || 0) + 1);
      }
      for (const [key, weight] of w) {
        const [aStr, bStr] = key.split("-");
        const a = Number(aStr);
        const b = Number(bStr);
        links.push({
          source: `community_${a}`,
          target: `community_${b}`,
          weight,
        });
      }
    } else {
      // One community expanded: include its member nodes + other supernodes
      const expandedComm = comm.communities.find((c) => c.id === expanded);
      if (!expandedComm) return null;
      const memberSet = new Set(expandedComm.nodes);

      // Add member nodes
      for (const n of graph.nodes) {
        if (memberSet.has(n.id)) {
          nodes.push({
            id: n.id,
            name: n.name || n.id,
            type: "node",
            size: 4,
            color: expandedComm.color,
            originalId: n.id,
          });
        }
      }
      // Other communities remain as supernodes
      for (const c of comm.communities) {
        if (c.id === expanded) continue;
        const density = calculateCommunityDensity(c.nodes, graph.links);

        nodes.push({
          id: `community_${c.id}`,
          name: c.label || `Community ${c.id}`,
          type: "community",
          size: Math.max(4, Math.sqrt(c.size) * 2),
          color: c.color,
          density,
          memberCount: c.size,
        });
      }

      // Intra-community edges only (for clarity)
      for (const l of graph.links) {
        const inA = memberSet.has(l.source);
        const inB = memberSet.has(l.target);
        if (inA && inB)
          links.push({ source: l.source, target: l.target, weight: 1 });
      }
    }

    return { nodes, links };
  }, [graph, comm, expanded]);

  // D3 render
  useEffect(() => {
    if (!aggregated || !svgRef.current || !containerRef.current) return;
    const container = containerRef.current;
    const width = container.clientWidth;
    const height = container.clientHeight;

    const svg = d3
      .select<SVGSVGElement, unknown>(svgRef.current)
      .attr("width", width)
      .attr("height", height);

    svg.selectAll("*").remove();
    const g = svg.append("g");

    const zoom = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.1, 10])
      .on("zoom", (event: d3.D3ZoomEvent<SVGSVGElement, unknown>) => {
        g.attr("transform", event.transform.toString());
        zoomTransformRef.current = event.transform;
      });
    svg.call(zoom);

    // Restore zoom state if it exists
    if (zoomTransformRef.current && !isFirstRenderRef.current) {
      svg.call(zoom.transform, zoomTransformRef.current);
    }

    type SimNode = D3Node & d3.SimulationNodeDatum;
    type SimLink = d3.SimulationLinkDatum<SimNode> & { weight: number };

    const nodes: SimNode[] = aggregated.nodes.map((n) => ({ ...n }));
    const links: SimLink[] = aggregated.links.map((l) => ({ ...l }));

    const sim = d3
      .forceSimulation<SimNode>(nodes)
      .force(
        "link",
        d3
          .forceLink<SimNode, SimLink>(links)
          .id((d) => d.id)
          .distance((d) => 40 + Math.min(200, 200 / Math.sqrt(d.weight || 1)))
      )
      .force("charge", d3.forceManyBody<SimNode>().strength(-200))
      .force("center", d3.forceCenter(width / 2, height / 2))
      .force(
        "collision",
        d3.forceCollide<SimNode>((d) => (d.size || 4) + 4)
      )
      .velocityDecay(0.88);

    const link = g
      .append("g")
      .attr("class", "links")
      .selectAll<SVGLineElement, SimLink>("line")
      .data(links)
      .enter()
      .append("line")
      .attr("stroke", "#888")
      .attr("stroke-opacity", 0.35)
      .attr(
        "stroke-width",
        (d) => 0.6 + Math.min(6, Math.log2((d.weight || 1) + 1))
      );

    const node = g
      .append("g")
      .attr("class", "nodes")
      .selectAll<SVGCircleElement, SimNode>("circle")
      .data(nodes)
      .enter()
      .append("circle")
      .attr("r", (d) => d.size)
      .attr("fill", (d) => d.color)
      .attr("stroke", "#111")
      .attr("stroke-width", 1)
      .style("cursor", "pointer")
      .on("click", (_e, d) => {
        if (d.type === "community") {
          const id = Number(d.id.split("_")[1]);
          setExpanded((prev) => (prev === id ? null : id));
        } else if (d.type === "node") {
          onFocusNode?.(d.name);
        }
      });

    // Create tooltip
    const tooltip = d3
      .select("body")
      .append("div")
      .attr("class", "community-map-tooltip")
      .style("position", "absolute")
      .style("background", "rgba(0, 0, 0, 0.9)")
      .style("color", "white")
      .style("padding", "8px 12px")
      .style("border-radius", "4px")
      .style("font-size", "12px")
      .style("pointer-events", "none")
      .style("opacity", "0")
      .style("z-index", "1000")
      .style("transition", "opacity 0.2s");

    node
      .on("mouseenter", function (_event, d) {
        d3.select(this).attr("stroke", "#fff").attr("stroke-width", 2);

        let content = `<strong>${d.name}</strong>`;
        if (d.type === "community" && d.memberCount !== undefined) {
          content += `<br/>Size: ${d.memberCount} nodes`;
          if (d.density !== undefined) {
            content += `<br/>Density: ${(d.density * 100).toFixed(1)}%`;
          }
          if (comm) {
            content += `<br/>Modularity: ${comm.modularity.toFixed(3)}`;
          }
        }

        tooltip.html(content).style("opacity", "1");
      })
      .on("mousemove", function (event) {
        tooltip
          .style("left", event.pageX + 10 + "px")
          .style("top", event.pageY + 10 + "px");
      })
      .on("mouseleave", function () {
        d3.select(this).attr("stroke", "#111").attr("stroke-width", 1);
        tooltip.style("opacity", "0");
      });

    // Only create labels for community-type nodes
    const communityNodes = nodes.filter((n) => n.type === "community");

    const label = g
      .append("g")
      .attr("class", "labels")
      .selectAll<SVGTextElement, SimNode>("text")
      .data(communityNodes)
      .enter()
      .append("text")
      .text((d) => (d.type === "community" ? d.name : ""))
      .attr("font-size", (d) => {
        if (d.type === "community") {
          return calculateLabelFontSize(d.size);
        }
        return 0;
      })
      .attr("fill", "#fff")
      .attr("text-anchor", "middle")
      .attr("pointer-events", "none")
      .style("user-select", "none")
      .style("font-weight", "600")
      .style("text-shadow", "0 0 3px rgba(0,0,0,0.8), 0 0 6px rgba(0,0,0,0.6)")
      .style("opacity", "0") // Start invisible for animation
      .transition()
      .duration(500)
      .delay((_, i) => i * 20)
      .style("opacity", "0.95");

    // Improved label deconfliction using force simulation
    type LabelNode = SimNode & { labelX?: number; labelY?: number };
    const labelNodes: LabelNode[] = nodes
      .filter((n) => n.type === "community")
      .map((n) => ({
        ...n,
        labelX: n.x,
        labelY: n.y,
      }));

    const labelSim = d3
      .forceSimulation<LabelNode>(labelNodes)
      .force("x", d3.forceX<LabelNode>((d) => d.x ?? 0).strength(0.1))
      .force("y", d3.forceY<LabelNode>((d) => d.y ?? 0).strength(0.1))
      .force(
        "collide",
        d3.forceCollide<LabelNode>((d) => {
          const fontSize = calculateLabelFontSize(d.size);
          return (d.name.length * fontSize) / 2 + 10;
        })
      )
      .stop();

    // Run label deconfliction for a few ticks
    for (let i = 0; i < 50; i++) {
      labelSim.tick();
    }

    sim.on("tick", () => {
      link
        .attr("x1", (d) => (typeof d.source === "object" ? d.source.x ?? 0 : 0))
        .attr("y1", (d) => (typeof d.source === "object" ? d.source.y ?? 0 : 0))
        .attr("x2", (d) => (typeof d.target === "object" ? d.target.x ?? 0 : 0))
        .attr("y2", (d) =>
          typeof d.target === "object" ? d.target.y ?? 0 : 0
        );
      node
        .attr("cx", (d) => d.x ?? 0)
        .attr("cy", (d) => d.y ?? 0);
      
      // Update label positions with deconflicted positions
      label.attr("x", (d, i) => {
        const labelNode = labelNodes[i];
        return labelNode?.labelX ?? d.x ?? 0;
      }).attr("y", (d, i) => {
        const labelNode = labelNodes[i];
        const yPos = labelNode?.labelY ?? d.y ?? 0;
        return yPos - (d.size || 4) - 6;
      });
    });

    // Auto-fit on first render
    if (isFirstRenderRef.current) {
      sim.on("end", () => {
        if (!isFirstRenderRef.current) return;

        // Calculate bounds
        const padding = 50;
        let minX = Infinity,
          maxX = -Infinity,
          minY = Infinity,
          maxY = -Infinity;

        nodes.forEach((n) => {
          if (n.x !== undefined && n.y !== undefined) {
            minX = Math.min(minX, n.x - (n.size || 4));
            maxX = Math.max(maxX, n.x + (n.size || 4));
            minY = Math.min(minY, n.y - (n.size || 4));
            maxY = Math.max(maxY, n.y + (n.size || 4));
          }
        });

        const graphWidth = maxX - minX;
        const graphHeight = maxY - minY;

        if (graphWidth > 0 && graphHeight > 0) {
          const scale = Math.min(
            (width - padding * 2) / graphWidth,
            (height - padding * 2) / graphHeight,
            3 // Max initial zoom
          );

          const centerX = (minX + maxX) / 2;
          const centerY = (minY + maxY) / 2;

          const transform = d3.zoomIdentity
            .translate(width / 2, height / 2)
            .scale(scale)
            .translate(-centerX, -centerY);

          svg
            .transition()
            .duration(750)
            .call(zoom.transform, transform);

          zoomTransformRef.current = transform;
        }

        isFirstRenderRef.current = false;
      });
    }

    sim.alpha(0.9).restart();
    return () => {
      sim.on('tick', null);
      sim.on('end', null);
      sim.stop();
      tooltip.remove();
    };
  }, [aggregated, comm, onFocusNode]);

  return (
    <div ref={containerRef} className="relative h-screen w-full bg-[#030506]">
      <div className="instrument-panel absolute left-3 top-36 z-10 flex items-center gap-3 rounded-xl px-3 py-2 text-sm text-white">
        <button
          className="instrument-button rounded-full px-3 text-xs"
          onClick={load}
        >
          Reload
        </button>
        <button
          className="instrument-button rounded-full px-3 text-xs"
          onClick={() => setExpanded(null)}
        >
          Collapse all
        </button>
        <span className="text-[#9aaba8]">
          {expanded === null
            ? "Community map"
            : `Expanded: Community ${expanded}`}
        </span>
      </div>
      <div className="instrument-panel absolute right-3 top-36 z-10 flex items-center gap-3 rounded-xl px-3 py-2 text-sm text-white">
        <button
          className="instrument-button rounded-full px-3 text-xs text-white"
          onClick={onBack}
        >
          Back
        </button>
      </div>
      {loading && (
        <div className="instrument-panel absolute left-3 top-36 z-20 rounded-xl px-3 py-2 text-sm text-white">
          Loading…
        </div>
      )}
      {error && (
        <div className="instrument-panel absolute left-3 top-36 z-20 rounded-xl border-red-400/30 px-3 py-2 text-sm text-red-100">
          Error: {error}
        </div>
      )}
      <svg ref={svgRef} className="w-full h-full" />
    </div>
  );
}
