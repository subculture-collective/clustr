import Admin from "./components/Admin";
import Sidebar from "./components/Sidebar.tsx";
import Communities from "./components/Communities";
import Dashboard from "./components/Dashboard";
import Graph2D from "./components/Graph2D";
import Graph3D from "./components/Graph3D.tsx";
import Inspector from "./components/Inspector.tsx";
import Legend from "./components/Legend.tsx";
import ShareButton from "./components/ShareButton.tsx";
import SearchBar, { type SearchBarHandle } from "./components/SearchBar.tsx";
import ErrorBoundary from "./components/ErrorBoundary.tsx";
import GraphErrorFallback from "./components/GraphErrorFallback.tsx";
import KeyboardShortcutsHelp from "./components/KeyboardShortcutsHelp.tsx";
import FieldGuide from "./components/FieldGuide.tsx";
import type { TypeFilters } from "./types/ui";
import type { CommunityResult } from "./utils/communityDetection";
import { readStateFromURL, writeStateToURL, type AppState } from "./utils/urlState";
import { useEffect, useState, useCallback, useRef } from "react";
import { detectWebGLSupport } from "./utils/webglDetect";
import { useKeyboardShortcuts } from "./hooks/useKeyboardShortcuts";

function readableSelection(id: string): string {
  const withoutPrefix = id.replace(/^(subreddit|user|post|comment)[_:]/i, "").replace(/^c:/i, "");
  const label = withoutPrefix.replace(/[_:.-]+/g, " ").trim();
  return label ? label.replace(/\b\w/g, character => character.toUpperCase()) : id;
}

function App() {
  // Initialize state from URL if available
  const urlState = readStateFromURL();

  const [filters, setFilters] = useState<TypeFilters>(() => {
    if (urlState.filters) return urlState.filters;
    return {
      subreddit: true,
      user: true,
      post: false,
      comment: false,
    };
  });
  
  const [minDegree, setMinDegree] = useState<number | undefined>(urlState.minDegree);
  const [maxDegree, setMaxDegree] = useState<number | undefined>(urlState.maxDegree);
  
  const [linkOpacity, setLinkOpacity] = useState(0.14);
  const [nodeRelSize, setNodeRelSize] = useState(5);
  const [physics, setPhysics] = useState<{
    chargeStrength: number;
    linkDistance: number;
    velocityDecay: number;
    cooldownTicks: number;
    collisionRadius: number;
    autoTune?: boolean;
  }>({
    chargeStrength: -220,
    linkDistance: 120,
    velocityDecay: 0.88,
    cooldownTicks: 80,
    collisionRadius: 3,
    autoTune: true, // Enable auto-tune by default for stability
  });
  const [focusNodeId, setFocusNodeId] = useState<string | undefined>();
  const [showLabels, setShowLabels] = useState(true);
  const [selectedId, setSelectedId] = useState<string | undefined>();
  const [subredditSize, setSubredditSize] = useState<
    "subscribers" | "activeUsers" | "contentActivity" | "interSubLinks"
  >("subscribers");
  const [viewMode, setViewMode] = useState<
    "3d" | "2d" | "dashboard" | "communities" | "admin"
  >(() => {
    // Prefer URL state over localStorage
    if (urlState.viewMode) return urlState.viewMode;
    const saved =
      typeof localStorage !== "undefined"
        ? localStorage.getItem("viewMode")
        : null;
    if (
      saved === "2d" ||
      saved === "3d" ||
      saved === "dashboard" ||
      saved === "communities" ||
      saved === "admin"
    ) {
      return saved;
    }
    return "3d";
  });
  const [experienceMode, setExperienceMode] = useState<"explore" | "analyst">(() => {
    try {
      return localStorage.getItem("experienceMode") === "analyst" ? "analyst" : "explore";
    } catch {
      return "explore";
    }
  });
  const [communityResult, setCommunityResult] =
    useState<CommunityResult | null>(null);
  const [useCommunityColors, setUseCommunityColors] = useState(() => {
    if (urlState.useCommunityColors !== undefined) return urlState.useCommunityColors;
    return false;
  });
  const [usePrecomputedLayout, setUsePrecomputedLayout] = useState<boolean>(
    () => {
      if (urlState.usePrecomputedLayout !== undefined) return urlState.usePrecomputedLayout;
      try {
        const saved = localStorage.getItem("usePrecomputedLayout");
        if (saved === "true" || saved === "false") return saved === "true";
      } catch {
        /* ignore */
      }
      return true; // default: on
    }
  );
  
  const [sizeAttenuation, setSizeAttenuation] = useState<boolean>(() => {
    if (urlState.sizeAttenuation !== undefined) return urlState.sizeAttenuation;
    return true; // default: enabled for better depth perception
  });
  
  const [enableAdaptiveLOD, setEnableAdaptiveLOD] = useState<boolean>(() => {
    if (urlState.enableAdaptiveLOD !== undefined) return urlState.enableAdaptiveLOD;
    try {
      const saved = localStorage.getItem("enableAdaptiveLOD");
      if (saved === "true" || saved === "false") return saved === "true";
    } catch {
      /* ignore */
    }
    return true; // default: enabled for better performance
  });
  
  const [currentLODTier, setCurrentLODTier] = useState<number>(3); // Start at HIGH tier
  
  const [camera3dRef, setCamera3dRef] = useState<{ x: number; y: number; z: number } | undefined>(urlState.camera3d);
  const [camera2dRef, setCamera2dRef] = useState<{ x: number; y: number; zoom: number } | undefined>(urlState.camera2d);
  
  const [showShortcutsHelp, setShowShortcutsHelp] = useState(false);
  const [showFieldGuide, setShowFieldGuide] = useState(false);
  const [showOrientation, setShowOrientation] = useState(() => {
    try { return localStorage.getItem("clustr-oriented") !== "true"; } catch { return true; }
  });
  
  // Ref for search bar to enable focus from keyboard shortcuts
  const searchInputRef = useRef<SearchBarHandle | null>(null);
  const closeFieldGuide = useCallback(() => setShowFieldGuide(false), []);

  // Persist view mode
  useEffect(() => {
    try {
      localStorage.setItem("viewMode", viewMode);
    } catch {
      /* ignore */
    }
  }, [viewMode]);

  useEffect(() => {
    try {
      localStorage.setItem("experienceMode", experienceMode);
    } catch {
      /* ignore */
    }
  }, [experienceMode]);

  useEffect(() => {
    try {
      localStorage.setItem(
        "usePrecomputedLayout",
        usePrecomputedLayout ? "true" : "false"
      );
    } catch {
      /* ignore */
    }
  }, [usePrecomputedLayout]);
  
  useEffect(() => {
    try {
      localStorage.setItem(
        "enableAdaptiveLOD",
        enableAdaptiveLOD ? "true" : "false"
      );
    } catch {
      /* ignore */
    }
  }, [enableAdaptiveLOD]);

  // Sync state to URL with debouncing to avoid excessive history API calls
  const urlWriteTimeoutRef = useRef<number | null>(null);
  
  useEffect(() => {
    // Clear any pending timeout
    if (urlWriteTimeoutRef.current !== null) {
      clearTimeout(urlWriteTimeoutRef.current);
    }
    
    // Debounce URL writes by 500ms
    urlWriteTimeoutRef.current = window.setTimeout(() => {
      writeStateToURL({
        viewMode,
        filters,
        minDegree,
        maxDegree,
        camera3d: camera3dRef,
        camera2d: camera2dRef,
        useCommunityColors,
        usePrecomputedLayout,
        sizeAttenuation,
        enableAdaptiveLOD,
      });
    }, 500);

    return () => {
      if (urlWriteTimeoutRef.current !== null) {
        clearTimeout(urlWriteTimeoutRef.current);
      }
    };
  }, [viewMode, filters, minDegree, maxDegree, camera3dRef, camera2dRef, useCommunityColors, usePrecomputedLayout, sizeAttenuation, enableAdaptiveLOD]);

  // Callback to get current state for sharing
  const getShareState = useCallback((): AppState => ({
    viewMode,
    filters,
    minDegree,
    maxDegree,
    camera3d: camera3dRef,
    camera2d: camera2dRef,
    useCommunityColors,
    usePrecomputedLayout,
    sizeAttenuation,
    enableAdaptiveLOD,
  }), [viewMode, filters, minDegree, maxDegree, camera3dRef, camera2dRef, useCommunityColors, usePrecomputedLayout, sizeAttenuation, enableAdaptiveLOD]);

  // Keyboard shortcut handlers
  const handleFocusSearch = useCallback(() => {
    if (viewMode !== "admin") {
      searchInputRef.current?.focus();
    }
  }, [viewMode]);

  // Keyboard shortcuts
  useKeyboardShortcuts({
    onFocusSearch: viewMode === "admin" ? undefined : handleFocusSearch,
    
    // Sidebar toggle is handled in Sidebar component itself via Ctrl+B
    
    onSwitch3D: useCallback(() => {
      if (viewMode !== "3d" && viewMode !== "admin") {
        setViewMode("3d");
      }
    }, [viewMode]),
    
    onSwitch2D: useCallback(() => {
      if (viewMode !== "2d" && viewMode !== "admin") {
        setViewMode("2d");
      }
    }, [viewMode]),
    
    onSwitchCommunity: useCallback(() => {
      if (viewMode !== "communities" && viewMode !== "admin") {
        setViewMode("communities");
      }
    }, [viewMode]),
    
    onToggleLabels: useCallback(() => {
      if (viewMode === "3d" || viewMode === "2d") {
        setShowLabels(prev => !prev);
      }
    }, [viewMode]),
    
    onEscape: useCallback(() => {
      // Close help overlay if open
      if (showShortcutsHelp) {
        setShowShortcutsHelp(false);
        return;
      }
      // Otherwise deselect node
      setSelectedId(undefined);
      setFocusNodeId(undefined);
    }, [showShortcutsHelp]),
    
    onShowHelp: useCallback(() => {
      setShowShortcutsHelp(prev => !prev);
    }, []),
    
    // Note: Fit graph, reset camera, and arrow navigation require graph instance methods
    // These will be handled by exposing methods from Graph3D/Graph2D components
    // For now, we'll leave them undefined and implement in a follow-up if needed
  });

  return (
    <div className="observatory-shell h-screen w-full">
      <a href="#main-content" className="skip-link">Skip to universe</a>
      {/* Accessibility: Screen reader announcements for state changes */}
      <div 
        role="status" 
        aria-live="polite" 
        aria-atomic="true"
        className="sr-only"
        id="screen-reader-announcements"
      >
        {selectedId ? `Selected ${readableSelection(selectedId)}.` : `Showing ${viewMode === "3d" ? "the three dimensional universe" : viewMode}.`}
      </div>

      <header className="pointer-events-none fixed inset-x-0 top-0 z-50 flex items-start justify-between gap-3 p-3 md:p-5">
        <div className="pointer-events-auto flex items-center gap-3">
          <div className="instrument-panel flex h-12 items-center rounded-full px-4 md:h-14 md:px-5">
            <span className="mr-3 signal-dot" aria-hidden="true" />
            <div><div className="font-semibold tracking-[-.03em] text-white">CLUSTR</div><div className="instrument-label mt-1 hidden sm:block">Community universe</div></div>
          </div>
        </div>
        <div className="pointer-events-auto flex items-center gap-2">
          <button onClick={() => setShowFieldGuide(true)} className="instrument-panel instrument-button h-12 rounded-full px-4 text-xs text-white md:h-14 md:px-5"><span className="hidden sm:inline">Field guide</span><span className="font-mono">?</span></button>
          <button type="button" aria-label={experienceMode === "explore" ? "Analyst" : "Explore"} aria-pressed={experienceMode === "analyst"} onClick={() => setExperienceMode(mode => mode === "explore" ? "analyst" : "explore")} className="instrument-panel instrument-button h-12 rounded-full px-4 text-xs text-white md:h-14 md:px-5"><span className="hidden sm:inline">{experienceMode === "explore" ? "Analyst mode" : "Exit analyst"}</span><span className="sm:hidden">{experienceMode === "explore" ? "Data" : "Close"}</span></button>
        </div>
      </header>

      <nav aria-label="Primary views" className="instrument-panel fixed bottom-[max(1rem,env(safe-area-inset-bottom))] left-1/2 z-50 flex max-w-[calc(100vw-1.5rem)] -translate-x-1/2 items-center gap-1 rounded-full p-1.5">
        {([
          ["3d", "Universe"],
          ["2d", "Map"],
          ["dashboard", "Data"],
          ["communities", "Places"],
        ] as const).map(([mode, label]) => (
          <button
            key={mode}
            type="button"
            onClick={() => setViewMode(mode)}
            data-active={viewMode === mode}
            aria-current={viewMode === mode ? "page" : undefined}
            className="instrument-button rounded-full px-3 text-[11px] sm:px-4 sm:text-xs"
          >
            {label}
          </button>
        ))}
      </nav>
      
      {/* Search bar - visible in all views except admin */}
      {viewMode !== "admin" && (
        <div className="fixed left-1/2 top-20 z-40 w-full max-w-xl -translate-x-1/2 px-3 md:top-5 md:max-w-md lg:max-w-xl">
          <SearchBar
            ref={searchInputRef}
            onSelectNode={(id) => {
              setFocusNodeId(id);
              setSelectedId(id);
              // Switch to 3D view if not already in a graph view
              if (viewMode === "dashboard") {
                setViewMode("3d");
              }
            }}
          />
        </div>
      )}
      
      {/* Main content area */}
      <main id="main-content" tabIndex={-1} className="relative z-0 h-full w-full">
        {viewMode === "admin" ? (
          <section className="content-view"><Admin
            onViewMode={(mode: "3d" | "2d") => {
              setViewMode(mode);
            }}
          /></section>
        ) : viewMode === "dashboard" ? (
          <section className="content-view"><Dashboard
            onViewMode={(mode: "3d" | "2d") => {
              setViewMode(mode);
            }}
            onFocusNode={(id) => {
              setFocusNodeId(id);
              setSelectedId(id);
            }}
          /></section>
        ) : viewMode === "communities" ? (
        <section className="content-view"><Communities
          onViewMode={(mode: "3d" | "2d") => {
            setViewMode(mode);
          }}
          onFocusNode={(id) => {
            setFocusNodeId(id);
            setSelectedId(id);
          }}
          onApplyCommunityColors={(result) => {
            setCommunityResult(result);
            setUseCommunityColors(true);
          }}
        /></section>
      ) : (
        <>
          {experienceMode === "analyst" && <Sidebar
            filters={filters}
            onFiltersChange={setFilters}
            minDegree={minDegree}
            onMinDegreeChange={setMinDegree}
            maxDegree={maxDegree}
            onMaxDegreeChange={setMaxDegree}
            linkOpacity={linkOpacity}
            onLinkOpacityChange={setLinkOpacity}
            nodeRelSize={nodeRelSize}
            onNodeRelSizeChange={setNodeRelSize}
            physics={physics}
            onPhysicsChange={setPhysics}
            subredditSize={subredditSize}
            onSubredditSizeChange={setSubredditSize}
            onFocusNode={setFocusNodeId}
            showLabels={showLabels}
            onShowLabelsChange={setShowLabels}
            graphMode={viewMode === "3d" ? "3d" : "2d"}
            onGraphModeChange={(mode) => setViewMode(mode)}
            onShowDashboard={() => setViewMode("dashboard")}
            onShowCommunities={() => setViewMode("communities")}
            onShowAdmin={() => setViewMode("admin")}
            useCommunityColors={useCommunityColors}
            onToggleCommunityColors={(enabled) =>
              setUseCommunityColors(enabled)
            }
            usePrecomputedLayout={usePrecomputedLayout}
            onTogglePrecomputedLayout={(enabled) =>
              setUsePrecomputedLayout(enabled)
            }
            sizeAttenuation={sizeAttenuation}
            onToggleSizeAttenuation={(enabled) =>
              setSizeAttenuation(enabled)
            }
            enableAdaptiveLOD={enableAdaptiveLOD}
            onToggleAdaptiveLOD={(enabled) =>
              setEnableAdaptiveLOD(enabled)
            }
            currentLODTier={currentLODTier}
          />}
          {experienceMode === "analyst" && <ShareButton getState={getShareState} />}
          {viewMode === "3d" ? (
            <ErrorBoundary
              fallback={(error, retry) => (
                <GraphErrorFallback
                  error={error}
                  onRetry={retry}
                  onFallbackTo2D={() => setViewMode("2d")}
                  mode="3d"
                  webglSupported={detectWebGLSupport()}
                />
              )}
            >
              <Graph3D
                filters={filters}
                minDegree={minDegree}
                maxDegree={maxDegree}
                linkOpacity={linkOpacity}
                nodeRelSize={nodeRelSize}
                physics={physics}
                subredditSize={subredditSize}
                focusNodeId={focusNodeId}
                showLabels={showLabels}
                selectedId={selectedId}
                onNodeSelect={(id?: string) => {
                  setFocusNodeId(id);
                  setSelectedId(id);
                }}
                onInspectNode={(id) => {
                  setFocusNodeId(id);
                  setSelectedId(id);
                  setExperienceMode("analyst");
                }}
                communityResult={useCommunityColors ? communityResult : null}
                usePrecomputedLayout={usePrecomputedLayout}
                initialCamera={camera3dRef}
                onCameraChange={setCamera3dRef}
                sizeAttenuation={sizeAttenuation}
                enableAdaptiveLOD={enableAdaptiveLOD}
                onLODTierChange={setCurrentLODTier}
              />
            </ErrorBoundary>
          ) : (
            <ErrorBoundary
              fallback={(error, retry) => (
                <GraphErrorFallback
                  error={error}
                  onRetry={retry}
                  mode="2d"
                />
              )}
            >
              <Graph2D
                filters={filters}
                minDegree={minDegree}
                maxDegree={maxDegree}
                linkOpacity={linkOpacity}
                nodeRelSize={nodeRelSize}
                physics={physics}
                subredditSize={subredditSize}
                focusNodeId={focusNodeId}
                showLabels={showLabels}
                selectedId={selectedId}
                onNodeSelect={(id?: string) => {
                  setFocusNodeId(id);
                  setSelectedId(id);
                }}
                communityResult={useCommunityColors ? communityResult : null}
                usePrecomputedLayout={usePrecomputedLayout}
                initialCamera={camera2dRef}
                onCameraChange={setCamera2dRef}
              />
            </ErrorBoundary>
          )}
          {experienceMode === "analyst" && <Legend
            filters={filters}
            useCommunityColors={useCommunityColors}
            communityCount={communityResult?.communities.length}
          />}
          {experienceMode === "analyst" && <Inspector
            selected={selectedId ? { id: selectedId, name: readableSelection(selectedId) } : undefined}
            onClear={() => {
              setSelectedId(undefined);
              setFocusNodeId(undefined);
            }}
            onFocus={(id) => {
              setFocusNodeId(id);
              setSelectedId(id);
            }}
          />}
        </>
        )}
      </main>

      {showOrientation && viewMode === "3d" && experienceMode === "explore" && (
        <aside className="instrument-panel fixed bottom-24 left-3 z-40 max-w-sm rounded-2xl p-5 md:bottom-6 md:left-5" aria-label="Universe orientation">
          <p className="instrument-label">Orientation / 001</p>
          <h2 className="mt-3 text-lg font-semibold tracking-tight text-white">This universe is made from relationships.</h2>
          <p className="mt-2 text-sm leading-6 text-[#9aaba8]">Large bodies are communities. Routes grow stronger with repeated activity. Travel closer to reveal finer detail.</p>
          <div className="mt-4 flex gap-2"><button className="instrument-button rounded-full border-white/15 px-4 text-xs text-white" onClick={() => { setShowOrientation(false); try { localStorage.setItem("clustr-oriented", "true"); } catch { /* ignore */ } }}>Begin exploring</button><button className="instrument-button rounded-full px-3 text-xs" onClick={() => setShowFieldGuide(true)}>How it works</button></div>
        </aside>
      )}

      {selectedId && experienceMode === "explore" && (viewMode === "3d" || viewMode === "2d") && (
        <aside className="instrument-panel fixed bottom-24 left-1/2 z-40 flex max-w-[calc(100vw-1.5rem)] -translate-x-1/2 items-center gap-4 rounded-full py-2 pl-4 pr-2" aria-label="Selected object">
          <span className="signal-dot" aria-hidden="true" />
          <div className="min-w-0"><p className="instrument-label">Attention lock</p><p className="max-w-48 truncate text-sm font-medium text-white">{readableSelection(selectedId)}</p></div>
          <button type="button" className="instrument-button rounded-full px-3 text-[11px] text-white" onClick={() => setExperienceMode("analyst")}>Inspect</button>
          <button type="button" className="instrument-button rounded-full px-3 text-[11px]" aria-label="Clear selection" onClick={() => { setSelectedId(undefined); setFocusNodeId(undefined); }}>×</button>
        </aside>
      )}
      
      {/* Keyboard shortcuts help overlay */}
      <KeyboardShortcutsHelp 
        isOpen={showShortcutsHelp}
        onClose={() => setShowShortcutsHelp(false)}
      />
      <FieldGuide open={showFieldGuide} onClose={closeFieldGuide} />
    </div>
  );
}

export default App;
