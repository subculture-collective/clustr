/**
 * GraphErrorFallback - Specialized error UI for graph rendering failures
 * Provides fallback options when 3D graph fails
 */

type GraphErrorFallbackProps = {
  error: Error;
  onRetry: () => void;
  onFallbackTo2D?: () => void;
  mode: '3d' | '2d';
  webglSupported?: boolean;
};

const GraphErrorFallback = ({
  error,
  onRetry,
  onFallbackTo2D,
  mode,
  webglSupported = true,
}: GraphErrorFallbackProps) => {
  const isWebGLError = !webglSupported || error.message.toLowerCase().includes('webgl');
  const is3DMode = mode === '3d';

  return (
    <div className="flex h-screen w-full items-center justify-center bg-[#030506]">
      <div className="instrument-panel mx-4 max-w-2xl rounded-2xl border-red-500/30 px-8 py-6">
        <div className="flex items-start gap-4">
          {/* Error icon */}
          <div className="flex-shrink-0">
            <svg
              className="w-8 h-8 text-red-500"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
              />
            </svg>
          </div>

          {/* Error content */}
          <div className="flex-1">
            <h2 className="text-red-100 text-xl font-semibold mb-2">
              {isWebGLError
                ? 'WebGL Not Supported'
                : `Graph Rendering Failed (${mode.toUpperCase()})`}
            </h2>
            
            {isWebGLError ? (
              <div className="space-y-3">
                <p className="text-red-200 text-sm">
                  Your browser doesn't support WebGL, which is required for 3D visualization.
                </p>
                <p className="text-red-200 text-sm">
                  Please try using a modern browser like Chrome, Firefox, Safari, or Edge.
                </p>
                {is3DMode && onFallbackTo2D && (
                  <p className="text-red-200 text-sm">
                    Alternatively, you can switch to the 2D view which doesn't require WebGL.
                  </p>
                )}
              </div>
            ) : (
              <p className="text-red-200 text-sm mb-4">
                The {mode.toUpperCase()} visualization encountered an error and couldn't render.
              </p>
            )}

            {/* Error details (collapsed by default) */}
            <details className="my-4">
              <summary className="text-red-300 text-sm cursor-pointer hover:text-red-200">
                Show technical details
              </summary>
              <pre className="mt-2 text-xs text-red-200 bg-black/30 rounded p-3 overflow-auto max-h-40">
                {error.toString()}
                {error.stack && `\n\n${error.stack}`}
              </pre>
            </details>

            {/* Action buttons */}
            <div className="flex flex-wrap gap-3">
              <button
                onClick={onRetry}
                className="instrument-button rounded-full border-red-400/30 px-4 font-medium text-red-100"
              >
                Try Again
              </button>
              
              {is3DMode && onFallbackTo2D && (
                <button
                  onClick={onFallbackTo2D}
                  className="instrument-button rounded-full border-white/15 px-4 font-medium text-white"
                >
                  Switch to 2D View
                </button>
              )}
              
              <button
                onClick={() => window.location.reload()}
                className="instrument-button rounded-full border-white/15 px-4 font-medium text-white"
              >
                Reload Page
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default GraphErrorFallback;
