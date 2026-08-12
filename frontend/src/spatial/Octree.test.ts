import { describe, it, expect, beforeEach } from 'vitest';
import * as THREE from 'three';
import { Octree, AABB, type OctreeItem } from './Octree';

interface TestNode {
    id: string;
    name: string;
}

describe('AABB', () => {
    describe('containsPoint', () => {
        it('should return true for points inside the box', () => {
            const aabb = new AABB(
                new THREE.Vector3(-10, -10, -10),
                new THREE.Vector3(10, 10, 10),
            );

            expect(aabb.containsPoint(new THREE.Vector3(0, 0, 0))).toBe(true);
            expect(aabb.containsPoint(new THREE.Vector3(5, 5, 5))).toBe(true);
            expect(aabb.containsPoint(new THREE.Vector3(-5, -5, -5))).toBe(true);
            expect(aabb.containsPoint(new THREE.Vector3(10, 10, 10))).toBe(true);
        });

        it('should return false for points outside the box', () => {
            const aabb = new AABB(
                new THREE.Vector3(-10, -10, -10),
                new THREE.Vector3(10, 10, 10),
            );

            expect(aabb.containsPoint(new THREE.Vector3(11, 0, 0))).toBe(false);
            expect(aabb.containsPoint(new THREE.Vector3(0, 11, 0))).toBe(false);
            expect(aabb.containsPoint(new THREE.Vector3(0, 0, 11))).toBe(false);
        });
    });

    describe('intersectsAABB', () => {
        it('should detect overlapping boxes', () => {
            const aabb1 = new AABB(
                new THREE.Vector3(0, 0, 0),
                new THREE.Vector3(10, 10, 10),
            );
            const aabb2 = new AABB(
                new THREE.Vector3(5, 5, 5),
                new THREE.Vector3(15, 15, 15),
            );

            expect(aabb1.intersectsAABB(aabb2)).toBe(true);
            expect(aabb2.intersectsAABB(aabb1)).toBe(true);
        });

        it('should detect non-overlapping boxes', () => {
            const aabb1 = new AABB(
                new THREE.Vector3(0, 0, 0),
                new THREE.Vector3(10, 10, 10),
            );
            const aabb2 = new AABB(
                new THREE.Vector3(20, 20, 20),
                new THREE.Vector3(30, 30, 30),
            );

            expect(aabb1.intersectsAABB(aabb2)).toBe(false);
            expect(aabb2.intersectsAABB(aabb1)).toBe(false);
        });
    });

    describe('getCenter', () => {
        it('should return the center point', () => {
            const aabb = new AABB(
                new THREE.Vector3(-10, -20, -30),
                new THREE.Vector3(10, 20, 30),
            );

            const center = aabb.getCenter();
            expect(center.x).toBe(0);
            expect(center.y).toBe(0);
            expect(center.z).toBe(0);
        });
    });

    describe('getSize', () => {
        it('should return the size vector', () => {
            const aabb = new AABB(
                new THREE.Vector3(0, 0, 0),
                new THREE.Vector3(10, 20, 30),
            );

            const size = aabb.getSize();
            expect(size.x).toBe(10);
            expect(size.y).toBe(20);
            expect(size.z).toBe(30);
        });
    });
});

describe('Octree', () => {
    let octree: Octree<TestNode>;

    beforeEach(() => {
        octree = new Octree<TestNode>({
            maxItemsPerNode: 8,
            maxDepth: 8,
            minCellSize: 1.0,
        });
    });

    describe('initialization', () => {
        it('should create an empty octree', () => {
            const stats = octree.getStats();
            expect(stats.totalItems).toBe(0);
            expect(stats.nodeCount).toBe(0);
        });

        it('should accept custom configuration', () => {
            const customOctree = new Octree<TestNode>({
                maxItemsPerNode: 16,
                maxDepth: 10,
            });
            expect(customOctree).toBeDefined();
        });
    });

    describe('build', () => {
        it('should build octree from items', () => {
            const items: OctreeItem<TestNode>[] = [
                {
                    id: '1',
                    position: new THREE.Vector3(0, 0, 0),
                    data: { id: '1', name: 'Node 1' },
                },
                {
                    id: '2',
                    position: new THREE.Vector3(10, 10, 10),
                    data: { id: '2', name: 'Node 2' },
                },
                {
                    id: '3',
                    position: new THREE.Vector3(-10, -10, -10),
                    data: { id: '3', name: 'Node 3' },
                },
            ];

            octree.build(items);

            const stats = octree.getStats();
            expect(stats.totalItems).toBe(3);
            expect(stats.nodeCount).toBeGreaterThan(0);
        });

        it('should handle empty item array', () => {
            octree.build([]);

            const stats = octree.getStats();
            expect(stats.totalItems).toBe(0);
            expect(stats.nodeCount).toBe(0);
        });

        it('should replace existing octree', () => {
            const items1: OctreeItem<TestNode>[] = [
                {
                    id: '1',
                    position: new THREE.Vector3(0, 0, 0),
                    data: { id: '1', name: 'Node 1' },
                },
            ];
            octree.build(items1);
            expect(octree.getStats().totalItems).toBe(1);

            const items2: OctreeItem<TestNode>[] = [
                {
                    id: '2',
                    position: new THREE.Vector3(5, 5, 5),
                    data: { id: '2', name: 'Node 2' },
                },
                {
                    id: '3',
                    position: new THREE.Vector3(10, 10, 10),
                    data: { id: '3', name: 'Node 3' },
                },
            ];
            octree.build(items2);
            expect(octree.getStats().totalItems).toBe(2);
        });
    });

    describe('insert and remove', () => {
        it('should insert single items', () => {
            const item: OctreeItem<TestNode> = {
                id: '1',
                position: new THREE.Vector3(0, 0, 0),
                data: { id: '1', name: 'Node 1' },
            };

            octree.insert(item);

            const stats = octree.getStats();
            expect(stats.totalItems).toBe(1);
        });

        it('should remove items', () => {
            const item: OctreeItem<TestNode> = {
                id: '1',
                position: new THREE.Vector3(0, 0, 0),
                data: { id: '1', name: 'Node 1' },
            };

            octree.insert(item);
            expect(octree.getStats().totalItems).toBe(1);

            const removed = octree.remove('1');
            expect(removed).toBe(true);
            expect(octree.getStats().totalItems).toBe(0);
        });

        it('should return false when removing non-existent item', () => {
            const removed = octree.remove('non-existent');
            expect(removed).toBe(false);
        });

        it('should handle multiple inserts and removes', () => {
            for (let i = 0; i < 10; i++) {
                octree.insert({
                    id: `node${i}`,
                    position: new THREE.Vector3(i * 10, i * 10, i * 10),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }
            expect(octree.getStats().totalItems).toBe(10);

            octree.remove('node5');
            expect(octree.getStats().totalItems).toBe(9);

            octree.remove('node0');
            octree.remove('node9');
            expect(octree.getStats().totalItems).toBe(7);
        });
    });

    describe('subdivision', () => {
        it('should subdivide when maxItemsPerNode is exceeded', () => {
            const smallOctree = new Octree<TestNode>({
                maxItemsPerNode: 4,
                maxDepth: 4,
            });

            // Add items to same region to trigger subdivision
            for (let i = 0; i < 10; i++) {
                smallOctree.insert({
                    id: `node${i}`,
                    position: new THREE.Vector3(i, i, i),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }

            const stats = smallOctree.getStats();
            expect(stats.totalItems).toBe(10);
            expect(stats.maxDepth).toBeGreaterThan(0);
            expect(stats.nodeCount).toBeGreaterThan(1);
        });

        it('should not exceed maxDepth', () => {
            const shallowOctree = new Octree<TestNode>({
                maxItemsPerNode: 2,
                maxDepth: 2,
            });

            for (let i = 0; i < 20; i++) {
                shallowOctree.insert({
                    id: `node${i}`,
                    position: new THREE.Vector3(i, i, i),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }

            const stats = shallowOctree.getStats();
            expect(stats.maxDepth).toBeLessThanOrEqual(2);
        });
    });

    describe('queryFrustum', () => {
        beforeEach(() => {
            // Build a grid of nodes
            const items: OctreeItem<TestNode>[] = [];
            for (let x = -50; x <= 50; x += 10) {
                for (let y = -50; y <= 50; y += 10) {
                    for (let z = -50; z <= 50; z += 10) {
                        items.push({
                            id: `${x}_${y}_${z}`,
                            position: new THREE.Vector3(x, y, z),
                            data: { id: `${x}_${y}_${z}`, name: `Node at ${x},${y},${z}` },
                        });
                    }
                }
            }
            octree.build(items);
        });

        it('should return nodes within frustum', () => {
            const camera = new THREE.PerspectiveCamera(75, 1, 0.1, 1000);
            camera.position.set(0, 0, 100);
            camera.lookAt(0, 0, 0);
            camera.updateMatrixWorld();
            camera.updateProjectionMatrix();

            const frustum = new THREE.Frustum();
            frustum.setFromProjectionMatrix(
                new THREE.Matrix4().multiplyMatrices(
                    camera.projectionMatrix,
                    camera.matrixWorldInverse,
                ),
            );

            const results = octree.queryFrustum(frustum);

            // Should find some nodes
            expect(results.length).toBeGreaterThan(0);

            // All returned nodes should be within frustum or close to it
            expect(results.length).toBeLessThan(octree.getStats().totalItems);
        });

        it('should return empty array for frustum with no nodes', () => {
            const camera = new THREE.PerspectiveCamera(75, 1, 0.1, 1000);
            camera.position.set(1000, 1000, 1000);
            camera.lookAt(1100, 1100, 1100);
            camera.updateMatrixWorld();
            camera.updateProjectionMatrix();

            const frustum = new THREE.Frustum();
            frustum.setFromProjectionMatrix(
                new THREE.Matrix4().multiplyMatrices(
                    camera.projectionMatrix,
                    camera.matrixWorldInverse,
                ),
            );

            const results = octree.queryFrustum(frustum);
            expect(results.length).toBe(0);
        });
    });

    describe('raycast', () => {
        beforeEach(() => {
            // Place nodes in a line
            const items: OctreeItem<TestNode>[] = [];
            for (let i = 0; i < 10; i++) {
                items.push({
                    id: `node${i}`,
                    position: new THREE.Vector3(i * 10, 0, 0),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }
            octree.build(items);
        });

        it('should find nearest node along ray', () => {
            const ray = new THREE.Ray(
                new THREE.Vector3(-10, 0, 0),
                new THREE.Vector3(1, 0, 0),
            );

            const result = octree.raycast(ray);

            expect(result).not.toBeNull();
            expect(result?.id).toBe('node0');
        });

        it('should return null when ray misses all nodes', () => {
            const ray = new THREE.Ray(
                new THREE.Vector3(0, 100, 0),
                new THREE.Vector3(0, 1, 0),
            );

            const result = octree.raycast(ray);
            expect(result).toBeNull();
        });

        it('should respect maxDistance parameter', () => {
            const ray = new THREE.Ray(
                new THREE.Vector3(-100, 0, 0),
                new THREE.Vector3(1, 0, 0),
            );

            // With no max distance, should find node0
            const resultNoLimit = octree.raycast(ray);
            expect(resultNoLimit?.id).toBe('node0');

            // With small max distance (less than distance to nearest node), should find nothing
            // Distance from ray origin to node0 is 100, so maxDistance of 50 should find nothing
            const resultWithLimit = octree.raycast(ray, 50);
            expect(resultWithLimit).toBeNull();
        });
    });

    describe('queryRange', () => {
        beforeEach(() => {
            // Build a grid of nodes
            const items: OctreeItem<TestNode>[] = [];
            for (let x = -50; x <= 50; x += 10) {
                for (let y = -50; y <= 50; y += 10) {
                    items.push({
                        id: `${x}_${y}`,
                        position: new THREE.Vector3(x, y, 0),
                        data: { id: `${x}_${y}`, name: `Node at ${x},${y}` },
                    });
                }
            }
            octree.build(items);
        });

        it('should find nodes within range', () => {
            const center = new THREE.Vector3(0, 0, 0);
            const radius = 15;

            const results = octree.queryRange(center, radius);

            // Should find center and immediate neighbors
            expect(results.length).toBeGreaterThan(0);

            // All results should be within radius
            for (const item of results) {
                const distance = item.position.distanceTo(center);
                expect(distance).toBeLessThanOrEqual(radius);
            }
        });

        it('should return empty array when no nodes in range', () => {
            const center = new THREE.Vector3(1000, 1000, 1000);
            const radius = 10;

            const results = octree.queryRange(center, radius);
            expect(results.length).toBe(0);
        });
    });

    describe('update', () => {
        it('should update item position', () => {
            const item: OctreeItem<TestNode> = {
                id: 'node1',
                position: new THREE.Vector3(0, 0, 0),
                data: { id: 'node1', name: 'Node 1' },
            };

            octree.insert(item);

            const updated = octree.update('node1', new THREE.Vector3(10, 10, 10));
            expect(updated).toBe(true);

            // Verify new position via range query
            const results = octree.queryRange(new THREE.Vector3(10, 10, 10), 1);
            expect(results.length).toBe(1);
            expect(results[0].id).toBe('node1');
        });

        it('should return false for non-existent item', () => {
            const updated = octree.update('non-existent', new THREE.Vector3(0, 0, 0));
            expect(updated).toBe(false);
        });
    });

    describe('performance', () => {
        it('should handle 100k nodes build in reasonable time (<300ms)', () => {
            const items: OctreeItem<TestNode>[] = [];
            const size = 100000;

            // Generate random positions
            for (let i = 0; i < size; i++) {
                items.push({
                    id: `node${i}`,
                    position: new THREE.Vector3(
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                    ),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }

            const start = performance.now();
            octree.build(items);
            const buildTime = performance.now() - start;

            // Relaxed for CI environments - target is <50ms but allow <300ms
            expect(buildTime).toBeLessThan(300);
            expect(octree.getStats().totalItems).toBe(size);
        });

        it('should query frustum without a catastrophic regression for 100k nodes', () => {
            const items: OctreeItem<TestNode>[] = [];
            const size = 100000;

            for (let i = 0; i < size; i++) {
                items.push({
                    id: `node${i}`,
                    position: new THREE.Vector3(
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                    ),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }

            octree.build(items);

            const camera = new THREE.PerspectiveCamera(75, 1, 0.1, 1000);
            camera.position.set(0, 0, 500);
            camera.lookAt(0, 0, 0);
            camera.updateMatrixWorld();
            camera.updateProjectionMatrix();

            const frustum = new THREE.Frustum();
            frustum.setFromProjectionMatrix(
                new THREE.Matrix4().multiplyMatrices(
                    camera.projectionMatrix,
                    camera.matrixWorldInverse,
                ),
            );

            const start = performance.now();
            const results = octree.queryFrustum(frustum);
            const queryTime = performance.now() - start;

            // Wall-clock microbenchmarks are noisy when Vitest runs files in
            // parallel. Keep a broad regression tripwire here; the documented
            // frame-time target is enforced in hardware-accelerated Chromium.
            expect(queryTime).toBeLessThan(75);
            expect(results.length).toBeGreaterThan(0);
        });

        it('should raycast efficiently for 100k nodes (<15ms)', () => {
            const items: OctreeItem<TestNode>[] = [];
            const size = 100000;

            for (let i = 0; i < size; i++) {
                items.push({
                    id: `node${i}`,
                    position: new THREE.Vector3(
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                    ),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }

            octree.build(items);

            const ray = new THREE.Ray(
                new THREE.Vector3(-600, 0, 0),
                new THREE.Vector3(1, 0, 0),
            );

            const start = performance.now();
            octree.raycast(ray);
            const raycastTime = performance.now() - start;

            // Relaxed for CI - target is <1ms but allow <15ms
            expect(raycastTime).toBeLessThan(15);
        });
    });

    describe('memory overhead', () => {
        it('should have <50MB overhead for 100k nodes', () => {
            const items: OctreeItem<TestNode>[] = [];
            const size = 100000;

            for (let i = 0; i < size; i++) {
                items.push({
                    id: `node${i}`,
                    position: new THREE.Vector3(
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                        Math.random() * 1000 - 500,
                    ),
                    data: { id: `node${i}`, name: `Node ${i}` },
                });
            }

            octree.build(items);

            const stats = octree.getStats();

            // Estimate memory:
            // Each node has bounds (2 * Vector3 = 6 floats = 48 bytes)
            // Each node has array overhead (~64 bytes)
            // Total per node: ~112 bytes
            const estimatedMemoryMB = (stats.nodeCount * 112) / (1024 * 1024);

            expect(estimatedMemoryMB).toBeLessThan(50);
        });
    });

    describe('clear', () => {
        it('should clear all items', () => {
            const items: OctreeItem<TestNode>[] = [
                {
                    id: '1',
                    position: new THREE.Vector3(0, 0, 0),
                    data: { id: '1', name: 'Node 1' },
                },
                {
                    id: '2',
                    position: new THREE.Vector3(10, 10, 10),
                    data: { id: '2', name: 'Node 2' },
                },
            ];

            octree.build(items);
            expect(octree.getStats().totalItems).toBe(2);

            octree.clear();
            const stats = octree.getStats();
            expect(stats.totalItems).toBe(0);
            expect(stats.nodeCount).toBe(0);
        });
    });

    describe('getAllItems', () => {
        it('should return all items', () => {
            const items: OctreeItem<TestNode>[] = [
                {
                    id: '1',
                    position: new THREE.Vector3(0, 0, 0),
                    data: { id: '1', name: 'Node 1' },
                },
                {
                    id: '2',
                    position: new THREE.Vector3(10, 10, 10),
                    data: { id: '2', name: 'Node 2' },
                },
                {
                    id: '3',
                    position: new THREE.Vector3(20, 20, 20),
                    data: { id: '3', name: 'Node 3' },
                },
            ];

            octree.build(items);

            const allItems = octree.getAllItems();
            expect(allItems.length).toBe(3);
            expect(allItems.map((item) => item.id).sort()).toEqual(['1', '2', '3']);
        });
    });
});
