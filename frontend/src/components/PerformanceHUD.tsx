import { useEffect, useRef } from 'react';
import type { WebGLRenderer } from 'three';
import { useMobileDetect } from '../hooks/useMobileDetect';

interface PerformanceHUDProps {
    renderer: WebGLRenderer | null;
    nodeCount?: number;
    totalNodeCount?: number;
    simulationState?: 'active' | 'idle' | 'precomputed';
    lodLevel?: number;
}

/**
 * PerformanceHUD - Real-time performance overlay for monitoring rendering performance
 * 
 * Uses direct DOM manipulation instead of React state to minimize overhead.
 * Updates at 1Hz to avoid impacting performance.
 * 
 * Metrics displayed:
 * - FPS (rolling average over last 60 frames)
 * - Draw calls per frame
 * - Visible/total node count
 * - GPU memory estimate
 * - Current LOD tier
 * - Simulation state
 */
export default function PerformanceHUD({
    renderer,
    nodeCount = 0,
    totalNodeCount = 0,
    simulationState = 'idle',
    lodLevel = 0,
}: PerformanceHUDProps) {
    const containerRef = useRef<HTMLDivElement>(null);
    const isVisibleRef = useRef<boolean>(false);
    const fpsHistoryRef = useRef<number[]>([]);
    const lastFrameTimeRef = useRef<number>(performance.now());
    const updateIntervalRef = useRef<number | null>(null);
    const rafIdRef = useRef<number | null>(null);
    const { isMobile } = useMobileDetect();

    // Initialize visibility state from localStorage or env
    useEffect(() => {
        // Hidden by default in production unless explicitly enabled
        const isProduction = import.meta.env.PROD;
        const forceShow = import.meta.env.VITE_SHOW_PERFORMANCE_HUD === 'true';
        
        let initialVisibility = false;
        if (forceShow) {
            // Force show when explicitly enabled via env var
            initialVisibility = true;
        } else if (!isProduction) {
            // In development, restore from localStorage
            try {
                const saved = localStorage.getItem('performanceHUD:visible');
                initialVisibility = saved === 'true';
            } catch {
                // ignore localStorage errors
            }
        }
        
        isVisibleRef.current = initialVisibility;
        if (containerRef.current) {
            containerRef.current.style.display = initialVisibility ? 'block' : 'none';
        }
    }, []);

    // Keyboard shortcut handler
    useEffect(() => {
        const isProduction = import.meta.env.PROD;
        const forceShow = import.meta.env.VITE_SHOW_PERFORMANCE_HUD === 'true';

        // In production, keep HUD inert unless explicitly enabled
        if (isProduction && !forceShow) {
            return;
        }

        const handleKeyDown = (e: KeyboardEvent) => {
            // Ctrl+Shift+P toggles the Performance HUD
            // Don't use F12 to avoid blocking DevTools
            if (e.ctrlKey && e.shiftKey && e.key.toLowerCase() === 'p') {
                e.preventDefault();
                isVisibleRef.current = !isVisibleRef.current;
                
                if (containerRef.current) {
                    containerRef.current.style.display = isVisibleRef.current
                        ? 'block'
                        : 'none';
                }
                
                // Persist visibility preference
                try {
                    localStorage.setItem(
                        'performanceHUD:visible',
                        String(isVisibleRef.current)
                    );
                } catch {
                    // ignore localStorage errors
                }
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, []);

    // FPS tracking via RAF
    useEffect(() => {
        const trackFPS = () => {
            const now = performance.now();
            const delta = now - lastFrameTimeRef.current;
            
            if (delta > 0) {
                const fps = 1000 / delta;
                fpsHistoryRef.current.push(fps);
                
                // Keep only last 60 frames for rolling average
                if (fpsHistoryRef.current.length > 60) {
                    fpsHistoryRef.current.shift();
                }
            }
            
            lastFrameTimeRef.current = now;
            rafIdRef.current = requestAnimationFrame(trackFPS);
        };

        rafIdRef.current = requestAnimationFrame(trackFPS);
        
        return () => {
            if (rafIdRef.current !== null) {
                cancelAnimationFrame(rafIdRef.current);
            }
        };
    }, []);

    // Update HUD display at 1Hz
    useEffect(() => {
        const updateDisplay = () => {
            if (!containerRef.current || !isVisibleRef.current) return;

            // Calculate average FPS
            const avgFPS =
                fpsHistoryRef.current.length > 0
                    ? fpsHistoryRef.current.reduce((a, b) => a + b, 0) /
                      fpsHistoryRef.current.length
                    : 0;

            // Get renderer info
            let drawCalls = 0;
            let triangles = 0;
            let textures = 0;
            let geometries = 0;
            let memoryEstimateMB = 0;

            if (renderer) {
                const info = renderer.info;
                drawCalls = info.render.calls;
                triangles = info.render.triangles;
                textures = info.memory.textures;
                geometries = info.memory.geometries;
                
                // Rough memory estimate:
                // Assume ~100 bytes per node + texture memory
                const nodeMem = nodeCount * 100;
                const texMem = textures * 1024 * 1024; // Assume 1MB per texture
                memoryEstimateMB = (nodeMem + texMem) / (1024 * 1024);
            }

            // Update DOM directly to avoid React re-renders
            const lines = [
                `FPS: ${avgFPS.toFixed(1)}`,
                `Draw Calls: ${drawCalls}`,
                `Triangles: ${triangles.toLocaleString()}`,
                `Nodes: ${nodeCount.toLocaleString()}${totalNodeCount > nodeCount ? ` / ${totalNodeCount.toLocaleString()}` : ''}`,
                `GPU Mem: ~${memoryEstimateMB.toFixed(1)} MB`,
                `Textures: ${textures}`,
                `Geometries: ${geometries}`,
                `LOD: ${lodLevel}`,
                `Simulation: ${simulationState}`,
            ];

            containerRef.current.textContent = lines.join('\n');
        };

        // Update immediately
        updateDisplay();
        
        // Then update every 1 second
        updateIntervalRef.current = window.setInterval(updateDisplay, 1000);

        return () => {
            if (updateIntervalRef.current !== null) {
                clearInterval(updateIntervalRef.current);
            }
        };
    }, [renderer, nodeCount, totalNodeCount, simulationState, lodLevel]);

    return (
        <div
            ref={containerRef}
            className={`fixed z-50 bg-black/80 text-green-400 font-mono text-xs px-3 py-2 rounded-sm pointer-events-none whitespace-pre
                ${isMobile 
                    ? 'top-16 right-2' /* Mobile: top-right */
                    : 'top-16 left-2' /* Desktop: top-left */
                }`}
            style={{ display: 'none' }}
            aria-label="Performance metrics overlay"
        />
    );
}
