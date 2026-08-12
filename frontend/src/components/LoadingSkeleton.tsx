/**
 * LoadingSkeleton - Professional loading state with skeleton UI
 * Shows animated placeholders during initial graph load
 */

const LoadingSkeleton = () => {
  return (
    <div className="relative h-screen w-full overflow-hidden bg-[#030506]" role="status" aria-live="polite" aria-label="Calculating visible universe">
      {/* Animated background gradient */}
      <div className="absolute inset-0 opacity-40" style={{ backgroundImage: 'linear-gradient(rgba(255,255,255,.025) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.025) 1px, transparent 1px)', backgroundSize: '48px 48px' }} />
      
      {/* Mock graph container */}
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="relative w-full h-full max-w-7xl max-h-[600px] mx-auto">
          {/* Skeleton nodes - pulsing circles */}
          <div className="absolute top-1/4 left-1/4 w-16 h-16 rounded-full bg-gradient-to-br from-green-500/30 to-green-600/10 animate-pulse" />
          <div className="absolute top-1/3 right-1/3 w-12 h-12 rounded-full bg-gradient-to-br from-blue-500/30 to-blue-600/10 animate-pulse delay-75" />
          <div className="absolute bottom-1/3 left-1/3 w-20 h-20 rounded-full bg-gradient-to-br from-green-500/30 to-green-600/10 animate-pulse delay-150" />
          <div className="absolute bottom-1/4 right-1/4 w-14 h-14 rounded-full bg-gradient-to-br from-blue-500/30 to-blue-600/10 animate-pulse delay-100" />
          <div className="absolute top-1/2 left-1/2 w-24 h-24 rounded-full bg-gradient-to-br from-green-500/30 to-green-600/10 animate-pulse delay-200" />
          
          {/* Skeleton links - faint lines */}
          <svg className="absolute inset-0 w-full h-full opacity-20">
            <line x1="25%" y1="25%" x2="33%" y2="33%" stroke="#4ade80" strokeWidth="2" className="animate-pulse" />
            <line x1="33%" y1="33%" x2="50%" y2="50%" stroke="#60a5fa" strokeWidth="2" className="animate-pulse delay-75" />
            <line x1="50%" y1="50%" x2="66%" y2="33%" stroke="#4ade80" strokeWidth="2" className="animate-pulse delay-150" />
            <line x1="50%" y1="50%" x2="33%" y2="66%" stroke="#60a5fa" strokeWidth="2" className="animate-pulse delay-100" />
          </svg>
        </div>
      </div>

      {/* Loading message */}
      <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
        <div className="instrument-panel max-w-md rounded-2xl px-8 py-6">
          {/* Spinner */}
          <div className="flex justify-center mb-4">
            <div className="h-12 w-12 animate-spin rounded-full border border-[#b6ff62]/20 border-t-[#b6ff62]" />
          </div>
          
          {/* Message */}
          <h2 className="text-white text-xl font-semibold text-center mb-2">
            Locating your universe
          </h2>
          <span className="sr-only">Loading Graph</span>
          <p className="text-gray-400 text-sm text-center mb-4">
            Pinning the current revision and preparing visible landmarks.
          </p>
          <span className="sr-only">Preparing network visualization...</span>
          
          {/* Pulsing dots */}
          <div className="flex justify-center gap-1 mt-2">
            <div className="h-1.5 w-1.5 animate-bounce rounded-full bg-[#b6ff62]" style={{ animationDelay: '0ms' }} />
            <div className="h-1.5 w-1.5 animate-bounce rounded-full bg-[#b6ff62]" style={{ animationDelay: '150ms' }} />
            <div className="h-1.5 w-1.5 animate-bounce rounded-full bg-[#b6ff62]" style={{ animationDelay: '300ms' }} />
          </div>
        </div>
      </div>
    </div>
  );
};

export default LoadingSkeleton;
