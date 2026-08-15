import * as THREE from 'three';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { GraphData } from '../types/graph';
import {
    InstancedNodeRenderer,
    type NodeData,
} from '../rendering/InstancedNodeRenderer';
import { LinkRenderer } from '../rendering/LinkRenderer';
import {
    ForceSimulation,
    type PhysicsConfig,
} from '../rendering/ForceSimulation';
import { SDFTextRenderer, type LabelData } from '../rendering/SDFTextRenderer';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { detectWebGLSupport } from '../utils/webglDetect';
import { AdaptiveLODManager, LODTier } from '../utils/levelOfDetail';
import LoadingSkeleton from './LoadingSkeleton';
import NodeTooltip from './NodeTooltip';
import PerformanceHUD from './PerformanceHUD';
import Minimap from './Minimap';
import { DEFAULT_LOD_CONFIG } from '../utils/levelOfDetail';
import { useTheme } from '../contexts/ThemeContext';
import { useMobileDetect, getMobileGraphConfig } from '../hooks/useMobileDetect';
import type { SpatialScene } from '../data/SpatialSceneClient';
import { useSpatialWorldClient } from '../contexts/spatialWorld';
import { DEFAULT_CAMERA_POSE, poseForTarget, prefersReducedMotion } from '../navigation/SpatialNavigation';

/**
 * Graph3DInstanced - High-performance 3D graph visualization using InstancedMesh
 *
 * Replaces react-force-graph-3d with a custom THREE.js renderer that uses
 * InstancedMesh for dramatically improved performance with large graphs.
 *
 * Performance characteristics:
 * - Renders 100k nodes in ~4 draw calls for nodes (one per node type)
 * - Position updates in <5ms
 * - Memory usage <500MB for 100k nodes
 *
 * This component maintains API compatibility with the original Graph3D component
 * to allow for easy migration.
 */

type Filters = {
    subreddit: boolean;
    user: boolean;
    post: boolean;
    comment: boolean;
};

const TYPE_ORDER: Array<keyof Filters> = [
    'subreddit',
    'user',
    'post',
    'comment',
];

interface Props {
    filters: Filters;
    minDegree?: number;
    maxDegree?: number;
    linkOpacity: number;
    nodeRelSize: number;
    physics?: PhysicsConfig;
    focusNodeId?: string;
    selectedId?: string;
    onNodeSelect?: (id?: string) => void;
    onInspectNode?: (id: string) => void;
    showLabels?: boolean;
    communityResult?: {
        nodeCommunities: Map<string, number>;
        communities: Array<{ id: number; color: string }>;
    } | null;
    usePrecomputedLayout?: boolean;
    initialCamera?: { x: number; y: number; z: number };
    onCameraChange?: (camera: { x: number; y: number; z: number }) => void;
    sizeAttenuation?: boolean;
    enableAdaptiveLOD?: boolean;
    lodConfig?: {
        fpsDowngradeThreshold?: number;
        fpsUpgradeThreshold?: number;
        enableAdaptiveLOD?: boolean;
    };
    onLODTierChange?: (tier: number) => void;
}

export default function Graph3DInstanced(props: Props) {
    const {
        filters,
        minDegree,
        maxDegree,
        linkOpacity,
        nodeRelSize,
        physics,
        focusNodeId,
        selectedId,
        onNodeSelect,
        onInspectNode,
        showLabels,
        communityResult,
        usePrecomputedLayout,
        initialCamera,
        onCameraChange,
        sizeAttenuation = true,
        enableAdaptiveLOD = true,
        lodConfig,
        onLODTierChange,
    } = props;

    const { theme } = useTheme();
    const spatialWorldClient = useSpatialWorldClient();
    
    // Mobile detection
    const { isMobile, isTablet, isTouchDevice } = useMobileDetect();
    const mobileConfig = useMemo(
        () => getMobileGraphConfig(isMobile, isTablet),
        [isMobile, isTablet]
    );

    // State
    const [graphData, setGraphData] = useState<GraphData | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [initialLoadComplete, setInitialLoadComplete] = useState(false);
    const [webglSupported] = useState(() => detectWebGLSupport());
    const [onlyLinked, setOnlyLinked] = useState(true);
    const [currentLODTier, setCurrentLODTier] = useState<LODTier>(() => {
        // Use mobile-optimized default LOD tier on mobile devices
        return isMobile || isTablet ? LODTier.MEDIUM : LODTier.HIGH;
    });
    const [currentCamera, setCurrentCamera] = useState<{ x: number; y: number; z: number } | undefined>();
    const [currentCameraTarget, setCurrentCameraTarget] = useState<{ x: number; y: number; z: number } | undefined>();
    const [scene, setScene] = useState<SpatialScene | null>(null);

    // Refs for Three.js objects
    const containerRef = useRef<HTMLDivElement>(null);
    const sceneRef = useRef<THREE.Scene | null>(null);
    const cameraRef = useRef<THREE.PerspectiveCamera | null>(null);
    const rendererRef = useRef<THREE.WebGLRenderer | null>(null);
    const controlsRef = useRef<OrbitControls | null>(null);
    const nodeRendererRef = useRef<InstancedNodeRenderer | null>(null);
    const linkRendererRef = useRef<LinkRenderer | null>(null);
    const simulationRef = useRef<ForceSimulation | null>(null);
    const labelRendererRef = useRef<SDFTextRenderer | null>(null);
    const raycasterRef = useRef<THREE.Raycaster>(new THREE.Raycaster());
    const mouseRef = useRef<THREE.Vector2>(new THREE.Vector2());
    const hoveredNodeRef = useRef<string | null>(null);
    const keyboardNodeIndexRef = useRef(0);
    const showLabelsRef = useRef<boolean>(false);
    const loadedFocusRef = useRef<string | null>(null);
    const labelSetRef = useRef<Set<string>>(new Set());
    const lastFrameTimeRef = useRef<number>(performance.now());
    const lastEmittedTierRef = useRef<LODTier>(currentLODTier);
    const lastFramedRevisionRef = useRef<string | null>(null);
    const overviewRadiusRef = useRef<number | null>(null);
    const lastRegionSignatureRef = useRef<string | null>(null);
    const regionAbortRef = useRef<AbortController | null>(null);
    const requestRegionRef = useRef<(camera: THREE.PerspectiveCamera, controls: OrbitControls) => void>(() => undefined);
    const sceneClientRef = useRef(spatialWorldClient);
    const sceneSourceRef = useRef<SpatialScene['source'] | null>(null);
    const onCameraChangeRef = useRef(onCameraChange);
    const onLODTierChangeRef = useRef(onLODTierChange);

    useEffect(() => { onCameraChangeRef.current = onCameraChange; }, [onCameraChange]);
    useEffect(() => { onLODTierChangeRef.current = onLODTierChange; }, [onLODTierChange]);
    useEffect(() => { sceneSourceRef.current = scene?.source ?? null; }, [scene?.source]);

    // Camera-driven discovery is the seam between the complete catalog and the
    // bounded GPU scene. A settled, meaningfully zoomed-in camera asks for the
    // visible spatial volume; the browser never downloads the full corpus.
    requestRegionRef.current = (camera, controls) => {
        if (!scene?.revision || !scene.catalog || focusNodeId || selectedId) return;
        const overviewRadius = overviewRadiusRef.current;
        const distance = camera.position.distanceTo(controls.target);
        if (!overviewRadius || distance >= overviewRadius * 1.6) return;

        const halfExtent = Math.max(20, distance * Math.tan(THREE.MathUtils.degToRad(camera.fov / 2)) * 0.9);
        const quantum = Math.max(5, halfExtent / 4);
        const signature = [controls.target.x, controls.target.y, controls.target.z, halfExtent]
            .map(value => Math.round(value / quantum))
            .join(':');
        if (signature === lastRegionSignatureRef.current) return;
        lastRegionSignatureRef.current = signature;

        regionAbortRef.current?.abort();
        const controller = new AbortController();
        regionAbortRef.current = controller;
        const target = controls.target;
        setLoading(true);
        sceneClientRef.current.region({
            xMin: target.x - halfExtent,
            xMax: target.x + halfExtent,
            yMin: target.y - halfExtent,
            yMax: target.y + halfExtent,
            zMin: target.z - halfExtent,
            zMax: target.z + halfExtent,
        }, controller.signal)
            .then(spatialScene => {
                if (!spatialScene.nodes.length) return;
                setScene(spatialScene);
                setGraphData({ nodes: spatialScene.nodes, links: spatialScene.links });
            })
            .catch(error => {
                if ((error as { name?: string }).name !== 'AbortError') setError((error as Error).message);
            })
            .finally(() => {
                if (regionAbortRef.current === controller) regionAbortRef.current = null;
                if (!controller.signal.aborted) setLoading(false);
            });
    };

    // State for tooltip
    const [hoveredNode] = useState<{
        id: string;
        name?: string;
        type?: string;
        mouseX: number;
        mouseY: number;
    } | null>(null);

    // Use mobile-optimized limits or environment variables
    const MAX_RENDER_NODES = useMemo(() => {
        // Check environment variable first
        const raw = import.meta.env?.VITE_MAX_RENDER_NODES as unknown as
            | string
            | number
            | undefined;
        const envValue = typeof raw === 'string' ? parseInt(raw) : Number(raw);
        if (Number.isFinite(envValue) && (envValue as number) > 0) {
            return envValue as number;
        }
        // Fall back to mobile-optimized defaults
        return mobileConfig.maxRenderNodes;
    }, [mobileConfig.maxRenderNodes]);

    const MAX_RENDER_LINKS = useMemo(() => {
        // Check environment variable first
        const raw = import.meta.env?.VITE_MAX_RENDER_LINKS as unknown as
            | string
            | number
            | undefined;
        const envValue = typeof raw === 'string' ? parseInt(raw) : Number(raw);
        if (Number.isFinite(envValue) && (envValue as number) > 0) {
            return envValue as number;
        }
        // Fall back to mobile-optimized defaults
        return mobileConfig.maxRenderLinks;
    }, [mobileConfig.maxRenderLinks]);

    const activeTypes = useMemo(() => {
        const enabled = Object.entries(filters)
            .filter(([, value]) => value)
            .map(([key]) => key as keyof Filters);
        return enabled.sort(
            (a, b) => TYPE_ORDER.indexOf(a) - TYPE_ORDER.indexOf(b),
        );
    }, [filters]);

    const activeTypesRef = useRef<string[]>(activeTypes);

    useEffect(() => {
        activeTypesRef.current = activeTypes;
    }, [activeTypes]);

    // Load the spatial overview first. A revision-aware client pins a world
    // before continuations and falls back only for older backend deployments.
    const load = useCallback(
        async ({
            signal,
            types,
        }: { signal?: AbortSignal; types?: string[] } = {}) => {
            const selected =
                types && types.length > 0 ? types : activeTypesRef.current;
            if (!selected || selected.length === 0) {
                setGraphData({ nodes: [], links: [] });
                setError(null);
                setLoading(false);
                return;
            }
            setLoading(true);
            setError(null);
            try {
                const spatialScene = await sceneClientRef.current.overview(signal);
                const allowed = new Set(selected);
                const nodeIds = new Set(spatialScene.nodes.filter(n => n.type === 'community' || !n.type || allowed.has(n.type)).map(n => n.id));
                const data = { nodes: spatialScene.nodes.filter(n => nodeIds.has(n.id)), links: spatialScene.links.filter(l => nodeIds.has(l.source) && nodeIds.has(l.target)) };
                lastRegionSignatureRef.current = null;
                setScene(spatialScene);
                setGraphData(data);
                setInitialLoadComplete(true);
            } catch (err) {
                if ((err as { name?: string })?.name === 'AbortError') return;
                setError((err as Error).message);
                setGraphData(null);
            } finally {
                if (!signal || !signal.aborted) {
                    setLoading(false);
                }
            }
        },
        [],
    );

    useEffect(() => {
        if (activeTypes.length === 0) {
            setGraphData({ nodes: [], links: [] });
            setError(null);
            setLoading(false);
            return;
        }
        const controller = new AbortController();
        load({ signal: controller.signal, types: activeTypes });
        return () => controller.abort();
    }, [activeTypes, load]);

    useEffect(() => () => regionAbortRef.current?.abort(), []);

    // Approaching a landmark or search result replaces the far overview with
    // its revision-pinned selectable neighborhood. Full-corpus entities do not
    // need to be resident before search-to-flight can reach them.
    useEffect(() => {
        if (!focusNodeId) {
            loadedFocusRef.current = null;
            return;
        }
        if (loadedFocusRef.current === focusNodeId) return;
        if (!focusNodeId.startsWith('c:') && graphData?.nodes.some(node => node.id === focusNodeId)) return;
        const controller = new AbortController();
        setLoading(true);
        const request = focusNodeId.startsWith('c:')
            ? sceneClientRef.current.community(focusNodeId, 'near', controller.signal)
            : sceneClientRef.current.entity(focusNodeId, controller.signal);
        request
            .then(spatialScene => {
                loadedFocusRef.current = focusNodeId;
                setScene(spatialScene);
                setGraphData({ nodes: spatialScene.nodes, links: spatialScene.links });
            })
            .catch(error => {
                if ((error as { name?: string }).name !== 'AbortError') setError((error as Error).message);
            })
            .finally(() => { if (!controller.signal.aborted) setLoading(false); });
        return () => controller.abort();
    }, [focusNodeId, graphData?.nodes]);

    // Initialize Three.js scene
    useEffect(() => {
        const container = containerRef.current;
        if (!container || !webglSupported) return;

        // Create scene
        const scene = new THREE.Scene();
        scene.background = new THREE.Color(0x030506);
        sceneRef.current = scene;

        // Create camera
        const camera = new THREE.PerspectiveCamera(
            75,
            container.clientWidth / container.clientHeight,
            0.1,
            10000,
        );
        camera.position.set(DEFAULT_CAMERA_POSE.x, DEFAULT_CAMERA_POSE.y, DEFAULT_CAMERA_POSE.z);
        cameraRef.current = camera;

        // Create renderer
        const renderer = new THREE.WebGLRenderer({
            antialias: false,
            powerPreference: 'high-performance',
        });
        renderer.setSize(
            container.clientWidth,
            container.clientHeight,
        );
        // Use mobile-optimized pixel ratio
        renderer.setPixelRatio(Math.min(window.devicePixelRatio, mobileConfig.pixelRatio));
        container.appendChild(renderer.domElement);
        rendererRef.current = renderer;

        // Create controls with touch support
        const controls = new OrbitControls(camera, renderer.domElement);
        controls.enableDamping = true;
        controls.dampingFactor = 0.05;
        
        // Enable touch gestures for mobile devices
        if (isTouchDevice) {
            // Touch gestures configuration
            controls.touches = {
                ONE: THREE.TOUCH.ROTATE,      // One finger: rotate camera
                TWO: THREE.TOUCH.DOLLY_PAN,   // Two fingers: pinch-to-zoom and pan
            };
            // Adjust rotation speed for touch
            controls.rotateSpeed = 0.7;
            // Adjust pan speed for touch
            controls.panSpeed = 0.7;
            // Adjust zoom speed for pinch gesture
            controls.zoomSpeed = 1.2;
        }
        
        controlsRef.current = controls;
        const handleControlsEnd = () => requestRegionRef.current(camera, controls);
        controls.addEventListener('end', handleControlsEnd);

        // Add lights
        const ambientLight = new THREE.AmbientLight(0xffffff, 0.6);
        scene.add(ambientLight);

        const directionalLight = new THREE.DirectionalLight(0xffffff, 0.4);
        directionalLight.position.set(1, 1, 1);
        scene.add(directionalLight);

        // Create node renderer
        const nodeRenderer = new InstancedNodeRenderer(scene, {
            maxNodes: MAX_RENDER_NODES,
            nodeRelSize,
            sizeAttenuation,
        });
        nodeRendererRef.current = nodeRenderer;
        
        // Set camera reference for distance-based scaling
        nodeRenderer.setCamera(camera);

        // Create link renderer with initial opacity
        const linkRenderer = new LinkRenderer(scene, {
            maxLinks: MAX_RENDER_LINKS,
            opacity: linkOpacity,
        });
        linkRendererRef.current = linkRenderer;

        // Create SDF text label renderer
        const labelRenderer = new SDFTextRenderer(scene, {
            maxLabels: DEFAULT_LOD_CONFIG.maxLabels,
            fontSize: 8,
        });
        labelRendererRef.current = labelRenderer;

        // Initialize LOD manager
        const lodManager = new AdaptiveLODManager();
        if (lodConfig) {
            lodManager.setConfig({
                enableAdaptiveLOD: enableAdaptiveLOD && (lodConfig.enableAdaptiveLOD ?? true),
                fpsDowngradeThreshold: lodConfig.fpsDowngradeThreshold ?? 24,
                fpsUpgradeThreshold: lodConfig.fpsUpgradeThreshold ?? 50,
            });
        } else {
            lodManager.setConfig({ enableAdaptiveLOD });
        }

        // Set initial camera if provided
        if (initialCamera) {
            camera.position.set(
                initialCamera.x,
                initialCamera.y,
                initialCamera.z,
            );
        } else {
            controls.target.set(DEFAULT_CAMERA_POSE.targetX, DEFAULT_CAMERA_POSE.targetY, DEFAULT_CAMERA_POSE.targetZ);
        }

        // Track last camera position for throttling
        let lastCameraUpdate = 0;
        let lastLinkVisibilityUpdate = 0;
        const CAMERA_UPDATE_INTERVAL = 1000; // Update every 1 second
        // Link visibility update: minimum interval between checks (actual timing depends on frame rate)
        // LinkRenderer has built-in camera movement detection to skip redundant updates
        const LINK_VISIBILITY_UPDATE_INTERVAL = 300; // Min 300ms between visibility checks
        const lastCamPos = { x: NaN, y: NaN, z: NaN };
        const EPSILON = 1e-3;

        // Animation loop
        let animationId: number;
        const animate = () => {
            animationId = requestAnimationFrame(animate);
            
            // Track FPS
            const now = performance.now();
            const delta = now - lastFrameTimeRef.current;
            if (delta > 0) {
                const fps = 1000 / delta;
                lodManager.recordFrame(fps);
            }
            lastFrameTimeRef.current = now;
            
            // Update LOD manager
            lodManager.update(now);
            const lodParams = lodManager.getRenderingParams(now);
            
            // Update LOD tier state if changed (use ref to avoid stale closure)
            if (lodParams.tier !== lastEmittedTierRef.current) {
                lastEmittedTierRef.current = lodParams.tier;
                setCurrentLODTier(lodParams.tier);
                onLODTierChangeRef.current?.(lodParams.tier);
            }
            
            controls.update();

            // Update node camera position for distance-based scaling
            if (nodeRenderer) {
                nodeRenderer.updateCameraPosition();
            }

            // Update link visibility and opacity based on LOD
            // updateVisibility() skips work if camera hasn't moved significantly
            // refresh() skips work if visibility hasn't changed (needsUpdate flag)
            const linkUpdateTime = Date.now();
            if (
                linkRenderer &&
                linkUpdateTime - lastLinkVisibilityUpdate > LINK_VISIBILITY_UPDATE_INTERVAL
            ) {
                if (lodParams.showLinks) {
                    linkRenderer.updateVisibility(camera);
                    // Apply LOD opacity multiplier
                    linkRenderer.setOpacity(linkOpacity * lodParams.linkOpacityMultiplier);
                    linkRenderer.refresh();
                } else {
                    // Hide all links in LOW/EMERGENCY tiers
                    linkRenderer.setOpacity(0);
                    linkRenderer.refresh();
                }
                lastLinkVisibilityUpdate = linkUpdateTime;
            }

            // Update label visibility and billboard orientation
            if (labelRendererRef.current && showLabelsRef.current && labelSetRef.current.size > 0) {
                // Zoom is distance to the navigation target, not distance to
                // the universe origin. The old origin check hid labels around
                // any far-away community even after the camera arrived there.
                const cameraDistance = camera.position.distanceTo(controls.target);
                const labelDistance = sceneSourceRef.current === 'overview'
                    ? Number.POSITIVE_INFINITY
                    : DEFAULT_LOD_CONFIG.labelVisibilityThreshold;
                labelRendererRef.current.updateVisibility(
                    camera,
                    labelSetRef.current,
                    cameraDistance,
                    labelDistance
                );
                labelRendererRef.current.updateBillboard(camera);
            }

            renderer.render(scene, camera);

            // Throttle camera change emissions
            if (onCameraChangeRef.current) {
                if (linkUpdateTime - lastCameraUpdate > CAMERA_UPDATE_INTERVAL) {
                    const { x, y, z } = camera.position;
                    // Only emit if position changed significantly
                    if (
                        Math.abs(x - lastCamPos.x) > EPSILON ||
                        Math.abs(y - lastCamPos.y) > EPSILON ||
                        Math.abs(z - lastCamPos.z) > EPSILON
                    ) {
                        onCameraChangeRef.current({ x, y, z });
                        lastCamPos.x = x;
                        lastCamPos.y = y;
                        lastCamPos.z = z;
                        lastCameraUpdate = linkUpdateTime;
                    }
                }
            }
        };
        animate();

        // Handle resize
        const handleResize = () => {
            camera.aspect =
                container.clientWidth / container.clientHeight;
            camera.updateProjectionMatrix();
            renderer.setSize(
                container.clientWidth,
                container.clientHeight,
            );
        };
        window.addEventListener('resize', handleResize);

        // Cleanup
        return () => {
            window.removeEventListener('resize', handleResize);
            cancelAnimationFrame(animationId);
            controls.removeEventListener('end', handleControlsEnd);
            controls.dispose();
            renderer.dispose();
            nodeRenderer.dispose();
            linkRenderer.dispose();
            
            // Dispose label renderer
            if (labelRendererRef.current) {
                labelRendererRef.current.dispose();
                labelRendererRef.current = null;
            }
            
            if (container && renderer.domElement.parentNode === container) {
                container.removeChild(renderer.domElement);
            }
        };
    // Renderer identity follows the canvas/device lifecycle. Live visual values
    // are applied through focused effects below; rebuilding here would clear
    // instance data and recreate the camera on every telemetry update.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        webglSupported,
        initialLoadComplete,
        MAX_RENDER_NODES,
        MAX_RENDER_LINKS,
        // Note: mobileConfig.pixelRatio and isTouchDevice are now stable after first render
        // since useMobileDetect computes them synchronously, so they won't trigger re-renders
        mobileConfig.pixelRatio,
        isTouchDevice,
    ]);

    // Track camera position for minimap (update every second to match original renderer)
    useEffect(() => {
        if (!cameraRef.current) return;

        const interval = setInterval(() => {
            if (cameraRef.current) {
                const { x, y, z } = cameraRef.current.position;
                setCurrentCamera({ x, y, z });
                if (controlsRef.current) {
                    const target = controlsRef.current.target;
                    setCurrentCameraTarget({ x: target.x, y: target.y, z: target.z });
                }
                if (onCameraChange) {
                    onCameraChange({ x, y, z });
                }
            }
        }, 1000);

        return () => clearInterval(interval);
    }, [onCameraChange]);

    // Process graph data with filters
    const filtered = useMemo(() => {
        if (!graphData) return { nodes: [], links: [] };

        const allowed = new Set(
            Object.entries(filters)
                .filter(([, v]) => v)
                .map(([k]) => k),
        );

        const hasCommunities = graphData.nodes.some(node => node.type === 'community');
        const semanticTypes = currentLODTier <= LODTier.LOW
            ? new Set([hasCommunities ? 'community' : 'subreddit'])
            : currentLODTier === LODTier.MEDIUM
                ? new Set(['community', 'subreddit', 'user'])
                : new Set(['community', 'subreddit', 'user', 'post', 'comment']);
        let nodes = graphData.nodes.filter(n =>
            semanticTypes.has(n.type || 'community') &&
            (n.type === 'community' || !n.type || allowed.has(n.type)),
        );
        let links = graphData.links;

        // Apply degree filters if specified
        if (minDegree !== undefined || maxDegree !== undefined) {
            const degreeMap = new Map<string, number>();
            for (const link of links) {
                degreeMap.set(
                    link.source,
                    (degreeMap.get(link.source) || 0) + 1,
                );
                degreeMap.set(
                    link.target,
                    (degreeMap.get(link.target) || 0) + 1,
                );
            }

            nodes = nodes.filter(n => {
                const degree = degreeMap.get(n.id) || 0;
                if (minDegree !== undefined && degree < minDegree) return false;
                if (maxDegree !== undefined && degree > maxDegree) return false;
                return true;
            });
        }

        const nodeIds = new Set(nodes.map(n => n.id));
        links = links.filter(
            l => nodeIds.has(l.source) && nodeIds.has(l.target),
        );

        // Filter to only linked nodes if enabled
        if (onlyLinked) {
            const linkedIds = new Set<string>();
            for (const link of links) {
                linkedIds.add(link.source);
                linkedIds.add(link.target);
            }
            nodes = nodes.filter(n => linkedIds.has(n.id));
        }

        // Limit to max nodes/links
        if (
            nodes.length > MAX_RENDER_NODES ||
            links.length > MAX_RENDER_LINKS
        ) {
            // Simple truncation for now (could be improved with weighting)
            nodes = nodes.slice(0, MAX_RENDER_NODES);
            const nodeIdSet = new Set(nodes.map(n => n.id));
            links = links
                .filter(l => nodeIdSet.has(l.source) && nodeIdSet.has(l.target))
                .slice(0, MAX_RENDER_LINKS);
        }

        if (selectedId && !nodes.some(node => node.id === selectedId)) {
            const selected = graphData.nodes.find(node => node.id === selectedId);
            if (selected) {
                nodes = [...nodes, selected];
                const visible = new Set(nodes.map(node => node.id));
                const pinnedLinks = graphData.links.filter(link => (link.source === selectedId || link.target === selectedId) && visible.has(link.source) && visible.has(link.target));
                links = [...links, ...pinnedLinks.filter(link => !links.includes(link))];
            }
        }

        return { nodes, links };
    }, [
        graphData,
        filters,
        minDegree,
        maxDegree,
        onlyLinked,
        MAX_RENDER_NODES,
        MAX_RENDER_LINKS,
        selectedId,
        currentLODTier,
    ]);

    // Frame each published overview once. Persisted worlds can have very
    // different coordinate extents, so a fixed origin-facing camera is not a
    // meaningful default and was a major source of apparently blank scenes.
    useEffect(() => {
        if (!scene?.revision || scene.source !== 'overview' || !cameraRef.current || !controlsRef.current || !filtered.nodes.length) return;
        if (lastFramedRevisionRef.current === scene.revision) return;
        const communities = filtered.nodes.filter(node => node.type === 'community');
        const subreddits = filtered.nodes.filter(node => node.type === 'subreddit');
        const landmarks = communities.length ? communities : subreddits.length ? subreddits : filtered.nodes;
        const points = landmarks
            .filter(node => Number.isFinite(node.x) && Number.isFinite(node.y) && Number.isFinite(node.z))
            .map(node => new THREE.Vector3(node.x, node.y, node.z));
        if (!points.length) return;
        const bounds = new THREE.Box3().setFromPoints(points);
        const sphere = bounds.getBoundingSphere(new THREE.Sphere());
        overviewRadiusRef.current = sphere.radius;
        const camera = cameraRef.current;
        const distance = Math.max(160, sphere.radius / Math.sin(THREE.MathUtils.degToRad(camera.fov / 2)) * 1.3);
        const direction = new THREE.Vector3(0.28, 0.18, 1).normalize();
        camera.position.copy(sphere.center).addScaledVector(direction, distance);
        camera.near = Math.max(0.1, distance / 1000);
        camera.far = Math.max(10000, distance * 12);
        camera.updateProjectionMatrix();
        controlsRef.current.target.copy(sphere.center);
        controlsRef.current.update();
        lastFramedRevisionRef.current = scene.revision;
    }, [scene, filtered.nodes]);

    // Build degree map for label selection
    const degreeMap = useMemo(() => {
        const map = new Map<string, number>();
        for (const link of filtered.links) {
            map.set(link.source, (map.get(link.source) || 0) + 1);
            map.set(link.target, (map.get(link.target) || 0) + 1);
        }
        return map;
    }, [filtered.links]);

    // Choose nodes to label (top-N by weight, including far-view landmarks)
    const labelSet = useMemo(() => {
        if (!showLabels) return new Set<string>();
        
        const weights = filtered.nodes.map((n) => {
            const deg = degreeMap.get(n.id) || 0;
            const val = typeof n.val === 'number' ? n.val : 0;
            const w = Math.max(val, deg);
            return { id: n.id, type: n.type, name: n.name || n.id, w };
        });
        
        // Prefer semantic navigation entities; limit to top N by weight
        const preferred = weights.filter((x) =>
            x.id === selectedId || x.type === 'community' || x.type === 'subreddit' || x.type === 'user'
        );
        preferred.sort(
            (a, b) => b.w - a.w || String(a.id).localeCompare(String(b.id))
        );
        
        const semanticLimit = scene?.source === 'overview'
            ? 40
            : currentLODTier >= LODTier.HIGH ? DEFAULT_LOD_CONFIG.maxLabels : 80;
        const TOP = Math.min(semanticLimit, preferred.length);
        const set = new Set<string>();
        if (selectedId && filtered.nodes.some(node => node.id === selectedId)) set.add(selectedId);
        for (let i = 0; i < TOP; i++) set.add(String(preferred[i].id));
        return set;
    }, [showLabels, filtered.nodes, degreeMap, selectedId, scene?.source, currentLODTier]);

    // Keep refs in sync with labelSet and showLabels
    useEffect(() => {
        showLabelsRef.current = showLabels || false;
        labelSetRef.current = labelSet;
    }, [showLabels, labelSet]);

    // Update node renderer when filtered data changes
    useEffect(() => {
        if (!nodeRendererRef.current) return;

        // If there are no filtered nodes, clear the renderer so the scene matches the current state
        if (!filtered.nodes.length) {
            nodeRendererRef.current.setNodeData([]);
            return;
        }

        const nodeData: NodeData[] = filtered.nodes.map(node => {
            // Get color from community or type
            let color: string | undefined;
            if (node.primary_color) {
                color = node.primary_color;
            } else if (communityResult) {
                const commId = communityResult.nodeCommunities.get(node.id);
                if (commId !== undefined) {
                    const community = communityResult.communities.find(
                        c => c.id === commId,
                    );
                    if (community) color = community.color;
                }
            }

            // Calculate size based on node value
            let size: number;
            const val = typeof node.val === 'number' ? node.val : 1;
            switch (node.type) {
                case 'community':
                    size = Math.max(2.5, Math.pow(val, 0.25));
                    break;
                case 'subreddit':
                    size = Math.max(2, Math.pow(val, 0.35));
                    break;
                case 'user':
                    size = Math.max(1.5, Math.pow(val, 0.5));
                    break;
                case 'post':
                    size = 1.4;
                    break;
                case 'comment':
                    size = 1;
                    break;
                default:
                    size = Math.max(1, Math.pow(val, 0.5));
            }

            return {
                id: node.id,
                type: node.type || 'default',
                x: node.x,
                y: node.y,
                z: node.z,
                size,
                color,
            };
        });

        nodeRendererRef.current.setNodeRelSize(nodeRelSize);
        nodeRendererRef.current.setNodeData(nodeData);
    }, [filtered, communityResult, nodeRelSize]);

    // Update labels when label set or filtered data changes
    useEffect(() => {
        if (!labelRendererRef.current || !showLabels || labelSet.size === 0) {
            // Clear labels if not showing
            if (labelRendererRef.current) {
                labelRendererRef.current.setLabels([]);
            }
            return;
        }

        const labelData: LabelData[] = filtered.nodes
            .filter(n => labelSet.has(n.id))
            .map(n => {
                const deg = degreeMap.get(n.id) || 1;
                const base = Math.max(2, Math.pow(deg, 0.35));
                const value = typeof n.val === 'number' ? n.val : 1;
                const visualRadius = (n.type === 'community'
                    ? Math.max(2.5, Math.pow(value, 0.25))
                    : base) * nodeRelSize;
                const size = (6 + Math.min(10, base)) / 8; // Normalize to fontSize multiplier
                
                return {
                    id: n.id,
                    text: `${n.display_name || n.name || n.id}${n.bridge ? ' · BRIDGE' : ''}`,
                    position: {
                        x: n.x || 0,
                        y: (n.y || 0) + visualRadius + 7,
                        z: n.z || 0,
                    },
                    size,
                    priority: n.id === selectedId
                        ? 1000000
                        : n.type === 'community' ? 100000 + value
                        : n.type === 'subreddit' ? 10000 + value
                        : n.type === 'user' ? 1000 + value
                        : value,
                    alwaysVisible: n.id === selectedId,
                };
            })
            .sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0) || a.id.localeCompare(b.id));

        labelRendererRef.current.setLabels(labelData);
    }, [filtered.nodes, labelSet, degreeMap, showLabels, nodeRelSize, selectedId]);

    // Initialize/update force simulation
    useEffect(() => {
        if (!nodeRendererRef.current) return;

        // Revision worlds carry persisted coordinates. They must never be
        // re-simulated client-side: that would destroy scene continuity.
        if (scene?.revision) {
            simulationRef.current?.stop();
            return;
        }
        // Create simulation only for legacy/unpinned fallback data.
        if (!simulationRef.current) {
            simulationRef.current = new ForceSimulation({
                onTick: positions => {
                    if (nodeRendererRef.current) {
                        nodeRendererRef.current.updatePositions(positions);
                    }
                    if (linkRendererRef.current) {
                        linkRendererRef.current.updatePositions(positions);
                        linkRendererRef.current.refresh();
                    }
                    if (labelRendererRef.current && showLabels) {
                        labelRendererRef.current.updatePositions(positions);
                    }
                },
                physics,
                usePrecomputedPositions: usePrecomputedLayout,
            });
        }

        // Set data and start simulation
        simulationRef.current.setData(filtered.nodes, filtered.links);
        simulationRef.current.start();

        return () => {
            if (simulationRef.current) {
                simulationRef.current.stop();
            }
        };
    }, [filtered, physics, usePrecomputedLayout, showLabels, scene?.revision]);

    // Update physics when it changes
    useEffect(() => {
        if (simulationRef.current && physics) {
            simulationRef.current.updatePhysics(physics);
        }
    }, [physics]);

    // Set up links with LinkRenderer
    useEffect(() => {
        if (!linkRendererRef.current) return;

        // Set links data
        linkRendererRef.current.setLinks(filtered.links);

        // Build initial positions map from filtered nodes
        const positions = new Map<
            string,
            { x: number; y: number; z: number }
        >();
        for (const node of filtered.nodes) {
            if (
                node.x !== undefined &&
                node.y !== undefined &&
                node.z !== undefined
            ) {
                positions.set(node.id, { x: node.x, y: node.y, z: node.z });
            }
        }

        linkRendererRef.current.updatePositions(positions);
        linkRendererRef.current.refresh();
    }, [filtered]);

    // Update link opacity
    useEffect(() => {
        if (!linkRendererRef.current) return;
        linkRendererRef.current.setOpacity(linkOpacity);
    }, [linkOpacity]);

    // Update size attenuation
    useEffect(() => {
        if (!nodeRendererRef.current) return;
        nodeRendererRef.current.setSizeAttenuation(sizeAttenuation);
    }, [sizeAttenuation]);

    // Handle mouse interactions
    useEffect(() => {
        if (
            !containerRef.current ||
            !nodeRendererRef.current ||
            !cameraRef.current
        )
            return;

        const container = containerRef.current;
        const raycaster = raycasterRef.current;
        const mouse = mouseRef.current;
        const nodeRenderer = nodeRendererRef.current;
        const camera = cameraRef.current;

        const pickNode = (event: PointerEvent) => {
            const rect = container.getBoundingClientRect();
            mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
            mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

            raycaster.setFromCamera(mouse, camera);
            return nodeRenderer.raycast(raycaster);
        };

        const handlePointerMove = (event: PointerEvent) => {
            const nodeId = pickNode(event);

            if (nodeId !== hoveredNodeRef.current) {
                hoveredNodeRef.current = nodeId;
                container.style.cursor = nodeId ? 'pointer' : 'default';

                // Could trigger tooltip here
                if (nodeId) {
                    const node = filtered.nodes.find(n => n.id === nodeId);
                    if (node) {
                        container.title = node.name || node.id;
                    }
                } else {
                    container.title = '';
                }
            }
        };

        const handlePointerUp = (event: PointerEvent) => {
            const nodeId = pickNode(event);
            if (nodeId && onNodeSelect) {
                hoveredNodeRef.current = nodeId;
                onNodeSelect(nodeId);
            }
        };

        container.addEventListener('pointermove', handlePointerMove);
        container.addEventListener('pointerup', handlePointerUp);

        return () => {
            container.removeEventListener('pointermove', handlePointerMove);
            container.removeEventListener('pointerup', handlePointerUp);
        };
    }, [filtered, onNodeSelect]);

    const handleSceneKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
        if (!filtered.nodes.length) return;
        if (event.key === '[' || event.key === 'ArrowLeft') {
            event.preventDefault();
            keyboardNodeIndexRef.current = (keyboardNodeIndexRef.current - 1 + filtered.nodes.length) % filtered.nodes.length;
            onNodeSelect?.(filtered.nodes[keyboardNodeIndexRef.current].id);
        } else if (event.key === ']' || event.key === 'ArrowRight') {
            event.preventDefault();
            keyboardNodeIndexRef.current = (keyboardNodeIndexRef.current + 1) % filtered.nodes.length;
            onNodeSelect?.(filtered.nodes[keyboardNodeIndexRef.current].id);
        } else if (event.key === 'Enter') {
            event.preventDefault();
            const id = filtered.nodes[keyboardNodeIndexRef.current].id;
            onNodeSelect?.(id);
            onInspectNode?.(id);
        }
    }, [filtered.nodes, onNodeSelect, onInspectNode]);

    // Focus camera on node
    useEffect(() => {
        if (
            !focusNodeId ||
            !cameraRef.current ||
            !controlsRef.current ||
            !nodeRendererRef.current
        )
            return;

        // Try to find node by id or name (case-insensitive)
        const matchedNode = filtered.nodes.find(
            n =>
                n.id === focusNodeId ||
                n.name?.toLowerCase() === focusNodeId.toLowerCase(),
        );

        if (!matchedNode) return;

        const position = nodeRendererRef.current.getNodePosition(
            matchedNode.id,
        );
        if (position) {
            const val = typeof matchedNode.val === 'number' ? Math.max(1, matchedNode.val) : 1;
            const baseRadius = matchedNode.type === 'community' ? Math.max(2.5, Math.pow(val, .25))
                : matchedNode.type === 'subreddit' ? Math.max(2, Math.pow(val, .35))
                : matchedNode.type === 'user' ? Math.max(1.5, Math.pow(val, .5))
                : matchedNode.type === 'post' ? 1.4 : 1;
            const pose = poseForTarget(position, { radius: baseRadius * nodeRelSize, verticalFovDegrees: cameraRef.current.fov });
            cameraRef.current.position.set(pose.x, pose.y, pose.z);
            controlsRef.current.target.set(pose.targetX, pose.targetY, pose.targetZ);
            controlsRef.current.update();
        }
    }, [focusNodeId, filtered, nodeRelSize]);

    // Update scene background color when theme changes
    useEffect(() => {
        if (!sceneRef.current) return;
        sceneRef.current.background = new THREE.Color(theme === 'dark' ? 0x000000 : 0xf8f9fa);
    }, [theme]);

    // Show loading skeleton during initial load
    if (loading && !initialLoadComplete) {
        return <LoadingSkeleton />;
    }

    // Show WebGL warning if not supported
    if (!webglSupported) {
        throw new Error('WebGL is not supported in your browser');
    }

    return (
        <div className='w-full h-screen relative'>
            {error && (
                <div className='absolute top-2 left-2 z-20 bg-red-900/70 text-red-100 rounded px-3 py-2 text-sm max-w-md'>
                    <div className='flex items-start gap-2'>
                        <svg
                            className='w-5 h-5 flex-shrink-0 mt-0.5'
                            fill='none'
                            viewBox='0 0 24 24'
                            stroke='currentColor'
                        >
                            <path
                                strokeLinecap='round'
                                strokeLinejoin='round'
                                strokeWidth={2}
                                d='M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z'
                            />
                        </svg>
                        <div className='flex-1'>
                            <p className='font-medium mb-1'>
                                Error loading graph
                            </p>
                            <p className='text-xs opacity-90'>{error}</p>
                            <button
                                onClick={() => load()}
                                className='mt-2 px-3 py-1 bg-red-700 hover:bg-red-600 rounded text-sm font-medium transition-colors'
                            >
                                Retry
                            </button>
                        </div>
                    </div>
                </div>
            )}
            {loading && initialLoadComplete && (
                <div className='absolute top-2 left-2 z-20 bg-black/50 text-white rounded px-3 py-2 text-sm'>
                    Updating graph…
                </div>
            )}
            <div className='instrument-panel absolute left-3 top-20 z-10 flex items-center gap-2 rounded-full p-1.5 text-[10px] text-white md:left-5 md:top-24'>
                <button
                    className='instrument-button rounded-full px-3 text-[10px]'
                    onClick={() => {
                        onNodeSelect?.(undefined);
                        loadedFocusRef.current = null;
                        lastFramedRevisionRef.current = null;
                        load();
                    }}
                >
                    World view
                </button>
                <label className='instrument-button cursor-pointer rounded-full px-3 text-[10px]'>
                    <input
                        type='checkbox'
                        checked={onlyLinked}
                        onChange={() => setOnlyLinked(v => !v)}
                        className='accent-blue-400'
                    />
                    <span>Linked only</span>
                </label>
            </div>
            <div
                ref={containerRef}
                className='h-full w-full touch-none'
                role='application'
                tabIndex={0}
                data-visible-node-count={filtered.nodes.length}
                data-visible-label-count={labelSet.size}
                data-revision={scene?.revision || 'legacy'}
                aria-label='Interactive community universe. Drag to orbit, scroll to travel, use left and right arrows to move through visible objects, and Enter to inspect.'
                onKeyDown={handleSceneKeyDown}
            />
            <NodeTooltip
                nodeId={hoveredNode?.id || null}
                nodeName={hoveredNode?.name}
                nodeType={hoveredNode?.type}
                mouseX={hoveredNode?.mouseX || 0}
                mouseY={hoveredNode?.mouseY || 0}
            />
            <PerformanceHUD
                renderer={rendererRef.current}
                nodeCount={filtered.nodes.length}
                totalNodeCount={graphData?.nodes.length || 0}
                simulationState={scene?.revision ? 'precomputed' : 'active'}
                lodLevel={currentLODTier}
            />
            <Minimap
                cameraPosition={currentCamera}
                cameraTarget={currentCameraTarget}
                communityResult={communityResult as import('../utils/communityDetection').CommunityResult | null}
                nodes={filtered.nodes}
                onCameraMove={(position) => {
                    if (cameraRef.current && controlsRef.current) {
                        // Smooth camera animation using GSAP-like approach
                        const startPos = {
                            x: cameraRef.current.position.x,
                            y: cameraRef.current.position.y,
                            z: cameraRef.current.position.z,
                        };
                        const duration = prefersReducedMotion() ? 0 : 1000;
                        const startTime = Date.now();

                        const animateCamera = () => {
                            const elapsed = Date.now() - startTime;
                            const progress = duration === 0 ? 1 : Math.min(elapsed / duration, 1);
                            
                            // Ease-out cubic easing
                            const eased = 1 - Math.pow(1 - progress, 3);
                            
                            cameraRef.current!.position.x = startPos.x + (position.x - startPos.x) * eased;
                            cameraRef.current!.position.y = startPos.y + (position.y - startPos.y) * eased;
                            cameraRef.current!.position.z = startPos.z + (position.z - startPos.z) * eased;
                            
                            if (progress < 1) {
                                requestAnimationFrame(animateCamera);
                            } else {
                                // Update controls target and state at the end
                                controlsRef.current!.target.set(position.x, position.y, 0);
                                controlsRef.current!.update();
                            }
                        };

                        animateCamera();
                    }
                }}
            />
        </div>
    );
}
