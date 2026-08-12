import { useState, useEffect } from "react";
import type { SelectedInfo, NodeDetails, NeighborInfo } from "../types/ui";
import VirtualList from "./VirtualList";
import { useMobileDetect } from "../hooks/useMobileDetect";

interface Props {
  selected?: SelectedInfo;
  onClear: () => void;
  onFocus: (id: string) => void;
}

type Tab = "overview" | "connections" | "statistics";

export default function Inspector({ selected, onClear, onFocus }: Props) {
  const [nodeDetails, setNodeDetails] = useState<NodeDetails | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("overview");
  const [isVisible, setIsVisible] = useState(false);
  const { isMobile } = useMobileDetect();

  // Fetch detailed node information when selection changes
  useEffect(() => {
    if (!selected?.id) {
      setNodeDetails(null);
      setError(null);
      return;
    }

    const abortController = new AbortController();
    const currentSelectionId = selected.id;

    const fetchNodeDetails = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const rawApiUrl = import.meta.env.VITE_API_URL || '/api';
        const apiUrl = rawApiUrl.endsWith('/') && rawApiUrl !== '/' 
          ? rawApiUrl.slice(0, -1) 
          : rawApiUrl;
        const response = await fetch(`${apiUrl}/nodes/${encodeURIComponent(selected.id)}?neighbor_limit=20`, {
          signal: abortController.signal
        });
        
        if (!response.ok) {
          throw new Error(`Failed to fetch node details: ${response.statusText}`);
        }
        
        const data: NodeDetails = await response.json();
        
        // Guard against stale responses - only update if this is still the current selection
        if (currentSelectionId === selected.id) {
          setNodeDetails(data);
        }
      } catch (err) {
        // Ignore abort errors - these are expected when switching nodes quickly
        if (err instanceof Error && err.name === 'AbortError') {
          return;
        }
        
        console.error('Error fetching node details:', err);
        setError(err instanceof Error ? err.message : 'Failed to load node details');
        // Fall back to basic selected info only if it has connections
        if (selected.degree && selected.degree > 0) {
          const fallbackNeighbors: NeighborInfo[] = (selected.neighbors || []).map(
            (neighbor) => ({
              id: neighbor.id,
              name: neighbor.name || neighbor.id,
              val: '1',
              type: neighbor.type,
              degree: 0,
            })
          );
          setNodeDetails({
            ...selected,
            neighbors: fallbackNeighbors,
          });
        }
      } finally {
        if (currentSelectionId === selected.id) {
          setLoading(false);
        }
      }
    };

    fetchNodeDetails();

    // Cleanup: abort fetch if selection changes before it completes
    return () => {
      abortController.abort();
    };
  }, [selected]);

  const hasConnectionsCheck = Boolean(
    (typeof selected?.degree === "number" && selected.degree > 0) ||
    (selected?.neighbors && selected.neighbors.length > 0) ||
    (nodeDetails?.neighbors && nodeDetails.neighbors.length > 0)
  );

  // Trigger slide-in animation when component mounts/unmounts
  useEffect(() => {
    if (!selected || (!hasConnectionsCheck && !loading)) {
      setIsVisible(false);
    } else {
      // Delay to ensure CSS transition works
      const timer = setTimeout(() => setIsVisible(true), 10);
      return () => clearTimeout(timer);
    }
  }, [selected, loading, hasConnectionsCheck]);

  if (!selected) return null;

  // Only show inspector when the selected node has at least one connection or we're loading/have data
  const hasConnections =
    (typeof selected.degree === "number" && selected.degree > 0) ||
    (selected.neighbors && selected.neighbors.length > 0) ||
    (nodeDetails && nodeDetails.neighbors && nodeDetails.neighbors.length > 0);
    
  if (!hasConnections && !loading) return null;

  const neighbors = nodeDetails?.neighbors || selected.neighbors || [];
  const displayName = nodeDetails?.name || selected.name || selected.id;
  const displayType = nodeDetails?.type || selected.type;
  const displayVal = nodeDetails?.val;
  const degree = nodeDetails?.degree !== undefined ? nodeDetails.degree : 
                 (selected.degree !== undefined ? selected.degree : neighbors.length);
  
  // Type guard to check if neighbors are NeighborInfo (with degree field)
  const isNeighborInfo = (n: unknown): n is NeighborInfo => typeof n === 'object' && n !== null && 'degree' in n && typeof (n as { degree?: unknown }).degree === 'number';

  return (
    <div className={`fixed z-30 pointer-events-none
      ${isMobile 
        ? 'bottom-20 left-3 right-3 top-auto h-[60vh]'
        : 'right-5 top-24 h-[calc(100vh-8.5rem)]'
      }`}>
      <div 
        className={`instrument-panel pointer-events-auto flex h-full flex-col overflow-hidden rounded-2xl text-white
                   transition-transform duration-300 ease-in-out ${
                     isMobile 
                       ? `w-full ${isVisible ? 'translate-y-0' : 'translate-y-full'}` /* Mobile: slide up */
                       : `w-96 ${isVisible ? 'translate-x-0' : 'translate-x-full'}` /* Desktop: slide left */
                   }`}
        role="complementary"
        aria-label="Node Inspector Panel"
      >
        {/* Header */}
        <div className="flex justify-between items-center p-4 border-b border-gray-700">
          <div><span className="sr-only">Node Inspector</span><p className="instrument-label">Attention lock</p><h3 className="mt-1 font-semibold text-lg" id="inspector-heading">{displayName}</h3></div>
          <button
            className="text-gray-400 hover:text-white transition-colors px-2 py-1 rounded hover:bg-gray-800"
            onClick={onClear}
            aria-label="Close inspector"
          >
            <span aria-hidden="true">✕</span>
          </button>
        </div>

        {/* Tab Navigation */}
        <div className="flex border-b border-gray-700" role="tablist" aria-label="Inspector sections">
          <button
            className={`flex-1 px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === "overview"
                ? "text-white border-b-2 border-blue-500 bg-gray-800/50"
                : "text-gray-400 hover:text-white hover:bg-gray-800/30"
            }`}
            onClick={() => setActiveTab("overview")}
            role="tab"
            aria-selected={activeTab === "overview"}
            aria-controls="overview-panel"
            id="overview-tab"
          >
            Overview
          </button>
          <button
            className={`flex-1 px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === "connections"
                ? "text-white border-b-2 border-blue-500 bg-gray-800/50"
                : "text-gray-400 hover:text-white hover:bg-gray-800/30"
            }`}
            onClick={() => setActiveTab("connections")}
            role="tab"
            aria-selected={activeTab === "connections"}
            aria-controls="connections-panel"
            id="connections-tab"
          >
            Connections ({neighbors.length})
          </button>
          <button
            className={`flex-1 px-4 py-3 text-sm font-medium transition-colors ${
              activeTab === "statistics"
                ? "text-white border-b-2 border-blue-500 bg-gray-800/50"
                : "text-gray-400 hover:text-white hover:bg-gray-800/30"
            }`}
            onClick={() => setActiveTab("statistics")}
            role="tab"
            aria-selected={activeTab === "statistics"}
            aria-controls="statistics-panel"
            id="statistics-tab"
          >
            Statistics
          </button>
        </div>

        {/* Content Area */}
        <div 
          className="flex-1 overflow-y-auto p-4 space-y-4"
          role="tabpanel"
          aria-labelledby={`${activeTab}-tab`}
          id={`${activeTab}-panel`}
        >
          {loading && (
            <div className="flex items-center justify-center py-8" role="status" aria-live="polite">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-white"></div>
            </div>
          )}

          {error && !nodeDetails && (
            <div className="bg-red-900/30 border border-red-700 rounded p-3 text-sm">
              <p className="text-red-200">{error}</p>
            </div>
          )}

          {!loading && (
            <>
              {/* Overview Tab */}
              {activeTab === "overview" && (
                <div className="space-y-3">
                  <div className="bg-gray-800/50 rounded-lg p-3 space-y-2">
                    {displayType && (
                      <div>
                        <span className="text-xs text-gray-400 uppercase tracking-wide">Type</span>
                        <p className="text-sm">
                          <span className="inline-block px-2 py-1 rounded text-xs font-medium bg-blue-900/50 text-blue-200">
                            {displayType}
                          </span>
                        </p>
                      </div>
                    )}

                    <div>
                      <span className="text-xs text-gray-400 uppercase tracking-wide">ID</span>
                      <p className="text-xs text-gray-300 font-mono break-all">{selected.id}</p>
                    </div>

                    {displayVal && (
                      <div>
                        <span className="text-xs text-gray-400 uppercase tracking-wide">Weight</span>
                        <p className="text-sm font-medium">{displayVal}</p>
                      </div>
                    )}

                    <div>
                      <span className="text-xs text-gray-400 uppercase tracking-wide">Connections</span>
                      <p className="text-sm font-medium">{degree}</p>
                    </div>
                  </div>

                  {/* Type-specific Stats */}
                  {nodeDetails?.stats && displayType === "subreddit" && (
                    <div className="bg-gray-800/50 rounded-lg p-3 space-y-2">
                      <h4 className="text-sm font-semibold text-gray-300 mb-2">Subreddit Info</h4>
                      
                      {nodeDetails.stats.subscribers !== undefined && (
                        <div>
                          <span className="text-xs text-gray-400 uppercase tracking-wide">Subscribers</span>
                          <p className="text-sm font-medium">{nodeDetails.stats.subscribers.toLocaleString()}</p>
                        </div>
                      )}

                      {nodeDetails.stats.title && (
                        <div>
                          <span className="text-xs text-gray-400 uppercase tracking-wide">Title</span>
                          <p className="text-sm">{nodeDetails.stats.title}</p>
                        </div>
                      )}

                      {nodeDetails.stats.description && (
                        <div>
                          <span className="text-xs text-gray-400 uppercase tracking-wide">Description</span>
                          <p className="text-xs text-gray-300 line-clamp-3">{nodeDetails.stats.description}</p>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* Connections Tab */}
              {activeTab === "connections" && (
                <div className="space-y-3">
                  {neighbors.length === 0 ? (
                    <p className="text-sm text-gray-400 text-center py-8">No connections found</p>
                  ) : (
                    <>
                      <div className="text-sm text-gray-400">
                        Showing top {neighbors.length} neighbors by connection strength
                      </div>
                      <VirtualList
                        items={neighbors}
                        itemHeight={64}
                        containerHeight={500}
                        className="pr-1"
                        itemKey={(n) => n.id}
                        renderItem={(n) => {
                          const neighbor = n as typeof neighbors[0];
                          const hasDegree = isNeighborInfo(neighbor);
                          return (
                            <button
                              className="w-full text-left p-3 rounded hover:bg-gray-800/70 transition-colors border border-gray-700/50 hover:border-gray-600"
                              onClick={() => onFocus(neighbor.id)}
                              title={`Navigate to ${neighbor.name || neighbor.id}`}
                            >
                              <div className="flex justify-between items-start">
                                <div className="flex-1 min-w-0">
                                  <p className="text-sm font-medium truncate">
                                    {neighbor.name || neighbor.id}
                                  </p>
                                  {neighbor.type && (
                                    <p className="text-xs text-gray-400 mt-1">
                                      <span className="inline-block px-1.5 py-0.5 rounded text-xs bg-gray-700/50">
                                        {neighbor.type}
                                      </span>
                                    </p>
                                  )}
                                </div>
                                {hasDegree && (
                                  <div className="ml-2 text-xs text-gray-400 flex-shrink-0">
                                    {(neighbor as NeighborInfo).degree} connections
                                  </div>
                                )}
                              </div>
                            </button>
                          );
                        }}
                      />
                    </>
                  )}
                </div>
              )}

              {/* Statistics Tab */}
              {activeTab === "statistics" && (
                <div className="space-y-3">
                  <div className="bg-gray-800/50 rounded-lg p-3 space-y-2">
                    <h4 className="text-sm font-semibold text-gray-300 mb-2">Connection Statistics</h4>
                    
                    <div>
                      <span className="text-xs text-gray-400 uppercase tracking-wide">Total Connections</span>
                      <p className="text-2xl font-bold">{degree}</p>
                    </div>

                    <div>
                      <span className="text-xs text-gray-400 uppercase tracking-wide">Top Neighbors</span>
                      <p className="text-sm">{Math.min(neighbors.length, 20)} shown</p>
                    </div>
                  </div>

                  {/* Connection breakdown by type */}
                  {neighbors.length > 0 && (
                    <div className="bg-gray-800/50 rounded-lg p-3 space-y-2">
                      <h4 className="text-sm font-semibold text-gray-300 mb-2">Connections by Type</h4>
                      {(() => {
                        const typeCounts = neighbors.reduce((acc, n) => {
                          const neighbor = n as typeof neighbors[0];
                          const type = neighbor.type || 'unknown';
                          acc[type] = (acc[type] || 0) + 1;
                          return acc;
                        }, {} as Record<string, number>);

                        return Object.entries(typeCounts).map(([type, count]) => (
                          <div key={type} className="flex justify-between items-center">
                            <span className="text-sm capitalize">{type}</span>
                            <span className="text-sm font-medium">{count}</span>
                          </div>
                        ));
                      })()}
                    </div>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
