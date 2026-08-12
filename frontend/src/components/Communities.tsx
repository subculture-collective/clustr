import { useEffect, useState, useMemo, useCallback, useRef } from "react";
import type { GraphData } from "../types/graph";
import {
  detectCommunities,
  type CommunityResult,
} from "../utils/communityDetection";
import VirtualList from "./VirtualList";

type CommunitiesProps = {
  onViewMode?: (mode: "3d" | "2d") => void;
  onFocusNode?: (id: string) => void;
  onApplyCommunityColors?: (result: CommunityResult) => void;
};

export default function Communities({
  onViewMode,
  onFocusNode,
  onApplyCommunityColors,
}: CommunitiesProps) {
  const [graphData, setGraphData] = useState<GraphData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [communityResult, setCommunityResult] = useState<CommunityResult | null>(null);
  const [selectedCommunity, setSelectedCommunity] = useState<number | null>(null);
  const [computing, setComputing] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const initialLoad = useRef(true);

  const computeCommunities = useCallback(
    (data: GraphData) => {
      setComputing(true);

      // Clear any existing timeout to prevent state updates from previous calls
      if (timeoutRef.current !== null) {
        clearTimeout(timeoutRef.current);
      }

      // Use setTimeout to allow UI to update
      timeoutRef.current = setTimeout(() => {
        const result = detectCommunities(data);
        setCommunityResult(result);
        setComputing(false);

        // Notify parent to apply colors
        onApplyCommunityColors?.(result);
      }, 100);
    },
    [onApplyCommunityColors]
  );

  const loadGraph = useCallback(async () => {
    if (initialLoad.current) {
      setLoading(true);
    }
    setError(null);
    try {
      const base = (import.meta.env?.VITE_API_URL || "/api").replace(/\/$/, "");
      const response = await fetch(`${base}/graph?max_nodes=50000&max_links=100000`);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = (await response.json()) as GraphData;
      setGraphData(data);

      // Auto-compute communities on load
      if (data.nodes.length > 0) {
        computeCommunities(data);
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      if (initialLoad.current) {
        setLoading(false);
        initialLoad.current = false;
      }
    }
  }, [computeCommunities]);

  useEffect(() => {
    // Only fetch once on mount
    loadGraph();
    // Cleanup timeout on unmount
    return () => {
      if (timeoutRef.current !== null) {
        clearTimeout(timeoutRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (timeoutRef.current !== null) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  const handleRecompute = () => {
    if (graphData) {
      computeCommunities(graphData);
    }
  };

  const formatNumber = (n: number) => {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
    if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
    return n.toString();
  };

  const stats = useMemo(() => {
    if (!communityResult || !graphData) return null;

    const avgSize =
      communityResult.communities.reduce((sum, c) => sum + c.size, 0) /
      communityResult.communities.length;

    const largestCommunity = communityResult.communities[0]; // Already sorted by size
    const smallestCommunity =
      communityResult.communities[communityResult.communities.length - 1];

    // Calculate inter-community links
    let interCommunityLinks = 0;
    for (const link of graphData.links) {
      const sourceCommunity = communityResult.nodeCommunities.get(link.source);
      const targetCommunity = communityResult.nodeCommunities.get(link.target);
      if (
        sourceCommunity !== undefined &&
        targetCommunity !== undefined &&
        sourceCommunity !== targetCommunity
      ) {
        interCommunityLinks++;
      }
    }

    return {
      avgSize,
      largestCommunity,
      smallestCommunity,
      interCommunityLinks,
      intraCommunityLinks: graphData.links.length - interCommunityLinks,
    };
  }, [communityResult, graphData]);

  if (loading) {
    return (
      <div className="w-full h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-xl">Loading graph data...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="w-full h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-red-400">Error: {error}</div>
      </div>
    );
  }

  if (!graphData || !communityResult) {
    return (
      <div className="w-full h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-xl">
          {computing ? "Computing communities..." : "No data available"}
        </div>
      </div>
    );
  }

  const selectedCommunityData =
    selectedCommunity !== null
      ? communityResult.communities.find((c) => c.id === selectedCommunity)
      : null;

  return (
    <div className="w-full h-screen bg-gray-900 text-white overflow-auto p-6">
      <div className="max-w-7xl mx-auto">
        <div className="flex justify-between items-center mb-8">
          <div>
            <p className="instrument-label mb-3">Places / Calculated structure</p>
            <h1 className="text-3xl font-bold">Community landmarks</h1>
            <p className="text-gray-400 mt-2">
              Weighted partition · modularity:{" "}
              {communityResult.modularity.toFixed(4)}
            </p>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleRecompute}
              disabled={computing}
              className="px-4 py-2 bg-purple-600 hover:bg-purple-700 disabled:bg-gray-600 rounded"
            >
              {computing ? "Computing..." : "Recompute"}
            </button>
            <button
              onClick={() => onViewMode?.("3d")}
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded"
            >
              View 3D Graph
            </button>
            <button
              onClick={() => onViewMode?.("2d")}
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded"
            >
              View 2D Graph
            </button>
          </div>
        </div>

        {/* Overview Stats */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">Total Communities</div>
            <div className="text-3xl font-bold">
              {communityResult.communities.length}
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">Average Size</div>
            <div className="text-3xl font-bold">
              {stats?.avgSize.toFixed(1)}
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">Modularity Score</div>
            <div className="text-3xl font-bold">
              {communityResult.modularity.toFixed(3)}
            </div>
            <div className="text-xs text-gray-400 mt-1">
              (higher = better separation)
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">
              Inter-Community Links
            </div>
            <div className="text-3xl font-bold">
              {formatNumber(stats?.interCommunityLinks || 0)}
            </div>
          </div>
        </div>

        {/* Community Size Distribution */}
        <div className="bg-gray-800 rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4">
            Community Size Distribution
          </h2>
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Largest community:</span>
              <span className="font-semibold">
                {stats?.largestCommunity.size} nodes (
                {stats?.largestCommunity.label})
              </span>
            </div>
            <div className="flex justify-between text-sm">
              <span>Smallest community:</span>
              <span className="font-semibold">
                {stats?.smallestCommunity.size} nodes (
                {stats?.smallestCommunity.label})
              </span>
            </div>
            <div className="flex justify-between text-sm">
              <span>Intra-community links:</span>
              <span className="font-semibold">
                {formatNumber(stats?.intraCommunityLinks || 0)}
              </span>
            </div>
          </div>
        </div>

        {/* Communities List */}
        <div className="bg-gray-800 rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4">
            All Communities ({communityResult.communities.length})
          </h2>
          <VirtualList
            items={communityResult.communities}
            itemKey={(community) => String(community.id)}
            itemHeight={160}
            containerHeight={600}
            renderItem={(community) => (
              <div
                className={`p-4 rounded-lg border-2 cursor-pointer transition-all mb-4 ${
                  selectedCommunity === community.id
                    ? "border-white bg-gray-700"
                    : "border-gray-700 bg-gray-750 hover:border-gray-600"
                }`}
                style={{
                  borderLeftWidth: "6px",
                  borderLeftColor: community.color,
                }}
                onClick={() => setSelectedCommunity(community.id)}
              >
                <div className="flex items-center gap-3 mb-2">
                  <div
                    className="w-4 h-4 rounded"
                    style={{ backgroundColor: community.color }}
                  />
                  <div className="font-semibold truncate flex-1">
                    {community.label}
                  </div>
                </div>
                <div className="text-2xl font-bold mb-1">{community.size}</div>
                <div className="text-xs text-gray-400">
                  {((community.size / graphData.nodes.length) * 100).toFixed(1)}
                  % of nodes
                </div>
                {community.topNodes && community.topNodes.length > 0 && (
                  <div className="mt-3 pt-3 border-t border-gray-600">
                    <div className="text-xs text-gray-400 mb-1">Top nodes:</div>
                    <div className="space-y-1">
                      {community.topNodes.slice(0, 3).map((node) => (
                        <div
                          key={node.id}
                          className="text-xs truncate hover:text-blue-400 cursor-pointer"
                          onClick={(e) => {
                            e.stopPropagation();
                            onFocusNode?.(node.name);
                            onViewMode?.("3d");
                          }}
                        >
                          {node.name} ({node.degree})
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          />

          {/* Selected Community Details */}
          {selectedCommunityData && (
            <div className="bg-gray-800 rounded-lg p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div
                    className="w-6 h-6 rounded"
                    style={{ backgroundColor: selectedCommunityData.color }}
                  />
                  <h2 className="text-xl font-semibold">
                    {selectedCommunityData.label}
                  </h2>
                </div>
                <button
                  onClick={() => setSelectedCommunity(null)}
                  className="text-gray-400 hover:text-white"
                >
                  Close
                </button>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
                <div>
                  <div className="text-gray-400 text-sm">Community Size</div>
                  <div className="text-2xl font-bold">
                    {selectedCommunityData.size}
                  </div>
                </div>
                <div>
                  <div className="text-gray-400 text-sm">Percentage</div>
                  <div className="text-2xl font-bold">
                    {(
                      (selectedCommunityData.size / graphData.nodes.length) *
                      100
                    ).toFixed(1)}
                    %
                  </div>
                </div>
                <div>
                  <div className="text-gray-400 text-sm">Rank by Size</div>
                  <div className="text-2xl font-bold">
                    #
                    {communityResult.communities.findIndex(
                      (c) => c.id === selectedCommunityData.id
                    ) + 1}
                  </div>
                </div>
              </div>

              <div>
                <h3 className="text-lg font-semibold mb-3">
                  Top Nodes ({selectedCommunityData.topNodes?.length || 0})
                </h3>
                <VirtualList
                  items={selectedCommunityData.topNodes || []}
                  itemKey={(node, i) => String(node.id ?? i)}
                  itemHeight={64}
                  containerHeight={400}
                  renderItem={(node, i) => (
                    <div
                      className="flex items-center justify-between p-3 bg-gray-700 rounded hover:bg-gray-600 cursor-pointer mb-2"
                      onClick={() => {
                        onFocusNode?.(node.name);
                        onViewMode?.("3d");
                      }}
                    >
                      <div className="flex items-center gap-3">
                        <div className="text-gray-400 w-6">#{i + 1}</div>
                        <div className="font-medium">{node.name}</div>
                      </div>
                      <div className="text-right">
                        <div className="font-semibold">{node.degree}</div>
                        <div className="text-xs text-gray-400">connections</div>
                      </div>
                    </div>
                  )}
                />
              </div>

              <div className="mt-6 flex gap-2">
                <button
                  onClick={() => {
                    onViewMode?.("3d");
                  }}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded"
                >
                  View in 3D Graph
                </button>
                <button
                  onClick={() => {
                    onViewMode?.("2d");
                  }}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded"
                >
                  View in 2D Graph
                </button>
              </div>
            </div>
          )}

          <div className="mt-8 text-center text-gray-400 text-sm">
            <p>Communities detected using the Louvain algorithm</p>
            <p className="mt-1">
              Colors are automatically assigned to maximize visual distinction
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
