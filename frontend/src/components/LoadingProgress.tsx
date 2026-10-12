interface LoadingProgressProps {
  nodesLoaded: number;
  linksLoaded: number;
  totalNodes?: number;
  totalLinks?: number;
  percentComplete: number;
}

export default function LoadingProgress({
  nodesLoaded,
  linksLoaded,
  totalNodes,
  totalLinks,
  percentComplete,
}: LoadingProgressProps) {
  return (
    <div className="absolute top-4 left-1/2 transform -translate-x-1/2 z-50">
      <div className="bg-gray-900/90 backdrop-blur-xs text-white px-6 py-3 rounded-lg shadow-lg border border-gray-700">
        <div className="flex items-center gap-4">
          {/* Progress bar */}
          <div className="flex-1 min-w-[200px]">
            <div className="flex items-center justify-between mb-1">
              <span className="text-sm font-medium">Loading Graph</span>
              <span className="text-sm text-gray-400">{percentComplete}%</span>
            </div>
            <div className="w-full bg-gray-700 rounded-full h-2 overflow-hidden">
              <div
                className="bg-blue-500 h-full transition-all duration-300 ease-out"
                style={{ width: `${percentComplete}%` }}
              />
            </div>
          </div>
          
          {/* Stats */}
          <div className="flex gap-4 text-sm">
            <div>
              <span className="text-gray-400">Nodes:</span>{' '}
              <span className="font-mono font-medium">
                {nodesLoaded.toLocaleString()}
                {totalNodes && (
                  <span className="text-gray-400">
                    {' / '}
                    {totalNodes.toLocaleString()}
                  </span>
                )}
              </span>
            </div>
            <div>
              <span className="text-gray-400">Links:</span>{' '}
              <span className="font-mono font-medium">
                {linksLoaded.toLocaleString()}
                {totalLinks && (
                  <span className="text-gray-400">
                    {' / '}
                    {totalLinks.toLocaleString()}
                  </span>
                )}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
