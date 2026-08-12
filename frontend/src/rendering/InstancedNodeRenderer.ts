import * as THREE from 'three';
import { Octree, type OctreeItem } from '../spatial/Octree';

/**
 * InstancedNodeRenderer - High-performance node rendering using THREE.InstancedMesh
 * 
 * Renders all nodes of the same type in a single draw call using GPU instancing.
 * This dramatically reduces CPU overhead and improves performance for large graphs.
 * 
 * Key features:
 * - Single InstancedMesh per node type (typically 4 draw calls for core node types)
 * - Position updates via instanceMatrix (no scene graph traversal)
 * - Per-instance colors via instanceColor attribute
 * - Per-instance sizes via scale in instance matrix
 * - Octree spatial index for O(log n) raycasting and frustum culling
 * - Optimized for 100k+ nodes
 * 
 * Performance targets:
 * - ~4 draw calls for core node types (plus extras for links/labels as configured)
 * - <5ms for position updates
 * - <500MB memory usage for 100k nodes
 * - <1ms raycasting with octree
 * - <2ms frustum culling with octree
 * 
 * @example
 * ```typescript
 * const renderer = new InstancedNodeRenderer(scene, 100000);
 * 
 * // Set initial data
 * renderer.setNodeData(nodes);
 * 
 * // Update positions (from layout engine)
 * renderer.updatePositions(positions);
 * 
 * // Update colors
 * renderer.updateColors(colors);
 * 
 * // Raycast for interactions (uses octree)
 * const node = renderer.raycast(raycaster);
 * 
 * // Frustum culling (uses octree)
 * const visibleNodes = renderer.queryFrustum(camera);
 * ```
 */

export interface NodeData {
  id: string;
  type: 'subreddit' | 'user' | 'post' | 'comment' | string;
  x?: number;
  y?: number;
  z?: number;
  size?: number;
  color?: string;
}

export interface InstancedNodeRendererConfig {
  maxNodes?: number;
  nodeRelSize?: number;
  sizeAttenuation?: boolean;
}

interface TypedMesh {
  mesh: THREE.InstancedMesh;
  nodeIds: string[];
  count: number;
}

const DEFAULT_COLORS: Record<string, THREE.Color> = {
  community: new THREE.Color('#b6ff62'),
  subreddit: new THREE.Color('#78d6b0'),
  user: new THREE.Color('#69a7d8'),
  post: new THREE.Color('#e6bd72'),
  comment: new THREE.Color('#d47e96'),
  default: new THREE.Color('#d8e7e3'),
};

export class InstancedNodeRenderer {
  private scene: THREE.Scene;
  private geometry: THREE.SphereGeometry;
  private meshes: Map<string, TypedMesh> = new Map();
  private nodeMap: Map<string, { type: string; index: number }> = new Map();
  private maxNodes: number;
  private nodeRelSize: number;
  private octree: Octree<NodeData>;
  private _frustum: THREE.Frustum | null = null;
  private _projectionMatrix: THREE.Matrix4 | null = null;
  private sizeAttenuation: boolean;
  private camera: THREE.Camera | null = null;
  private cameraPosVector: THREE.Vector3 = new THREE.Vector3(); // Reusable vector to avoid per-frame allocation

  constructor(scene: THREE.Scene, config: InstancedNodeRendererConfig = {}) {
    this.scene = scene;
    this.maxNodes = config.maxNodes || 100000;
    this.nodeRelSize = config.nodeRelSize || 4;
    this.sizeAttenuation = config.sizeAttenuation !== undefined ? config.sizeAttenuation : true;
    
    // Create shared geometry (8 segments for performance vs quality balance)
    this.geometry = new THREE.SphereGeometry(1, 8, 8);
    
    // Initialize octree for spatial queries
    this.octree = new Octree<NodeData>({
      maxItemsPerNode: 8,
      maxDepth: 8,
      minCellSize: 1.0,
    });
  }

  /**
   * Set node data and create/update instanced meshes
   */
  public setNodeData(nodes: NodeData[]): void {
    // Group nodes by type
    const nodesByType = new Map<string, NodeData[]>();
    for (const node of nodes) {
      const type = node.type || 'default';
      if (!nodesByType.has(type)) {
        nodesByType.set(type, []);
      }
      nodesByType.get(type)!.push(node);
    }

    // Clear old node map
    this.nodeMap.clear();

    // Create or update meshes for each type
    for (const [type, typedNodes] of nodesByType.entries()) {
      this.createOrUpdateMeshForType(type, typedNodes);
    }

    // Remove meshes for types that no longer exist
    for (const [type, typedMesh] of this.meshes.entries()) {
      if (!nodesByType.has(type)) {
        this.scene.remove(typedMesh.mesh);
        // Only dispose material, not shared geometry
        (typedMesh.mesh.material as THREE.Material).dispose();
        this.meshes.delete(type);
      }
    }

    // Build octree from node positions
    const octreeItems: OctreeItem<NodeData>[] = [];
    for (const node of nodes) {
      if (node.x !== undefined && node.y !== undefined && node.z !== undefined) {
        octreeItems.push({
          id: node.id,
          position: new THREE.Vector3(node.x, node.y, node.z),
          data: node,
        });
      }
    }
    this.octree.build(octreeItems);
  }

  /**
   * Create or update instanced mesh for a specific node type
   */
  private createOrUpdateMeshForType(type: string, nodes: NodeData[]): void {
    const count = Math.min(nodes.length, this.maxNodes);
    
    // Check if we need to recreate the mesh (different count)
    const existing = this.meshes.get(type);
    if (existing && existing.mesh.count !== count) {
      // Remove old mesh
      this.scene.remove(existing.mesh);
      // Only dispose material, not shared geometry
      (existing.mesh.material as THREE.Material).dispose();
      this.meshes.delete(type);
    }

    let typedMesh: TypedMesh;
    
    if (!this.meshes.has(type)) {
      // Create new mesh
      const material = this.createMaterial(type);
      
      const mesh = new THREE.InstancedMesh(this.geometry, material, count);
      mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage); // Will be updated frequently
      
      // Enable per-instance colors
      mesh.instanceColor = new THREE.InstancedBufferAttribute(
        new Float32Array(count * 3),
        3
      );
      mesh.instanceColor.setUsage(THREE.DynamicDrawUsage);
      
      this.scene.add(mesh);
      
      typedMesh = {
        mesh,
        nodeIds: new Array(count),
        count: 0,
      };
      
      this.meshes.set(type, typedMesh);
    } else {
      typedMesh = this.meshes.get(type)!;
      typedMesh.count = 0;
    }

    // Set positions, colors, and scales for each node
    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const rotation = new THREE.Quaternion();
    const scale = new THREE.Vector3();
    const color = new THREE.Color();

    for (let i = 0; i < count; i++) {
      const node = nodes[i];
      
      // Store node mapping
      this.nodeMap.set(node.id, { type, index: i });
      typedMesh.nodeIds[i] = node.id;
      
      // Set position
      position.set(node.x || 0, node.y || 0, node.z || 0);
      
      // Set scale based on node size
      const size = (node.size || 1) * this.nodeRelSize;
      scale.set(size, size, size);
      
      // Create matrix
      matrix.compose(position, rotation, scale);
      typedMesh.mesh.setMatrixAt(i, matrix);
      
      // Set color
      if (node.color) {
        color.set(node.color);
      } else {
        color.copy(DEFAULT_COLORS[type] || DEFAULT_COLORS.default);
      }
      typedMesh.mesh.setColorAt(i, color);
    }

    typedMesh.count = count;
    typedMesh.mesh.instanceMatrix.needsUpdate = true;
    if (typedMesh.mesh.instanceColor) {
      typedMesh.mesh.instanceColor.needsUpdate = true;
    }
    typedMesh.mesh.computeBoundingSphere();
  }

  /**
   * Update positions for all nodes
   * @param positions Map of node ID to position
   */
  public updatePositions(positions: Map<string, { x: number; y: number; z: number }>): void {
    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const rotation = new THREE.Quaternion();
    const scale = new THREE.Vector3();

    // Track ALL items for octree rebuild (not just updated ones)
    const octreeItems: OctreeItem<NodeData>[] = [];

    for (const [, typedMesh] of this.meshes.entries()) {
      let updated = false;

      for (let i = 0; i < typedMesh.count; i++) {
        const nodeId = typedMesh.nodeIds[i];
        const pos = positions.get(nodeId);
        
        // Get current position
        typedMesh.mesh.getMatrixAt(i, matrix);
        matrix.decompose(position, rotation, scale);
        
        if (pos) {
          // Update position
          position.set(pos.x, pos.y, pos.z);
          matrix.compose(position, rotation, scale);
          typedMesh.mesh.setMatrixAt(i, matrix);
          updated = true;
        }
        
        // Add ALL nodes to octree (both updated and unchanged)
        octreeItems.push({
          id: nodeId,
          position: new THREE.Vector3(position.x, position.y, position.z),
          data: {
            id: nodeId,
            type: typedMesh.nodeIds[i].startsWith('subreddit_') ? 'subreddit' :
                  typedMesh.nodeIds[i].startsWith('user_') ? 'user' :
                  typedMesh.nodeIds[i].startsWith('post_') ? 'post' :
                  typedMesh.nodeIds[i].startsWith('comment_') ? 'comment' : 'default',
            x: position.x,
            y: position.y,
            z: position.z,
          },
        });
      }

      if (updated) {
        typedMesh.mesh.instanceMatrix.needsUpdate = true;
        typedMesh.mesh.computeBoundingSphere();
      }
    }

    // Rebuild octree with ALL node positions
    if (octreeItems.length > 0) {
      this.octree.build(octreeItems);
    }
  }

  /**
   * Update colors for specific nodes
   * @param colors Map of node ID to color
   */
  public updateColors(colors: Map<string, string | THREE.Color>): void {
    const color = new THREE.Color();

    for (const [nodeId, colorValue] of colors.entries()) {
      const nodeInfo = this.nodeMap.get(nodeId);
      if (!nodeInfo) continue;

      const typedMesh = this.meshes.get(nodeInfo.type);
      if (!typedMesh || !typedMesh.mesh.instanceColor) continue;

      if (typeof colorValue === 'string') {
        color.set(colorValue);
      } else {
        color.copy(colorValue);
      }

      typedMesh.mesh.setColorAt(nodeInfo.index, color);
      typedMesh.mesh.instanceColor.needsUpdate = true;
    }
  }

  /**
   * Update size for specific nodes
   * @param sizes Map of node ID to size
   */
  public updateSizes(sizes: Map<string, number>): void {
    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const rotation = new THREE.Quaternion();
    const scale = new THREE.Vector3();

    const updatedTypes = new Set<string>();

    for (const [nodeId, size] of sizes.entries()) {
      const nodeInfo = this.nodeMap.get(nodeId);
      if (!nodeInfo) continue;

      const typedMesh = this.meshes.get(nodeInfo.type);
      if (!typedMesh) continue;

      // Get current matrix
      typedMesh.mesh.getMatrixAt(nodeInfo.index, matrix);
      matrix.decompose(position, rotation, scale);

      // Update scale
      const newSize = size * this.nodeRelSize;
      scale.set(newSize, newSize, newSize);
      matrix.compose(position, rotation, scale);
      typedMesh.mesh.setMatrixAt(nodeInfo.index, matrix);

      updatedTypes.add(nodeInfo.type);
    }

    // Mark updated meshes
    for (const type of updatedTypes) {
      const typedMesh = this.meshes.get(type);
      if (typedMesh) {
        typedMesh.mesh.instanceMatrix.needsUpdate = true;
      }
    }
  }

  /**
   * Raycast to find intersected node using octree spatial index
   * @returns Node ID if intersected, null otherwise
   */
  public raycast(raycaster: THREE.Raycaster): string | null {
    // Use octree for fast spatial query to get candidates
    const ray = raycaster.ray;
    const maxDistance = raycaster.far || 1000;
    
    const nearestItem = this.octree.raycast(ray, maxDistance);
    if (!nearestItem) return null;

    // Verify hit with actual geometry raycasting
    // Raycast all meshes to find actual intersections
    const allIntersects: Array<{ nodeId: string; distance: number }> = [];
    
    for (const [, typedMesh] of this.meshes.entries()) {
      const intersects = raycaster.intersectObject(typedMesh.mesh, false);
      
      for (const intersect of intersects) {
        if (intersect.instanceId !== undefined) {
          const hitNodeId = typedMesh.nodeIds[intersect.instanceId];
          allIntersects.push({
            nodeId: hitNodeId,
            distance: intersect.distance,
          });
        }
      }
    }

    // Return closest actual hit
    if (allIntersects.length > 0) {
      allIntersects.sort((a, b) => a.distance - b.distance);
      return allIntersects[0].nodeId;
    }

    // No geometry hits - octree candidate was outside pick radius
    return null;
  }

  /**
   * Query nodes within camera frustum using octree
   * Returns node IDs that are potentially visible
   * @param camera Camera to use for frustum culling
   * @returns Array of visible node IDs
   */
  public queryFrustum(camera: THREE.Camera): string[] {
    // Ensure camera matrices are up to date
    camera.updateMatrixWorld();
    if (camera instanceof THREE.PerspectiveCamera || camera instanceof THREE.OrthographicCamera) {
      camera.updateProjectionMatrix();
    }
    
    // Reuse frustum and matrix instances to avoid allocations
    const projectionMatrix = this._projectionMatrix ?? (this._projectionMatrix = new THREE.Matrix4());
    const frustum = this._frustum ?? (this._frustum = new THREE.Frustum());
    projectionMatrix.multiplyMatrices(
      camera.projectionMatrix,
      camera.matrixWorldInverse
    );
    frustum.setFromProjectionMatrix(projectionMatrix);

    // Query octree for nodes in frustum
    const visibleItems = this.octree.queryFrustum(frustum);
    
    return visibleItems.map(item => item.id);
  }

  /**
   * Get node position by ID
   */
  public getNodePosition(nodeId: string): { x: number; y: number; z: number } | null {
    const nodeInfo = this.nodeMap.get(nodeId);
    if (!nodeInfo) return null;

    const typedMesh = this.meshes.get(nodeInfo.type);
    if (!typedMesh) return null;

    const matrix = new THREE.Matrix4();
    const position = new THREE.Vector3();
    const rotation = new THREE.Quaternion();
    const scale = new THREE.Vector3();

    typedMesh.mesh.getMatrixAt(nodeInfo.index, matrix);
    matrix.decompose(position, rotation, scale);

    return { x: position.x, y: position.y, z: position.z };
  }

  /**
   * Set camera reference for distance-based scaling
   */
  public setCamera(camera: THREE.Camera): void {
    this.camera = camera;
  }

  /** Update the global node scale before repopulating instance matrices. */
  public setNodeRelSize(nodeRelSize: number): void {
    this.nodeRelSize = nodeRelSize;
  }

  /**
   * Update size attenuation setting
   */
  public setSizeAttenuation(enabled: boolean): void {
    if (this.sizeAttenuation === enabled) return;
    this.sizeAttenuation = enabled;
    
    // Recreate materials for all meshes
    for (const [type, typedMesh] of this.meshes.entries()) {
      const oldMaterial = typedMesh.mesh.material as THREE.Material;
      const material = this.createMaterial(type);
      typedMesh.mesh.material = material;
      oldMaterial.dispose();
    }
  }

  /**
   * Create material for a node type with optional distance-based scaling
   */
  private createMaterial(type: string): THREE.Material {
    // Three's built-in material handles InstancedMesh transforms and instance
    // colors across WebGL implementations. The former custom shader redeclared
    // Three-provided uniforms/attributes and silently failed compilation,
    // leaving the primary canvas blank while the minimap still rendered.
    return new THREE.MeshBasicMaterial({
      color: DEFAULT_COLORS[type] || DEFAULT_COLORS.default,
      vertexColors: false,
      toneMapped: false,
    });
  }

  /**
   * Update camera position in shader uniforms (call this each frame)
   */
  public updateCameraPosition(): void {
    if (!this.camera) return;
    this.camera.getWorldPosition(this.cameraPosVector);
  }

  /**
   * Dispose of all resources
   */
  public dispose(): void {
    for (const [, typedMesh] of this.meshes.entries()) {
      this.scene.remove(typedMesh.mesh);
      // Only dispose material, not geometry (shared across all meshes)
      (typedMesh.mesh.material as THREE.Material).dispose();
    }
    this.meshes.clear();
    this.nodeMap.clear();
    // Dispose shared geometry once after all meshes are cleared
    this.geometry.dispose();
  }

  /**
   * Get statistics about the renderer
   */
  public getStats(): {
    totalNodes: number;
    drawCalls: number;
    types: string[];
  } {
    return {
      totalNodes: this.nodeMap.size,
      drawCalls: this.meshes.size,
      types: Array.from(this.meshes.keys()),
    };
  }
}
