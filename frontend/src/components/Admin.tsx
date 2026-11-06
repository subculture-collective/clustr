import { useEffect, useState } from "react";

interface JobStats {
  queued_count: number;
  running_count: number;
  failed_count: number;
  completed_count: number;
  total_count: number;
}

interface Job {
  id: number;
  subreddit_id: number;
  subreddit_name: string;
  status: string;
  retries: number | null;
  priority: number | null;
  last_attempt: string | null;
  enqueued_by: string | null;
  created_at: string | null;
  updated_at: string | null;
}

interface Settings {
  crawler_enabled: boolean;
  precalc_enabled: boolean;
  detailed_graph: boolean;
  crawler_rps: number;
  rate_limit_global: number;
  rate_limit_per_ip: number;
  layout_max_nodes: number;
  layout_iterations: number;
  posts_per_sub_in_graph: number;
  comments_per_post_in_graph: number;
  max_author_content_links: number;
  max_posts_per_sub: number;
}

interface AuditLogEntry {
  id: number;
  action: string;
  resource_type: string;
  resource_id: string | null;
  user_id: string;
  details: Record<string, unknown>;
  ip_address: string | null;
  created_at: string;
}

interface AdminProps {
  onViewMode: (mode: "3d" | "2d") => void;
}

function Admin({ onViewMode }: AdminProps) {
  const [token, setToken] = useState<string>("");
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [activeTab, setActiveTab] = useState<"jobs" | "settings" | "audit">("jobs");
  const [jobStats, setJobStats] = useState<JobStats | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selectedStatus, setSelectedStatus] = useState<string>("queued");
  const [settings, setSettings] = useState<Settings | null>(null);
  const [auditLog, setAuditLog] = useState<AuditLogEntry[]>([]);
  const [error, setError] = useState<string>("");
  const [editingPriority, setEditingPriority] = useState<{ [jobId: number]: number }>({});

  const apiUrl = import.meta.env.VITE_API_URL || "/api";

  const fetchWithAuth = async (url: string, options: RequestInit = {}) => {
    const headers = {
      ...options.headers,
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    };
    const response = await fetch(url, { ...options, headers });
    if (response.status === 401) {
      setIsAuthenticated(false);
      throw new Error("Unauthorized");
    }
    return response;
  };

  const handleLogin = async () => {
    if (!token.trim()) {
      setError("Please enter an admin token");
      return;
    }
    
    try {
      // Validate token by making an authenticated API call
      const response = await fetch(`${apiUrl}/admin/jobs/stats`, {
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
      });
      
      if (response.status === 200) {
        setIsAuthenticated(true);
        setError("");
      } else if (response.status === 401) {
        setError("Invalid admin token");
      } else {
        setError("Failed to authenticate. Please try again.");
      }
    } catch (err) {
      setError(`Authentication failed: ${err}`);
    }
  };

  const loadJobStats = async () => {
    try {
      const response = await fetchWithAuth(`${apiUrl}/admin/jobs/stats`);
      const data = await response.json();
      setJobStats(data);
    } catch (err) {
      setError(`Failed to load job stats: ${err}`);
    }
  };

  const loadJobs = async (status: string) => {
    try {
      const response = await fetchWithAuth(
        `${apiUrl}/admin/jobs?status=${status}&limit=50`
      );
      const data = await response.json();
      setJobs(data || []);
    } catch (err) {
      setError(`Failed to load jobs: ${err}`);
    }
  };

  const loadSettings = async () => {
    try {
      const response = await fetchWithAuth(`${apiUrl}/admin/settings`);
      const data = await response.json();
      setSettings(data);
    } catch (err) {
      setError(`Failed to load settings: ${err}`);
    }
  };

  const loadAuditLog = async () => {
    try {
      const response = await fetchWithAuth(
        `${apiUrl}/admin/audit-log?limit=100`
      );
      const data = await response.json();
      setAuditLog(data || []);
    } catch (err) {
      setError(`Failed to load audit log: ${err}`);
    }
  };

  const updateJobStatus = async (jobId: number, newStatus: string) => {
    try {
      await fetchWithAuth(`${apiUrl}/admin/jobs/${jobId}/status`, {
        method: "PUT",
        body: JSON.stringify({ status: newStatus }),
      });
      loadJobs(selectedStatus);
      loadJobStats();
    } catch (err) {
      setError(`Failed to update job status: ${err}`);
    }
  };

  const updateJobPriority = async (jobId: number, newPriority: number) => {
    try {
      await fetchWithAuth(`${apiUrl}/admin/jobs/${jobId}/priority`, {
        method: "PUT",
        body: JSON.stringify({ priority: newPriority }),
      });
      loadJobs(selectedStatus);
    } catch (err) {
      setError(`Failed to update job priority: ${err}`);
    }
  };

  const retryJob = async (jobId: number) => {
    try {
      await fetchWithAuth(`${apiUrl}/admin/jobs/${jobId}/retry`, {
        method: "POST",
      });
      loadJobs(selectedStatus);
      loadJobStats();
    } catch (err) {
      setError(`Failed to retry job: ${err}`);
    }
  };

  const updateSettings = async (updatedSettings: Partial<Settings>) => {
    try {
      await fetchWithAuth(`${apiUrl}/admin/settings`, {
        method: "PUT",
        body: JSON.stringify(updatedSettings),
      });
      loadSettings();
    } catch (err) {
      setError(`Failed to update settings: ${err}`);
    }
  };

  useEffect(() => {
    if (isAuthenticated && activeTab === "jobs") {
      loadJobStats();
      loadJobs(selectedStatus);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, activeTab, selectedStatus]);

  useEffect(() => {
    if (isAuthenticated && activeTab === "settings") {
      loadSettings();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, activeTab]);

  useEffect(() => {
    if (isAuthenticated && activeTab === "audit") {
      loadAuditLog();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, activeTab]);

  if (!isAuthenticated) {
    return (
      <div className="w-full h-screen bg-gray-900 flex items-center justify-center">
        <div className="bg-gray-800 p-8 rounded-lg shadow-lg max-w-md w-full">
          <h1 className="text-2xl font-bold text-white mb-6">Admin Login</h1>
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">
                Admin Token
              </label>
              <input
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                onKeyPress={(e) => {
                  if (e.key === "Enter") {
                    handleLogin();
                  }
                }}
                className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-sm text-white focus:outline-hidden focus:border-blue-500"
                placeholder="Enter admin token"
              />
            </div>
            <button
              onClick={handleLogin}
              className="w-full py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-sm font-medium transition-colors"
            >
              Login
            </button>
            {error && <p className="text-red-500 text-sm">{error}</p>}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="w-full h-screen bg-gray-900 text-white overflow-hidden flex flex-col">
      {/* Header */}
      <div className="bg-gray-800 border-b border-gray-700 p-4">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">Admin Control Panel</h1>
          <div className="flex gap-2">
            <button
              onClick={() => onViewMode("3d")}
              className="px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-sm transition-colors"
            >
              Back to Graph
            </button>
            <button
              onClick={() => setIsAuthenticated(false)}
              className="px-4 py-2 bg-red-600 hover:bg-red-700 rounded-sm transition-colors"
            >
              Logout
            </button>
          </div>
        </div>
        
        {/* Tabs */}
        <div className="flex gap-4 mt-4">
          <button
            onClick={() => setActiveTab("jobs")}
            className={`px-4 py-2 rounded-sm transition-colors ${
              activeTab === "jobs"
                ? "bg-blue-600 text-white"
                : "bg-gray-700 text-gray-300 hover:bg-gray-600"
            }`}
          >
            Job Queue
          </button>
          <button
            onClick={() => setActiveTab("settings")}
            className={`px-4 py-2 rounded-sm transition-colors ${
              activeTab === "settings"
                ? "bg-blue-600 text-white"
                : "bg-gray-700 text-gray-300 hover:bg-gray-600"
            }`}
          >
            Settings
          </button>
          <button
            onClick={() => setActiveTab("audit")}
            className={`px-4 py-2 rounded-sm transition-colors ${
              activeTab === "audit"
                ? "bg-blue-600 text-white"
                : "bg-gray-700 text-gray-300 hover:bg-gray-600"
            }`}
          >
            Audit Log
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-6">
        {error && (
          <div className="bg-red-900 border border-red-700 text-red-200 px-4 py-3 rounded-sm mb-4">
            {error}
            <button
              onClick={() => setError("")}
              className="float-right text-red-300 hover:text-white"
            >
              ×
            </button>
          </div>
        )}

        {activeTab === "jobs" && (
          <div>
            {/* Job Stats */}
            {jobStats && (
              <div className="grid grid-cols-5 gap-4 mb-6">
                <div className="bg-gray-800 p-4 rounded-sm">
                  <div className="text-gray-400 text-sm">Queued</div>
                  <div className="text-2xl font-bold text-yellow-500">
                    {jobStats.queued_count}
                  </div>
                </div>
                <div className="bg-gray-800 p-4 rounded-sm">
                  <div className="text-gray-400 text-sm">Running</div>
                  <div className="text-2xl font-bold text-blue-500">
                    {jobStats.running_count}
                  </div>
                </div>
                <div className="bg-gray-800 p-4 rounded-sm">
                  <div className="text-gray-400 text-sm">Failed</div>
                  <div className="text-2xl font-bold text-red-500">
                    {jobStats.failed_count}
                  </div>
                </div>
                <div className="bg-gray-800 p-4 rounded-sm">
                  <div className="text-gray-400 text-sm">Completed</div>
                  <div className="text-2xl font-bold text-green-500">
                    {jobStats.completed_count}
                  </div>
                </div>
                <div className="bg-gray-800 p-4 rounded-sm">
                  <div className="text-gray-400 text-sm">Total</div>
                  <div className="text-2xl font-bold">{jobStats.total_count}</div>
                </div>
              </div>
            )}

            {/* Status Filter */}
            <div className="mb-4">
              <label className="text-sm text-gray-400 mr-2">Filter by status:</label>
              <select
                value={selectedStatus}
                onChange={(e) => setSelectedStatus(e.target.value)}
                className="bg-gray-800 border border-gray-700 rounded-sm px-3 py-1 text-white focus:outline-hidden focus:border-blue-500"
              >
                <option value="queued">Queued</option>
                <option value="crawling">Running</option>
                <option value="failed">Failed</option>
                <option value="success">Completed</option>
              </select>
            </div>

            {/* Jobs Table */}
            <div className="bg-gray-800 rounded-sm overflow-hidden">
              <table className="w-full">
                <thead className="bg-gray-700">
                  <tr>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                      ID
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                      Subreddit
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                      Status
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                      Priority
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                      Retries
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-700">
                  {jobs.map((job) => (
                    <tr key={job.id} className="hover:bg-gray-750">
                      <td className="px-4 py-3 text-sm">{job.id}</td>
                      <td className="px-4 py-3 text-sm font-medium">
                        {job.subreddit_name}
                      </td>
                      <td className="px-4 py-3 text-sm">
                        <span
                          className={`px-2 py-1 rounded-sm text-xs ${
                            job.status === "queued"
                              ? "bg-yellow-900 text-yellow-200"
                              : job.status === "crawling"
                              ? "bg-blue-900 text-blue-200"
                              : job.status === "failed"
                              ? "bg-red-900 text-red-200"
                              : "bg-green-900 text-green-200"
                          }`}
                        >
                          {job.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-sm">
                        <input
                          type="number"
                          value={editingPriority[job.id] ?? job.priority ?? 0}
                          onChange={(e) =>
                            setEditingPriority({
                              ...editingPriority,
                              [job.id]: parseInt(e.target.value) || 0,
                            })
                          }
                          onBlur={(e) => {
                            const newPriority = parseInt(e.target.value) || 0;
                            if (newPriority !== (job.priority || 0)) {
                              updateJobPriority(job.id, newPriority);
                            }
                          }}
                          className="w-20 bg-gray-700 border border-gray-600 rounded-sm px-2 py-1 text-sm"
                        />
                      </td>
                      <td className="px-4 py-3 text-sm">{job.retries || 0}</td>
                      <td className="px-4 py-3 text-sm space-x-2">
                        {job.status === "failed" && (
                          <button
                            onClick={() => retryJob(job.id)}
                            className="px-3 py-1 bg-blue-600 hover:bg-blue-700 rounded-sm text-xs transition-colors"
                          >
                            Retry
                          </button>
                        )}
                        {job.status === "queued" && (
                          <button
                            onClick={() => updateJobStatus(job.id, "failed")}
                            className="px-3 py-1 bg-red-600 hover:bg-red-700 rounded-sm text-xs transition-colors"
                          >
                            Cancel
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {jobs.length === 0 && (
                <div className="p-8 text-center text-gray-400">
                  No jobs found with status: {selectedStatus}
                </div>
              )}
            </div>
          </div>
        )}

        {activeTab === "settings" && settings && (
          <div className="space-y-6">
            <div className="bg-gray-800 p-6 rounded-sm">
              <h2 className="text-xl font-bold mb-4">Service Controls</h2>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium">Crawler Enabled</label>
                  <input
                    type="checkbox"
                    checked={settings.crawler_enabled}
                    onChange={(e) =>
                      updateSettings({ crawler_enabled: e.target.checked })
                    }
                    className="w-5 h-5"
                  />
                </div>
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium">
                    Precalculation Enabled
                  </label>
                  <input
                    type="checkbox"
                    checked={settings.precalc_enabled}
                    onChange={(e) =>
                      updateSettings({ precalc_enabled: e.target.checked })
                    }
                    className="w-5 h-5"
                  />
                </div>
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium">Detailed Graph</label>
                  <input
                    type="checkbox"
                    checked={settings.detailed_graph}
                    onChange={(e) =>
                      updateSettings({ detailed_graph: e.target.checked })
                    }
                    className="w-5 h-5"
                  />
                </div>
              </div>
            </div>

            <div className="bg-gray-800 p-6 rounded-sm">
              <h2 className="text-xl font-bold mb-4">Rate Limits</h2>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Crawler RPS
                  </label>
                  <input
                    type="number"
                    value={settings.crawler_rps}
                    onChange={(e) =>
                      updateSettings({
                        crawler_rps: parseFloat(e.target.value),
                      })
                    }
                    step="0.1"
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Global Rate Limit
                  </label>
                  <input
                    type="number"
                    value={settings.rate_limit_global}
                    onChange={(e) =>
                      updateSettings({
                        rate_limit_global: parseFloat(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Per-IP Rate Limit
                  </label>
                  <input
                    type="number"
                    value={settings.rate_limit_per_ip}
                    onChange={(e) =>
                      updateSettings({
                        rate_limit_per_ip: parseFloat(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
              </div>
            </div>

            <div className="bg-gray-800 p-6 rounded-sm">
              <h2 className="text-xl font-bold mb-4">Layout Settings</h2>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Max Nodes
                  </label>
                  <input
                    type="number"
                    value={settings.layout_max_nodes}
                    onChange={(e) =>
                      updateSettings({
                        layout_max_nodes: parseInt(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Iterations
                  </label>
                  <input
                    type="number"
                    value={settings.layout_iterations}
                    onChange={(e) =>
                      updateSettings({
                        layout_iterations: parseInt(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
              </div>
            </div>

            <div className="bg-gray-800 p-6 rounded-sm">
              <h2 className="text-xl font-bold mb-4">Graph Content Limits</h2>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Posts per Subreddit
                  </label>
                  <input
                    type="number"
                    value={settings.posts_per_sub_in_graph}
                    onChange={(e) =>
                      updateSettings({
                        posts_per_sub_in_graph: parseInt(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Comments per Post
                  </label>
                  <input
                    type="number"
                    value={settings.comments_per_post_in_graph}
                    onChange={(e) =>
                      updateSettings({
                        comments_per_post_in_graph: parseInt(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Max Author Links
                  </label>
                  <input
                    type="number"
                    value={settings.max_author_content_links}
                    onChange={(e) =>
                      updateSettings({
                        max_author_content_links: parseInt(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-2">
                    Max Posts per Sub
                  </label>
                  <input
                    type="number"
                    value={settings.max_posts_per_sub}
                    onChange={(e) =>
                      updateSettings({
                        max_posts_per_sub: parseInt(e.target.value),
                      })
                    }
                    className="w-full bg-gray-700 border border-gray-600 rounded-sm px-3 py-2"
                  />
                </div>
              </div>
            </div>
          </div>
        )}

        {activeTab === "audit" && (
          <div className="bg-gray-800 rounded-sm overflow-hidden">
            <table className="w-full">
              <thead className="bg-gray-700">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                    Timestamp
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                    Action
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                    Resource
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                    User
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-300 uppercase">
                    Details
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-700">
                {auditLog.map((entry) => (
                  <tr key={entry.id} className="hover:bg-gray-750">
                    <td className="px-4 py-3 text-sm">
                      {new Date(entry.created_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-3 text-sm font-medium">
                      {entry.action}
                    </td>
                    <td className="px-4 py-3 text-sm">
                      {entry.resource_type}
                      {entry.resource_id && ` #${entry.resource_id}`}
                    </td>
                    <td className="px-4 py-3 text-sm">{entry.user_id}</td>
                    <td className="px-4 py-3 text-sm text-gray-400">
                      {JSON.stringify(entry.details)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {auditLog.length === 0 && (
              <div className="p-8 text-center text-gray-400">
                No audit log entries found
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

export default Admin;
