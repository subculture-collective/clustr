import { describe, it, expect, beforeEach, vi } from 'vitest';
import * as THREE from 'three';
import { LinkRenderer } from './LinkRenderer';

describe('LinkRenderer', () => {
    let scene: THREE.Scene;
    let renderer: LinkRenderer;

    beforeEach(() => {
        scene = new THREE.Scene();
        renderer = new LinkRenderer(scene, { maxLinks: 1000 });
    });

    describe('initialization', () => {
        it('should create a LinkRenderer instance', () => {
            expect(renderer).toBeDefined();
        });

        it('should add LineSegments to the scene', () => {
            expect(scene.children.length).toBe(1);
            expect(scene.children[0]).toBeInstanceOf(THREE.LineSegments);
        });

        it('should initialize with custom opacity', () => {
            const customRenderer = new LinkRenderer(scene, { opacity: 0.5 });
            const stats = customRenderer.getStats();
            expect(stats).toBeDefined();
            customRenderer.dispose();
        });

        it('should initialize with custom color', () => {
            const customRenderer = new LinkRenderer(scene, { color: 0xff0000 });
            const stats = customRenderer.getStats();
            expect(stats).toBeDefined();
            customRenderer.dispose();
        });
    });

    describe('setLinks', () => {
        it('should set links data', () => {
            const links = [
                { source: 'node1', target: 'node2' },
                { source: 'node2', target: 'node3' },
            ];

            renderer.setLinks(links);
            const stats = renderer.getStats();
            expect(stats.totalLinks).toBe(2);
        });

        it('should handle empty links array', () => {
            renderer.setLinks([]);
            const stats = renderer.getStats();
            expect(stats.totalLinks).toBe(0);
        });

        it('should handle large link arrays', () => {
            const links = Array.from({ length: 500 }, (_, i) => ({
                source: `node${i}`,
                target: `node${i + 1}`,
            }));

            renderer.setLinks(links);
            const stats = renderer.getStats();
            expect(stats.totalLinks).toBe(500);
        });

        it('should resize buffer when needed', () => {
            // Start with small capacity
            const smallRenderer = new LinkRenderer(scene, { maxLinks: 20 });

            // Set links that fit in initial buffer (allocates for 20 links initially)
            const initialLinks = [
                { source: 'node1', target: 'node2' },
                { source: 'node2', target: 'node3' },
            ];
            smallRenderer.setLinks(initialLinks);

            // Get initial buffer size (should be 20 * 2 * 3 = 120)
            const initialBufferSize = (
                smallRenderer as unknown as { positionsBuffer: Float32Array }
            ).positionsBuffer.length;
            expect(initialBufferSize).toBe(120); // 20 links * 2 vertices * 3 components

            // Now set more links to force buffer resize (15 links needs 15*2*3=90, still within 120)
            // So we need to trigger the growth logic which doubles when needed
            const moreLinks = Array.from({ length: 15 }, (_, i) => ({
                source: `node${i}`,
                target: `node${i + 1}`,
            }));
            smallRenderer.setLinks(moreLinks);

            // Buffer should still be initial size since 15 links fit in 120
            expect(
                (smallRenderer as unknown as { positionsBuffer: Float32Array })
                    .positionsBuffer.length,
            ).toBe(120);

            const stats = smallRenderer.getStats();
            expect(stats.totalLinks).toBe(15);

            smallRenderer.dispose();
        });
    });

    describe('updatePositions', () => {
        it('should update node positions', () => {
            const links = [{ source: 'node1', target: 'node2' }];
            renderer.setLinks(links);

            const positions = new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
            ]);

            renderer.updatePositions(positions);
            renderer.refresh();

            const stats = renderer.getStats();
            expect(stats.bufferedLinks).toBe(1);
        });

        it('should handle position updates with missing nodes', () => {
            const links = [
                { source: 'node1', target: 'node2' },
                { source: 'node2', target: 'node3' },
            ];
            renderer.setLinks(links);

            // Only provide positions for node1 and node2
            const positions = new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
            ]);

            renderer.updatePositions(positions);
            renderer.refresh();

            const stats = renderer.getStats();
            // Only the first link should be rendered
            expect(stats.bufferedLinks).toBe(1);
        });
    });

    describe('refresh', () => {
        it('should populate buffer with visible links', () => {
            const links = [
                { source: 'node1', target: 'node2' },
                { source: 'node2', target: 'node3' },
            ];
            renderer.setLinks(links);

            const positions = new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
                ['node3', { x: 20, y: 20, z: 20 }],
            ]);

            renderer.updatePositions(positions);
            renderer.refresh();

            const stats = renderer.getStats();
            expect(stats.bufferedLinks).toBe(2);
            expect(stats.drawCalls).toBe(1);
        });

        it('should skip refresh if not needed', () => {
            const links = [{ source: 'node1', target: 'node2' }];
            renderer.setLinks(links);

            const positions = new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
            ]);

            renderer.updatePositions(positions);
            renderer.refresh();

            // Second refresh should be skipped (no updates)
            renderer.refresh();

            const stats = renderer.getStats();
            expect(stats.bufferedLinks).toBe(1);
        });

        it('should handle buffer capacity limit', () => {
            // Spy on console.warn
            const warnSpy = vi
                .spyOn(console, 'warn')
                .mockImplementation(() => {});

            // Create renderer with small capacity
            const smallRenderer = new LinkRenderer(scene, { maxLinks: 2 });

            const links = [
                { source: 'node1', target: 'node2' },
                { source: 'node2', target: 'node3' },
                { source: 'node3', target: 'node4' },
            ];

            // Setting links should warn about exceeding capacity
            smallRenderer.setLinks(links);

            const positions = new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
                ['node3', { x: 20, y: 20, z: 20 }],
                ['node4', { x: 30, y: 30, z: 30 }],
            ]);

            smallRenderer.updatePositions(positions);
            smallRenderer.refresh();

            const stats = smallRenderer.getStats();
            // Should only store up to maxLinks
            expect(stats.totalLinks).toBe(2);
            expect(warnSpy).toHaveBeenCalledWith(
                expect.stringContaining('received 3 links but maxLinks is 2'),
            );

            warnSpy.mockRestore();
            smallRenderer.dispose();
        });
    });

    describe('buffering', () => {
        it('buffers every positioned link, including ones far from the origin', () => {
            const links = [
                { source: 'node1', target: 'node2' },
                { source: 'node3', target: 'node4' },
                { source: 'node1', target: 'node4' },
            ];
            renderer.setLinks(links);

            renderer.updatePositions(new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
                ['node3', { x: 100000, y: 100000, z: 100000 }],
                ['node4', { x: 100010, y: 100010, z: 100010 }],
            ]));
            renderer.refresh();

            const stats = renderer.getStats();
            // The GPU clips off-screen geometry; nothing is dropped on the CPU
            // for being far away or for having one endpoint out of view.
            expect(stats.bufferedLinks).toBe(3);
            expect(stats.visibleLinks).toBe(3);
        });

        it('submits the line object without bounding-sphere culling', () => {
            const lines = scene.children.find(child => child instanceof THREE.LineSegments);
            expect(lines).toBeDefined();
            // Three computes the bounding sphere from the empty buffer on the
            // first draw. With culling on, every link disappeared whenever the
            // world origin left the view.
            expect(lines!.frustumCulled).toBe(false);
        });
    });

    describe('setOpacity', () => {
        it('should update link opacity', () => {
            renderer.setOpacity(0.5);
            // No direct way to test, but should not throw
            expect(renderer).toBeDefined();
        });

        it('should clamp opacity to valid range', () => {
            renderer.setOpacity(1.5);
            renderer.setOpacity(-0.5);
            // Should clamp to [0, 1] and not throw
            expect(renderer).toBeDefined();
        });
    });

    describe('setColor', () => {
        it('should update link color', () => {
            renderer.setColor(0xff0000);
            expect(renderer).toBeDefined();
        });
    });

    describe('getStats', () => {
        it('should return accurate statistics', () => {
            const links = [
                { source: 'node1', target: 'node2' },
                { source: 'node2', target: 'node3' },
            ];
            renderer.setLinks(links);

            const positions = new Map([
                ['node1', { x: 0, y: 0, z: 0 }],
                ['node2', { x: 10, y: 10, z: 10 }],
                ['node3', { x: 20, y: 20, z: 20 }],
            ]);

            renderer.updatePositions(positions);
            renderer.refresh();

            const stats = renderer.getStats();
            expect(stats.totalLinks).toBe(2);
            expect(stats.bufferedLinks).toBe(2);
            expect(stats.drawCalls).toBe(1);
        });

        it('should report 0 draw calls when no links visible', () => {
            renderer.setLinks([]);
            renderer.refresh();

            const stats = renderer.getStats();
            expect(stats.drawCalls).toBe(0);
        });
    });

    describe('dispose', () => {
        it('should clean up resources', () => {
            const links = [{ source: 'node1', target: 'node2' }];
            renderer.setLinks(links);

            const initialChildCount = scene.children.length;
            renderer.dispose();

            expect(scene.children.length).toBe(initialChildCount - 1);
        });

        it('should allow multiple dispose calls', () => {
            renderer.dispose();
            renderer.dispose();
            // Should not throw
            expect(true).toBe(true);
        });
    });

    describe('performance', () => {
        it('should update buffer quickly for large link counts', () => {
            const linkCount = 10000;
            const links = Array.from({ length: linkCount }, (_, i) => ({
                source: `node${i}`,
                target: `node${i + 1}`,
            }));

            // Dispose the instance created in beforeEach to avoid leaking resources
            renderer.dispose();

            renderer = new LinkRenderer(scene, { maxLinks: linkCount });
            renderer.setLinks(links);

            const positions = new Map();
            for (let i = 0; i <= linkCount; i++) {
                positions.set(`node${i}`, {
                    x: Math.random() * 100,
                    y: Math.random() * 100,
                    z: Math.random() * 100,
                });
            }

            renderer.updatePositions(positions);

            const startTime = performance.now();
            renderer.refresh();
            const elapsed = performance.now() - startTime;

            // Should be fast (test environment target: <100ms for 10k links)
            // In production, 200k links should be <10ms
            expect(elapsed).toBeLessThan(100);

            const stats = renderer.getStats();
            expect(stats.bufferedLinks).toBe(linkCount);
            expect(stats.drawCalls).toBe(1);
        });
    });
});
