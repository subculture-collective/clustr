import * as THREE from 'three';

/**
 * Axis-Aligned Bounding Box for spatial queries
 */
export class AABB {
    public min: THREE.Vector3;
    public max: THREE.Vector3;
    private _box: THREE.Box3; // Reusable box for intersection tests

    constructor(min: THREE.Vector3, max: THREE.Vector3) {
        this.min = min.clone();
        this.max = max.clone();
        this._box = new THREE.Box3(this.min, this.max);
    }

    /**
     * Check if this AABB contains a point
     */
    containsPoint(point: THREE.Vector3): boolean {
        return (
            point.x >= this.min.x &&
            point.x <= this.max.x &&
            point.y >= this.min.y &&
            point.y <= this.max.y &&
            point.z >= this.min.z &&
            point.z <= this.max.z
        );
    }

    /**
     * Check if this AABB intersects with another AABB
     */
    intersectsAABB(other: AABB): boolean {
        return (
            this.min.x <= other.max.x &&
            this.max.x >= other.min.x &&
            this.min.y <= other.max.y &&
            this.max.y >= other.min.y &&
            this.min.z <= other.max.z &&
            this.max.z >= other.min.z
        );
    }

    /**
     * Check if this AABB intersects with a THREE.Frustum
     */
    intersectsFrustum(frustum: THREE.Frustum): boolean {
        // Update reusable box and use THREE's Box3 for frustum intersection
        this._box.min.copy(this.min);
        this._box.max.copy(this.max);
        return frustum.intersectsBox(this._box);
    }

    /**
     * Check if a ray intersects this AABB
     * Returns distance to intersection or Infinity if no hit
     */
    intersectsRay(ray: THREE.Ray, target: THREE.Vector3): number {
        // Update reusable box
        this._box.min.copy(this.min);
        this._box.max.copy(this.max);
        const result = ray.intersectBox(this._box, target);
        return result ? ray.origin.distanceTo(result) : Infinity;
    }

    /**
     * Get center point of AABB
     */
    getCenter(): THREE.Vector3 {
        return new THREE.Vector3(
            (this.min.x + this.max.x) * 0.5,
            (this.min.y + this.max.y) * 0.5,
            (this.min.z + this.max.z) * 0.5,
        );
    }

    /**
     * Get size of AABB
     */
    getSize(): THREE.Vector3 {
        return new THREE.Vector3(
            this.max.x - this.min.x,
            this.max.y - this.min.y,
            this.max.z - this.min.z,
        );
    }

    /**
     * Check if point is within distance of AABB
     */
    distanceToPoint(point: THREE.Vector3): number {
        this._box.min.copy(this.min);
        this._box.max.copy(this.max);
        return this._box.distanceToPoint(point);
    }
}

/**
 * Item stored in the octree with spatial position
 */
export interface OctreeItem<T> {
    id: string;
    position: THREE.Vector3;
    data: T;
}

/**
 * Configuration for octree behavior
 */
export interface OctreeConfig {
    /** Maximum items per node before subdivision (default: 8) */
    maxItemsPerNode?: number;
    /** Maximum depth of the tree (default: 8) */
    maxDepth?: number;
    /** Minimum cell size to prevent infinite subdivision (default: 1.0) */
    minCellSize?: number;
}

/**
 * Internal octree node
 */
class OctreeNode<T> {
    public bounds: AABB;
    public items: OctreeItem<T>[] = [];
    public children: OctreeNode<T>[] | null = null;
    public depth: number;

    constructor(bounds: AABB, depth: number) {
        this.bounds = bounds;
        this.depth = depth;
    }

    /**
     * Check if this node is a leaf (has no children)
     */
    isLeaf(): boolean {
        return this.children === null;
    }

    /**
     * Subdivide this node into 8 octants
     */
    subdivide(): void {
        if (!this.isLeaf()) return;

        const center = this.bounds.getCenter();
        const { min, max } = this.bounds;

        this.children = [
            // Bottom four octants
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(min.x, min.y, min.z),
                    new THREE.Vector3(center.x, center.y, center.z),
                ),
                this.depth + 1,
            ),
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(center.x, min.y, min.z),
                    new THREE.Vector3(max.x, center.y, center.z),
                ),
                this.depth + 1,
            ),
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(min.x, min.y, center.z),
                    new THREE.Vector3(center.x, center.y, max.z),
                ),
                this.depth + 1,
            ),
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(center.x, min.y, center.z),
                    new THREE.Vector3(max.x, center.y, max.z),
                ),
                this.depth + 1,
            ),
            // Top four octants
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(min.x, center.y, min.z),
                    new THREE.Vector3(center.x, max.y, center.z),
                ),
                this.depth + 1,
            ),
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(center.x, center.y, min.z),
                    new THREE.Vector3(max.x, max.y, center.z),
                ),
                this.depth + 1,
            ),
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(min.x, center.y, center.z),
                    new THREE.Vector3(center.x, max.y, max.z),
                ),
                this.depth + 1,
            ),
            new OctreeNode<T>(
                new AABB(
                    new THREE.Vector3(center.x, center.y, center.z),
                    new THREE.Vector3(max.x, max.y, max.z),
                ),
                this.depth + 1,
            ),
        ];

        // Redistribute items to children
        const itemsToRedistribute = this.items;
        this.items = [];

        for (const item of itemsToRedistribute) {
            for (const child of this.children) {
                if (child.bounds.containsPoint(item.position)) {
                    child.items.push(item);
                    break;
                }
            }
        }
    }
}

/**
 * Octree spatial index for efficient 3D spatial queries
 *
 * Provides O(log n) performance for:
 * - Frustum culling queries
 * - Nearest-neighbor raycasting
 * - Range/viewport queries
 *
 * Performance targets for 100k nodes:
 * - Frustum query: <2ms
 * - Ray intersection: <1ms
 * - Rebuild: <50ms
 * - Memory overhead: <50MB
 *
 * @example
 * ```typescript
 * const octree = new Octree<NodeData>({
 *   maxItemsPerNode: 8,
 *   maxDepth: 8
 * });
 *
 * // Build from nodes
 * octree.build(nodes.map(n => ({
 *   id: n.id,
 *   position: new THREE.Vector3(n.x, n.y, n.z),
 *   data: n
 * })));
 *
 * // Frustum query
 * const visibleNodes = octree.queryFrustum(camera.frustum);
 *
 * // Ray intersection for hover
 * const nearest = octree.raycast(ray, maxDistance);
 * ```
 */
export class Octree<T> {
    private root: OctreeNode<T> | null = null;
    private config: Required<OctreeConfig>;
    private itemMap: Map<string, OctreeItem<T>> = new Map();
    private totalItems = 0;

    constructor(config: OctreeConfig = {}) {
        this.config = {
            maxItemsPerNode: config.maxItemsPerNode ?? 8,
            maxDepth: config.maxDepth ?? 8,
            minCellSize: config.minCellSize ?? 1.0,
        };
    }

    /**
     * Build octree from a collection of items
     * Replaces any existing octree structure
     */
    public build(items: OctreeItem<T>[]): void {
        this.clear();

        if (items.length === 0) return;

        // Calculate bounds from all items
        const bounds = this.calculateBounds(items);
        this.root = new OctreeNode<T>(bounds, 0);

        // Insert all items
        for (const item of items) {
            this.insertIntoNode(this.root, item);
            this.itemMap.set(item.id, item);
        }

        this.totalItems = items.length;
    }

    /**
     * Insert a single item into the octree
     */
    public insert(item: OctreeItem<T>): void {
        if (!this.root) {
            // Create root from first item
            const padding = 100;
            const bounds = new AABB(
                new THREE.Vector3(
                    item.position.x - padding,
                    item.position.y - padding,
                    item.position.z - padding,
                ),
                new THREE.Vector3(
                    item.position.x + padding,
                    item.position.y + padding,
                    item.position.z + padding,
                ),
            );
            this.root = new OctreeNode<T>(bounds, 0);
        }

        // Check if item is within root bounds, expand if necessary
        if (!this.root.bounds.containsPoint(item.position)) {
            this.expandRoot(item.position);
        }

        this.insertIntoNode(this.root, item);
        this.itemMap.set(item.id, item);
        this.totalItems++;
    }

    /**
     * Remove an item from the octree
     */
    public remove(itemId: string): boolean {
        const item = this.itemMap.get(itemId);
        if (!item || !this.root) return false;

        const removed = this.removeFromNode(this.root, itemId);
        if (removed) {
            this.itemMap.delete(itemId);
            this.totalItems--;
        }

        return removed;
    }

    /**
     * Update an item's position
     * More efficient than remove + insert for small movements
     */
    public update(itemId: string, newPosition: THREE.Vector3): boolean {
        const item = this.itemMap.get(itemId);
        if (!item) return false;

        // Update position
        item.position.copy(newPosition);

        // For simplicity, remove and re-insert
        // Could be optimized to check if still in same cell
        this.remove(itemId);
        this.insert(item);

        return true;
    }

    /**
     * Query items within camera frustum
     * Returns items that are potentially visible
     */
    public queryFrustum(frustum: THREE.Frustum): OctreeItem<T>[] {
        if (!this.root) return [];

        const results: OctreeItem<T>[] = [];
        this.queryFrustumRecursive(this.root, frustum, results);
        return results;
    }

    /**
     * Find nearest item to a ray (for raycasting/hover)
     * @param ray The ray to test
     * @param maxDistance Maximum distance to consider
     * @returns Nearest item or null
     */
    public raycast(
        ray: THREE.Ray,
        maxDistance: number = Infinity,
    ): OctreeItem<T> | null {
        if (!this.root) return null;

        let nearest: OctreeItem<T> | null = null;
        let nearestDistance = maxDistance;
        
        // Reusable vector for intersection tests
        const target = new THREE.Vector3();

        this.raycastRecursive(this.root, ray, target, nearestDistance, (item, distance) => {
            if (distance < nearestDistance) {
                nearest = item;
                nearestDistance = distance;
            }
            // Return updated nearest distance for pruning
            return nearestDistance;
        });

        return nearest;
    }

    /**
     * Query items within a range (sphere)
     * @param center Center point
     * @param radius Search radius
     */
    public queryRange(
        center: THREE.Vector3,
        radius: number,
    ): OctreeItem<T>[] {
        if (!this.root) return [];

        const results: OctreeItem<T>[] = [];
        const radiusSquared = radius * radius;

        this.queryRangeRecursive(
            this.root,
            center,
            radiusSquared,
            results,
        );

        return results;
    }

    /**
     * Get all items in the octree
     */
    public getAllItems(): OctreeItem<T>[] {
        return Array.from(this.itemMap.values());
    }

    /**
     * Clear the octree
     */
    public clear(): void {
        this.root = null;
        this.itemMap.clear();
        this.totalItems = 0;
    }

    /**
     * Get statistics about the octree
     */
    public getStats(): {
        totalItems: number;
        maxDepth: number;
        nodeCount: number;
        leafCount: number;
    } {
        if (!this.root) {
            return { totalItems: 0, maxDepth: 0, nodeCount: 0, leafCount: 0 };
        }

        let maxDepth = 0;
        let nodeCount = 0;
        let leafCount = 0;

        const traverse = (node: OctreeNode<T>) => {
            nodeCount++;
            maxDepth = Math.max(maxDepth, node.depth);

            if (node.isLeaf()) {
                leafCount++;
            } else if (node.children) {
                for (const child of node.children) {
                    traverse(child);
                }
            }
        };

        traverse(this.root);

        return {
            totalItems: this.totalItems,
            maxDepth,
            nodeCount,
            leafCount,
        };
    }

    // Private helper methods

    private insertIntoNode(node: OctreeNode<T>, item: OctreeItem<T>): void {
        // If node is a leaf and not at max depth, check if we need to subdivide
        if (node.isLeaf()) {
            node.items.push(item);

            // Check if we should subdivide
            const shouldSubdivide =
                node.items.length > this.config.maxItemsPerNode &&
                node.depth < this.config.maxDepth &&
                node.bounds.getSize().length() > this.config.minCellSize;

            if (shouldSubdivide) {
                node.subdivide();
            }
        } else if (node.children) {
            // Find appropriate child and insert
            for (const child of node.children) {
                if (child.bounds.containsPoint(item.position)) {
                    this.insertIntoNode(child, item);
                    return;
                }
            }
            // If no child contains it (shouldn't happen), add to this node
            node.items.push(item);
        }
    }

    private removeFromNode(
        node: OctreeNode<T>,
        itemId: string,
    ): boolean {
        // Check this node's items
        const index = node.items.findIndex((item) => item.id === itemId);
        if (index !== -1) {
            node.items.splice(index, 1);
            return true;
        }

        // Check children
        if (node.children) {
            for (const child of node.children) {
                if (this.removeFromNode(child, itemId)) {
                    return true;
                }
            }
        }

        return false;
    }

    private queryFrustumRecursive(
        node: OctreeNode<T>,
        frustum: THREE.Frustum,
        results: OctreeItem<T>[],
    ): void {
        // Check if node bounds intersect frustum
        if (!node.bounds.intersectsFrustum(frustum)) {
            return;
        }

        // Add items from this node
        for (const item of node.items) {
            results.push(item);
        }

        // Recurse into children
        if (node.children) {
            for (const child of node.children) {
                this.queryFrustumRecursive(child, frustum, results);
            }
        }
    }

    private raycastRecursive(
        node: OctreeNode<T>,
        ray: THREE.Ray,
        target: THREE.Vector3,
        maxDistance: number,
        callback: (item: OctreeItem<T>, distance: number) => number,
    ): number {
        // Check if ray intersects node bounds
        const distance = node.bounds.intersectsRay(ray, target);
        if (distance === Infinity || distance > maxDistance) {
            return maxDistance;
        }

        let currentMaxDistance = maxDistance;

        // Check items in this node - compute actual ray distance
        for (const item of node.items) {
            // Project point onto ray to get distance along ray
            const directionDot = ray.direction.dot(
                target.subVectors(item.position, ray.origin)
            );
            
            if (directionDot > 0 && directionDot <= currentMaxDistance) {
                // Get closest point on ray
                const closestPoint = ray.origin.clone().addScaledVector(
                    ray.direction,
                    directionDot
                );
                
                // Check perpendicular distance (as a pick radius)
                const perpDistance = closestPoint.distanceTo(item.position);
                
                // Use a small pick radius based on typical node sizes
                const pickRadius = 10; // Adjust based on your node sizes
                
                if (perpDistance <= pickRadius) {
                    // Use distance along ray for ordering
                    currentMaxDistance = callback(item, directionDot);
                }
            }
        }

        // Recurse into children, sorted by distance for early exit optimization
        if (node.children) {
            // Calculate distances to children for sorting
            const childDistances = node.children.map((child) => ({
                child,
                distance: child.bounds.intersectsRay(ray, target),
            }));

            // Sort by distance (closest first)
            childDistances.sort((a, b) => a.distance - b.distance);

            for (const { child, distance } of childDistances) {
                if (distance !== Infinity && distance <= currentMaxDistance) {
                    currentMaxDistance = this.raycastRecursive(
                        child,
                        ray,
                        target,
                        currentMaxDistance,
                        callback
                    );
                }
            }
        }

        return currentMaxDistance;
    }

    private queryRangeRecursive(
        node: OctreeNode<T>,
        center: THREE.Vector3,
        radiusSquared: number,
        results: OctreeItem<T>[],
    ): void {
        // Check if node bounds could contain items in range
        if (node.bounds.distanceToPoint(center) > Math.sqrt(radiusSquared)) {
            return;
        }

        // Check items in this node
        for (const item of node.items) {
            const distSquared = item.position.distanceToSquared(center);
            if (distSquared <= radiusSquared) {
                results.push(item);
            }
        }

        // Recurse into children
        if (node.children) {
            for (const child of node.children) {
                this.queryRangeRecursive(child, center, radiusSquared, results);
            }
        }
    }

    private calculateBounds(items: OctreeItem<T>[]): AABB {
        const min = new THREE.Vector3(Infinity, Infinity, Infinity);
        const max = new THREE.Vector3(-Infinity, -Infinity, -Infinity);

        for (const item of items) {
            min.x = Math.min(min.x, item.position.x);
            min.y = Math.min(min.y, item.position.y);
            min.z = Math.min(min.z, item.position.z);
            max.x = Math.max(max.x, item.position.x);
            max.y = Math.max(max.y, item.position.y);
            max.z = Math.max(max.z, item.position.z);
        }

        // Add padding to ensure all points are within bounds
        const padding = 10;
        min.subScalar(padding);
        max.addScalar(padding);

        return new AABB(min, max);
    }

    /**
     * Expand the root bounds to contain positions that fall outside
     * Expands in a loop until the position is contained
     */
    private expandRoot(position: THREE.Vector3): void {
        if (!this.root) return;

        // Keep expanding until position is contained
        while (this.root && !this.root.bounds.containsPoint(position)) {
            const currentRoot: OctreeNode<T> = this.root;
            // Double the size of the root bounds
            const center: THREE.Vector3 = currentRoot.bounds.getCenter();
            const size: THREE.Vector3 = currentRoot.bounds.getSize();
            const maxSize = Math.max(size.x, size.y, size.z);

            const newSize = maxSize * 2;
            const halfSize = newSize / 2;

            const newMin: THREE.Vector3 = new THREE.Vector3(
                center.x - halfSize,
                center.y - halfSize,
                center.z - halfSize,
            );
            const newMax = new THREE.Vector3(
                center.x + halfSize,
                center.y + halfSize,
                center.z + halfSize,
            );

            const newRoot: OctreeNode<T> = new OctreeNode<T>(new AABB(newMin, newMax), 0);

            // Reinsert all items into new root
            const allItems = this.getAllItems();
            this.root = newRoot;
            this.itemMap.clear();
            this.totalItems = 0;

            for (const item of allItems) {
                this.insertIntoNode(newRoot, item);
                this.itemMap.set(item.id, item);
                this.totalItems++;
            }
        }
    }
}
