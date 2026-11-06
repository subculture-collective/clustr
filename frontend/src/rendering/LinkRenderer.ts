import * as THREE from 'three';

/**
 * LinkRenderer - High-performance link rendering using THREE.LineSegments
 *
 * Renders all visible links in a single draw call using GPU-accelerated LineSegments.
 * This dramatically reduces draw calls and improves performance for large graphs.
 *
 * Key features:
 * - Single LineSegments draw call for all links
 * - Viewport-based link filtering (frustum culling)
 * - Pre-allocated Float32Array buffer for efficient updates
 * - Dynamic buffer resizing as needed
 * - Opacity control via material uniform
 *
 * Performance targets:
 * - 1 draw call for all links
 * - <10ms buffer update for 200k links
 * - Links hidden when endpoints are off-screen
 *
 * @example
 * ```typescript
 * const linkRenderer = new LinkRenderer(scene, {
 *   maxLinks: 200000,
 *   opacity: 0.3
 * });
 *
 * // Set link data
 * linkRenderer.setLinks(links);
 *
 * // Update when node positions change
 * linkRenderer.updatePositions(nodePositions);
 *
 * // Update when camera moves (for frustum culling)
 * linkRenderer.updateVisibility(camera);
 *
 * // Update opacity
 * linkRenderer.setOpacity(0.5);
 * ```
 */

export interface LinkData {
    source: string;
    target: string;
}

export interface LinkRendererConfig {
    maxLinks?: number;
    opacity?: number;
    color?: number;
}

export class LinkRenderer {
    private scene: THREE.Scene;
    private geometry: THREE.BufferGeometry;
    private material: THREE.LineBasicMaterial;
    private lineSegments: THREE.LineSegments | null = null;
    private links: LinkData[] = [];
    private nodePositions: Map<string, { x: number; y: number; z: number }> =
        new Map();
    private positionsBuffer: Float32Array;
    private maxLinks: number;
    private needsUpdate = false;
    private lastPerformanceWarning = 0; // Timestamp of last performance warning
    private static readonly PERFORMANCE_WARNING_THROTTLE = 5000; // Warn at most once per 5 seconds

    constructor(scene: THREE.Scene, config: LinkRendererConfig = {}) {
        this.scene = scene;
        this.maxLinks = config.maxLinks || 200000;

        // Pre-allocate buffer for link positions (2 vertices per link × 3 components per vertex)
        this.positionsBuffer = new Float32Array(this.maxLinks * 2 * 3);

        // Create geometry with position attribute
        this.geometry = new THREE.BufferGeometry();
        this.geometry.setAttribute(
            'position',
            new THREE.BufferAttribute(this.positionsBuffer, 3).setUsage(
                THREE.DynamicDrawUsage,
            ),
        );
        this.geometry.setDrawRange(0, 0); // Start with no links visible

        // Create material
        this.material = new THREE.LineBasicMaterial({
            color: config.color !== undefined ? config.color : 0x999999,
            opacity: config.opacity !== undefined ? config.opacity : 0.3,
            transparent: true,
            depthTest: true,
            depthWrite: false,
        });

        // Create LineSegments. The buffer is filled once per scene and the GPU
        // clips it, so there is no per-camera work. Three would otherwise cull
        // the whole object by a bounding sphere computed from the empty buffer
        // on the first frame, which hid every link once the origin left the view.
        this.lineSegments = new THREE.LineSegments(
            this.geometry,
            this.material,
        );
        this.lineSegments.frustumCulled = false;
        this.scene.add(this.lineSegments);
    }

    /**
     * Set the links to be rendered
     */
    public setLinks(links: LinkData[]): void {
        const maxRenderableLinks = this.maxLinks;

        // Clamp to maxLinks to avoid storing more links than the buffer can represent
        let clampedLinks = links;
        if (links.length > maxRenderableLinks) {
            console.warn(
                `LinkRenderer: received ${links.length} links but maxLinks is ${maxRenderableLinks}. ` +
                    `Only the first ${maxRenderableLinks} links will be rendered.`,
            );
            clampedLinks = links.slice(0, maxRenderableLinks);
        }

        this.links = clampedLinks;

        // Resize buffer if necessary
        const requiredSize = clampedLinks.length * 2 * 3;
        const maxBufferSize = this.maxLinks * 2 * 3;

        if (
            requiredSize > this.positionsBuffer.length &&
            this.positionsBuffer.length < maxBufferSize
        ) {
            const candidateSize = Math.max(
                requiredSize,
                this.positionsBuffer.length * 2,
            );
            const newSize = Math.min(candidateSize, maxBufferSize);

            // Avoid reallocating if we cannot actually increase capacity
            if (newSize > this.positionsBuffer.length) {
                this.positionsBuffer = new Float32Array(newSize);
                this.geometry.setAttribute(
                    'position',
                    new THREE.BufferAttribute(this.positionsBuffer, 3).setUsage(
                        THREE.DynamicDrawUsage,
                    ),
                );
            }
        }

        this.needsUpdate = true;
    }

    /**
     * Update node positions and refresh link endpoints
     */
    public updatePositions(
        positions: Map<string, { x: number; y: number; z: number }>,
    ): void {
        this.nodePositions = positions;
        this.needsUpdate = true;
    }

    /**
     * Refresh the link geometry buffer
     * Call this after setting links or updating positions. Every link whose
     * endpoints have positions is written; a link that leaves the view is
     * clipped by the GPU rather than rebuilt on the CPU for each camera move,
     * and a link with one endpoint off screen still shows the part in view.
     */
    public refresh(): void {
        if (!this.needsUpdate) {
            return;
        }

        const startTime = performance.now();

        let vertexCount = 0;

        for (const link of this.links) {
            const sourcePos = this.nodePositions.get(link.source);
            const targetPos = this.nodePositions.get(link.target);

            // Skip if either node doesn't have a position
            if (!sourcePos || !targetPos) {
                continue;
            }

            // Check if we've exceeded buffer capacity
            if (vertexCount * 3 >= this.positionsBuffer.length) {
                // Buffer capacity exceeded - this should not happen since setLinks() clamps to maxLinks
                break;
            }

            const baseIndex = vertexCount * 3;

            // Source vertex
            this.positionsBuffer[baseIndex] = sourcePos.x;
            this.positionsBuffer[baseIndex + 1] = sourcePos.y;
            this.positionsBuffer[baseIndex + 2] = sourcePos.z;

            // Target vertex
            this.positionsBuffer[baseIndex + 3] = targetPos.x;
            this.positionsBuffer[baseIndex + 4] = targetPos.y;
            this.positionsBuffer[baseIndex + 5] = targetPos.z;

            vertexCount += 2;
        }

        // Update draw range to only render the populated vertices
        this.geometry.setDrawRange(0, vertexCount);
        this.geometry.attributes.position.needsUpdate = true;

        this.needsUpdate = false;

        const elapsed = performance.now() - startTime;
        if (elapsed > 10) {
            // Throttle warnings to avoid console spam (max once per 5 seconds)
            const now = Date.now();
            if (
                now - this.lastPerformanceWarning >
                LinkRenderer.PERFORMANCE_WARNING_THROTTLE
            ) {
                console.warn(
                    `LinkRenderer: Buffer update took ${elapsed.toFixed(2)}ms for ${vertexCount / 2} links (target <10ms)`,
                );
                this.lastPerformanceWarning = now;
            }
        }
    }

    /**
     * Set the opacity of all links
     */
    public setOpacity(opacity: number): void {
        this.material.opacity = Math.max(0, Math.min(1, opacity));
    }

    /**
     * Set the color of all links
     */
    public setColor(color: number): void {
        this.material.color.setHex(color);
    }

    /**
     * Get rendering statistics
     */
    public getStats(): {
        totalLinks: number;
        visibleLinks: number;
        bufferedLinks: number;
        drawCalls: number;
    } {
        const drawRange = this.geometry.drawRange;
        return {
            totalLinks: this.links.length,
            // Every buffered link is submitted; the GPU clips what is off screen.
            visibleLinks: drawRange.count / 2,
            bufferedLinks: drawRange.count / 2,
            drawCalls: drawRange.count > 0 ? 1 : 0,
        };
    }

    /**
     * Dispose of all resources
     */
    public dispose(): void {
        if (this.lineSegments) {
            this.scene.remove(this.lineSegments);
        }
        this.geometry.dispose();
        this.material.dispose();
        this.links = [];
        this.nodePositions.clear();
    }
}
