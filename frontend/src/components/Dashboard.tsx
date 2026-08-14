import { useEffect, useState } from "react";
import type { GraphData } from "../types/graph";
import VirtualList from "./VirtualList";

interface Stats {
  totalNodes: number;
  totalLinks: number;
  sampledNodes: number;
  sampledLinks: number;
  nodesByType: Record<string, number>;
  avgDegree: number;
  maxDegree: number;
  topNodes: Array<{
    id: string;
    name: string;
    type?: string;
    degree: number;
    val?: number;
  }>;
  topSubreddits: Array<{
    id: string;
    name: string;
    subscribers?: number;
    activeUsers: number;
  }>;
  mostActiveUsers: Array<{
    id: string;
    name: string;
    posts: number;
    comments: number;
  }>;
}

interface GraphManifest {
  node_count: number;
  link_count: number;
}

type DashboardProps = {
  onViewMode?: (mode: "3d" | "2d") => void;
  onFocusNode?: (id: string) => void;
};

export default function Dashboard({ onViewMode, onFocusNode }: DashboardProps) {
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadStats();
  }, []);

  const loadStats = async () => {
    setLoading(true);
    setError(null);
    try {
      const base = (import.meta.env?.VITE_API_URL || "/api").replace(/\/$/, "");
      const [graphResponse, manifestResponse] = await Promise.all([
        fetch(`${base}/graph?max_nodes=50000&max_links=100000`),
        fetch(`${base}/graph/manifest`),
      ]);
      if (!graphResponse.ok) throw new Error(`HTTP ${graphResponse.status}`);
      if (!manifestResponse.ok) throw new Error(`HTTP ${manifestResponse.status}`);
      const data = (await graphResponse.json()) as GraphData;
      const manifest = (await manifestResponse.json()) as GraphManifest;

      // Calculate stats
      const degreeMap = new Map<string, number>();
      for (const link of data.links) {
        degreeMap.set(link.source, (degreeMap.get(link.source) || 0) + 1);
        degreeMap.set(link.target, (degreeMap.get(link.target) || 0) + 1);
      }

      const nodesByType: Record<string, number> = {};
      for (const node of data.nodes) {
        const type = node.type || "unknown";
        nodesByType[type] = (nodesByType[type] || 0) + 1;
      }

      const degrees = Array.from(degreeMap.values());
      const avgDegree =
        degrees.length > 0
          ? degrees.reduce((a, b) => a + b, 0) / degrees.length
          : 0;
      const maxDegree = degrees.length > 0 ? Math.max(...degrees) : 0;

      // Top nodes by degree
      const nodesWithDegree = data.nodes.map((n) => ({
        ...n,
        degree: degreeMap.get(n.id) || 0,
      }));
      const topNodes = nodesWithDegree
        .sort((a, b) => b.degree - a.degree)
        .slice(0, 20)
        .map((n) => ({
          id: n.id,
          name: n.name,
          type: n.type,
          degree: n.degree,
          val: n.val,
        }));

      // Top subreddits by subscribers and activity
      const subreddits = data.nodes.filter((n) => n.type === "subreddit");
      const subredditActivity = new Map<string, { users: Set<string> }>();

      for (const link of data.links) {
        const s = String(link.source);
        const t = String(link.target);
        if (s.startsWith("user_") && t.startsWith("subreddit_")) {
          if (!subredditActivity.has(t)) {
            subredditActivity.set(t, { users: new Set() });
          }
          subredditActivity.get(t)!.users.add(s);
        } else if (t.startsWith("user_") && s.startsWith("subreddit_")) {
          if (!subredditActivity.has(s)) {
            subredditActivity.set(s, { users: new Set() });
          }
          subredditActivity.get(s)!.users.add(t);
        }
      }

      const topSubreddits = subreddits
        .map((s) => ({
          id: s.id,
          name: s.name,
          subscribers: typeof s.val === "number" ? s.val : undefined,
          activeUsers: subredditActivity.get(s.id)?.users.size || 0,
        }))
        .sort((a, b) => (b.subscribers || 0) - (a.subscribers || 0))
        .slice(0, 15);

      // Most active users by posts and comments
      const userActivity = new Map<
        string,
        { posts: number; comments: number; name: string }
      >();

      for (const link of data.links) {
        const s = String(link.source);
        const t = String(link.target);

        if (s.startsWith("user_")) {
          const user = data.nodes.find((n) => n.id === s);
          if (!userActivity.has(s)) {
            userActivity.set(s, {
              posts: 0,
              comments: 0,
              name: user?.name || s,
            });
          }
          if (t.startsWith("post_")) {
            userActivity.get(s)!.posts++;
          } else if (t.startsWith("comment_")) {
            userActivity.get(s)!.comments++;
          }
        }
      }

      const mostActiveUsers = Array.from(userActivity.entries())
        .map(([id, data]) => ({
          id,
          name: data.name,
          posts: data.posts,
          comments: data.comments,
        }))
        .sort((a, b) => b.posts + b.comments - (a.posts + a.comments))
        .slice(0, 15);

      setStats({
        totalNodes: manifest.node_count,
        totalLinks: manifest.link_count,
        sampledNodes: data.nodes.length,
        sampledLinks: data.links.length,
        nodesByType,
        avgDegree,
        maxDegree,
        topNodes,
        topSubreddits,
        mostActiveUsers,
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="w-full h-screen bg-gray-900 text-white flex items-center justify-center">
        <div className="text-xl">Loading statistics...</div>
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

  if (!stats) {
    return null;
  }

  const formatNumber = (n: number) => {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
    if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
    return n.toString();
  };

  return (
    <div className="w-full h-screen bg-gray-900 text-white overflow-auto p-6">
      <div className="max-w-7xl mx-auto">
        <div className="flex justify-between items-center mb-8">
          <div><p className="instrument-label mb-3">Data view / Current world</p><h1 className="text-3xl font-bold">Universe telemetry</h1><p className="mt-2 text-sm text-gray-400">A numerical reading of the same relationships drawn in the spatial explorer.</p></div>
          <div className="flex gap-2">
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
            <div className="text-gray-400 text-sm mb-2">Catalog Entities</div>
            <div className="text-3xl font-bold" title={stats.totalNodes.toLocaleString()}>
              {formatNumber(stats.totalNodes)}
            </div>
            <div className="mt-2 text-xs text-gray-500">
              {formatNumber(stats.sampledNodes)} sampled for analysis
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">Catalog Links</div>
            <div className="text-3xl font-bold" title={stats.totalLinks.toLocaleString()}>
              {formatNumber(stats.totalLinks)}
            </div>
            <div className="mt-2 text-xs text-gray-500">
              {formatNumber(stats.sampledLinks)} sampled for analysis
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">Sample Average Degree</div>
            <div className="text-3xl font-bold">
              {stats.avgDegree.toFixed(1)}
            </div>
          </div>
          <div className="bg-gray-800 rounded-lg p-6">
            <div className="text-gray-400 text-sm mb-2">Sample Max Degree</div>
            <div className="text-3xl font-bold">{stats.maxDegree}</div>
          </div>
        </div>

        {/* Nodes by Type */}
        <div className="bg-gray-800 rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4">Sampled Nodes by Type</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            {Object.entries(stats.nodesByType).map(([type, count]) => {
              const colors: Record<string, string> = {
                subreddit: "bg-green-500",
                user: "bg-blue-500",
                post: "bg-orange-500",
                comment: "bg-red-500",
              };
              return (
                <div key={type} className="flex items-center gap-3">
                  <div
                    className={`w-4 h-4 rounded ${
                      colors[type] || "bg-purple-500"
                    }`}
                  />
                  <div>
                    <div className="text-sm text-gray-400 capitalize">
                      {type}
                    </div>
                    <div className="text-xl font-semibold">
                      {formatNumber(count)}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          {/* Top Nodes */}
          <div className="bg-gray-800 rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">
              Top Nodes by Connections
            </h2>
            <VirtualList
              items={stats.topNodes}
              itemHeight={48}
              containerHeight={480}
              className="space-y-2"
              itemKey={(node) => node.id}
              renderItem={(node, i) => (
                <div
                  className="flex items-center justify-between p-2 bg-gray-700 rounded hover:bg-gray-600 cursor-pointer"
                  onClick={() => {
                    onFocusNode?.(node.name || node.id);
                    onViewMode?.("3d");
                  }}
                >
                  <div className="flex items-center gap-3">
                    <div className="text-gray-400 w-6">{i + 1}</div>
                    <div>
                      <div className="font-medium">{node.name}</div>
                      <div className="text-xs text-gray-400 capitalize">
                        {node.type}
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="font-semibold">{node.degree}</div>
                    <div className="text-xs text-gray-400">connections</div>
                  </div>
                </div>
              )}
            />
          </div>

          {/* Top Subreddits */}
          <div className="bg-gray-800 rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">Top Subreddits</h2>
            <VirtualList
              items={stats.topSubreddits}
              itemHeight={48}
              containerHeight={480}
              className="space-y-2"
              itemKey={(sub) => sub.id}
              renderItem={(sub, i) => (
                <div
                  className="flex items-center justify-between p-2 bg-gray-700 rounded hover:bg-gray-600 cursor-pointer"
                  onClick={() => {
                    onFocusNode?.(sub.name || sub.id);
                    onViewMode?.("3d");
                  }}
                >
                  <div className="flex items-center gap-3">
                    <div className="text-gray-400 w-6">{i + 1}</div>
                    <div>
                      <div className="font-medium">{sub.name}</div>
                      <div className="text-xs text-gray-400">
                        {sub.activeUsers} active users
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    {sub.subscribers !== undefined && (
                      <>
                        <div className="font-semibold">
                          {formatNumber(sub.subscribers)}
                        </div>
                        <div className="text-xs text-gray-400">subscribers</div>
                      </>
                    )}
                  </div>
                </div>
              )}
            />
          </div>

          {/* Most Active Users */}
          <div className="bg-gray-800 rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">Most Active Users</h2>
            <VirtualList
              items={stats.mostActiveUsers}
              itemHeight={48}
              containerHeight={480}
              className="space-y-2"
              itemKey={(user) => user.id}
              renderItem={(user, i) => (
                <div
                  className="flex items-center justify-between p-2 bg-gray-700 rounded hover:bg-gray-600 cursor-pointer"
                  onClick={() => {
                    onFocusNode?.(user.name || user.id);
                    onViewMode?.("3d");
                  }}
                >
                  <div className="flex items-center gap-3">
                    <div className="text-gray-400 w-6">{i + 1}</div>
                    <div className="font-medium">{user.name}</div>
                  </div>
                  <div className="text-right">
                    <div className="font-semibold">
                      {user.posts + user.comments}
                    </div>
                    <div className="text-xs text-gray-400">
                      {user.posts}p / {user.comments}c
                    </div>
                  </div>
                </div>
              )}
            />
          </div>

          {/* Graph Density */}
          <div className="bg-gray-800 rounded-lg p-6">
            <h2 className="text-xl font-semibold mb-4">Graph Metrics</h2>
            <div className="space-y-4">
              <div>
                <div className="text-sm text-gray-400 mb-1">Graph Density</div>
                <div className="text-2xl font-bold">
                  {stats.totalNodes > 1
                    ? (
                        (2 * stats.totalLinks) /
                        (stats.totalNodes * (stats.totalNodes - 1))
                      ).toFixed(6)
                    : "0"}
                </div>
                <div className="text-xs text-gray-400 mt-1">
                  (ratio of actual to possible edges)
                </div>
              </div>
              <div>
                <div className="text-sm text-gray-400 mb-1">
                  Average Clustering
                </div>
                <div className="text-2xl font-bold">
                  {stats.avgDegree > 0
                    ? (stats.avgDegree / stats.maxDegree).toFixed(3)
                    : "0"}
                </div>
                <div className="text-xs text-gray-400 mt-1">
                  (avg degree / max degree ratio)
                </div>
              </div>
              <div>
                <div className="text-sm text-gray-400 mb-1">Nodes per Type</div>
                <div className="space-y-1">
                  {Object.entries(stats.nodesByType).map(([type, count]) => (
                    <div key={type} className="flex justify-between text-sm">
                      <span className="capitalize">{type}:</span>
                      <span className="font-semibold">
                        {((count / stats.totalNodes) * 100).toFixed(1)}%
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="mt-8 text-center">
          <button
            onClick={loadStats}
            className="px-6 py-2 bg-gray-700 hover:bg-gray-600 rounded"
          >
            Refresh Statistics
          </button>
        </div>
      </div>
    </div>
  );
}
