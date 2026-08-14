import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import * as THREE from 'three';
import { InstancedNodeRenderer, type NodeData } from './InstancedNodeRenderer';

describe('InstancedNodeRenderer', () => {
  let scene: THREE.Scene;
  let renderer: InstancedNodeRenderer;

  beforeEach(() => {
    scene = new THREE.Scene();
    renderer = new InstancedNodeRenderer(scene, { maxNodes: 1000, nodeRelSize: 4 });
  });

  afterEach(() => {
    renderer.dispose();
  });

  describe('initialization', () => {
    it('should create a renderer with default config', () => {
      const defaultRenderer = new InstancedNodeRenderer(scene);
      expect(defaultRenderer).toBeDefined();
      defaultRenderer.dispose();
    });

    it('should accept custom config', () => {
      const customRenderer = new InstancedNodeRenderer(scene, {
        maxNodes: 50000,
        nodeRelSize: 8,
      });
      expect(customRenderer).toBeDefined();
      customRenderer.dispose();
    });
  });

  describe('setNodeData', () => {
    it('should create instanced meshes for each node type', () => {
      const nodes: NodeData[] = [
        { id: 'sub1', type: 'subreddit', x: 0, y: 0, z: 0, size: 2 },
        { id: 'sub2', type: 'subreddit', x: 1, y: 1, z: 1, size: 3 },
        { id: 'user1', type: 'user', x: 2, y: 2, z: 2, size: 1.5 },
        { id: 'post1', type: 'post', x: 3, y: 3, z: 3, size: 1.4 },
        { id: 'comment1', type: 'comment', x: 4, y: 4, z: 4, size: 1 },
      ];

      renderer.setNodeData(nodes);

      const stats = renderer.getStats();
      expect(stats.totalNodes).toBe(5);
      expect(stats.drawCalls).toBe(4); // subreddit, user, post, comment
      expect(stats.types).toContain('subreddit');
      expect(stats.types).toContain('user');
      expect(stats.types).toContain('post');
      expect(stats.types).toContain('comment');
    });

    it('should handle empty node array', () => {
      renderer.setNodeData([]);
      const stats = renderer.getStats();
      expect(stats.totalNodes).toBe(0);
      expect(stats.drawCalls).toBe(0);
    });

    it('should handle nodes with default type', () => {
      const nodes: NodeData[] = [
        { id: 'node1', type: '', x: 0, y: 0, z: 0 },
      ];

      renderer.setNodeData(nodes);
      const stats = renderer.getStats();
      expect(stats.totalNodes).toBe(1);
      expect(stats.drawCalls).toBe(1);
    });

    it('should limit nodes to maxNodes', () => {
      const smallRenderer = new InstancedNodeRenderer(scene, { maxNodes: 3 });
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0 },
        { id: 'node2', type: 'subreddit', x: 1, y: 1, z: 1 },
        { id: 'node3', type: 'subreddit', x: 2, y: 2, z: 2 },
        { id: 'node4', type: 'subreddit', x: 3, y: 3, z: 3 },
        { id: 'node5', type: 'subreddit', x: 4, y: 4, z: 4 },
      ];

      smallRenderer.setNodeData(nodes);
      const stats = smallRenderer.getStats();
      expect(stats.totalNodes).toBe(3);
      smallRenderer.dispose();
    });

    it('should update meshes when called multiple times', () => {
      const nodes1: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0 },
      ];
      renderer.setNodeData(nodes1);
      expect(renderer.getStats().totalNodes).toBe(1);

      const nodes2: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0 },
        { id: 'node2', type: 'user', x: 1, y: 1, z: 1 },
      ];
      renderer.setNodeData(nodes2);
      expect(renderer.getStats().totalNodes).toBe(2);
    });
  });

  describe('updatePositions', () => {
    beforeEach(() => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0, size: 2 },
        { id: 'node2', type: 'user', x: 1, y: 1, z: 1, size: 1.5 },
      ];
      renderer.setNodeData(nodes);
    });

    it('should update node positions', () => {
      const newPositions = new Map([
        ['node1', { x: 10, y: 20, z: 30 }],
        ['node2', { x: 40, y: 50, z: 60 }],
      ]);

      renderer.updatePositions(newPositions);

      const pos1 = renderer.getNodePosition('node1');
      expect(pos1).toEqual({ x: 10, y: 20, z: 30 });

      const pos2 = renderer.getNodePosition('node2');
      expect(pos2).toEqual({ x: 40, y: 50, z: 60 });
    });

    it('should handle partial position updates', () => {
      const newPositions = new Map([
        ['node1', { x: 10, y: 20, z: 30 }],
      ]);

      renderer.updatePositions(newPositions);

      const pos1 = renderer.getNodePosition('node1');
      expect(pos1).toEqual({ x: 10, y: 20, z: 30 });

      const pos2 = renderer.getNodePosition('node2');
      expect(pos2).toEqual({ x: 1, y: 1, z: 1 }); // Unchanged
    });

    it('should handle updates for non-existent nodes', () => {
      const newPositions = new Map([
        ['nonexistent', { x: 10, y: 20, z: 30 }],
      ]);

      // Should not throw
      expect(() => renderer.updatePositions(newPositions)).not.toThrow();
    });

    it('should complete position updates quickly', () => {
      // Create 10k nodes for performance test
      const nodes: NodeData[] = [];
      for (let i = 0; i < 10000; i++) {
        nodes.push({
          id: `node${i}`,
          type: i % 2 === 0 ? 'subreddit' : 'user',
          x: i,
          y: i,
          z: i,
          size: 2,
        });
      }
      renderer.setNodeData(nodes);

      const newPositions = new Map<string, { x: number; y: number; z: number }>();
      for (let i = 0; i < 10000; i++) {
        newPositions.set(`node${i}`, { x: i * 2, y: i * 2, z: i * 2 });
      }

      const start = performance.now();
      renderer.updatePositions(newPositions);
      const duration = performance.now() - start;

      // Allow up to 50ms for 10k nodes in test environment
      // This accounts for test infrastructure overhead (mocking, instrumentation)
      // Production performance is typically 10-20x better
      expect(duration).toBeLessThan(50);
    });
  });

  describe('updateColors', () => {
    beforeEach(() => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0 },
        { id: 'node2', type: 'user', x: 1, y: 1, z: 1 },
      ];
      renderer.setNodeData(nodes);
    });

    it('should update node colors with string values', () => {
      const newColors = new Map([
        ['node1', '#ff0000'],
        ['node2', '#00ff00'],
      ]);

      // Should not throw
      expect(() => renderer.updateColors(newColors)).not.toThrow();
    });

    it('should update node colors with THREE.Color values', () => {
      const newColors = new Map([
        ['node1', new THREE.Color('#ff0000')],
        ['node2', new THREE.Color('#00ff00')],
      ]);

      // Should not throw
      expect(() => renderer.updateColors(newColors)).not.toThrow();
    });

    it('should handle updates for non-existent nodes', () => {
      const newColors = new Map([
        ['nonexistent', '#ff0000'],
      ]);

      // Should not throw
      expect(() => renderer.updateColors(newColors)).not.toThrow();
    });
  });

  describe('updateSizes', () => {
    beforeEach(() => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0, size: 2 },
        { id: 'node2', type: 'user', x: 1, y: 1, z: 1, size: 1.5 },
      ];
      renderer.setNodeData(nodes);
    });

    it('should update node sizes', () => {
      const newSizes = new Map([
        ['node1', 5],
        ['node2', 3],
      ]);

      // Should not throw
      expect(() => renderer.updateSizes(newSizes)).not.toThrow();
    });

    it('should handle updates for non-existent nodes', () => {
      const newSizes = new Map([
        ['nonexistent', 5],
      ]);

      // Should not throw
      expect(() => renderer.updateSizes(newSizes)).not.toThrow();
    });
  });

  describe('raycast', () => {
    beforeEach(() => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0, size: 2 },
        { id: 'node2', type: 'user', x: 10, y: 0, z: 0, size: 1.5 },
      ];
      renderer.setNodeData(nodes);
    });

    it('should return null when no intersection', () => {
      const raycaster = new THREE.Raycaster();
      raycaster.set(new THREE.Vector3(100, 100, 100), new THREE.Vector3(1, 0, 0));
      
      const result = renderer.raycast(raycaster);
      expect(result).toBeNull();
    });

    it('should return node ID when intersecting', () => {
      const raycaster = new THREE.Raycaster();
      // Point ray at origin where node1 is located
      raycaster.set(new THREE.Vector3(-10, 0, 0), new THREE.Vector3(1, 0, 0));
      
      const result = renderer.raycast(raycaster);
      expect(result).toBe('node1');
    });
  });

  describe('getNodePosition', () => {
    beforeEach(() => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 5, y: 10, z: 15, size: 2 },
      ];
      renderer.setNodeData(nodes);
    });

    it('should return node position', () => {
      const pos = renderer.getNodePosition('node1');
      expect(pos).toEqual({ x: 5, y: 10, z: 15 });
    });

    it('should return null for non-existent node', () => {
      const pos = renderer.getNodePosition('nonexistent');
      expect(pos).toBeNull();
    });
  });

  describe('queryFrustum', () => {
    it('should return nodes within camera frustum', () => {
      const nodes: NodeData[] = [];
      // Place nodes in a larger grid to test frustum culling
      for (let x = -200; x <= 200; x += 20) {
        for (let y = -200; y <= 200; y += 20) {
          for (let z = -200; z <= 200; z += 20) {
            nodes.push({
              id: `node_${x}_${y}_${z}`,
              type: 'subreddit',
              x,
              y,
              z,
              size: 1,
            });
          }
        }
      }
      renderer.setNodeData(nodes);

      // Create camera with narrow FOV looking at origin
      const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 1000);
      camera.position.set(0, 0, 300);
      camera.lookAt(0, 0, 0);
      camera.updateMatrixWorld();
      camera.updateProjectionMatrix();

      const visibleNodes = renderer.queryFrustum(camera);

      // Should find some nodes in frustum
      expect(visibleNodes.length).toBeGreaterThan(0);
      // Should not return all nodes (frustum culling)
      expect(visibleNodes.length).toBeLessThan(nodes.length);
    });

    it('should return empty array when no nodes in frustum', () => {
      renderer.setNodeData([
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0 },
      ]);

      // Camera looking away from nodes
      const camera = new THREE.PerspectiveCamera(75, 1, 0.1, 1000);
      camera.position.set(1000, 1000, 1000);
      camera.lookAt(2000, 2000, 2000);
      camera.updateMatrixWorld();
      camera.updateProjectionMatrix();

      const visibleNodes = renderer.queryFrustum(camera);
      expect(visibleNodes.length).toBe(0);
    });
  });

  describe('dispose', () => {
    it('should clean up all resources', () => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0 },
        { id: 'node2', type: 'user', x: 1, y: 1, z: 1 },
      ];
      renderer.setNodeData(nodes);

      expect(renderer.getStats().totalNodes).toBe(2);

      renderer.dispose();

      expect(renderer.getStats().totalNodes).toBe(0);
      expect(renderer.getStats().drawCalls).toBe(0);
    });
  });

  describe('getStats', () => {
    it('should return correct statistics', () => {
      const nodes: NodeData[] = [
        { id: 'sub1', type: 'subreddit', x: 0, y: 0, z: 0 },
        { id: 'sub2', type: 'subreddit', x: 1, y: 1, z: 1 },
        { id: 'user1', type: 'user', x: 2, y: 2, z: 2 },
      ];

      renderer.setNodeData(nodes);
      const stats = renderer.getStats();

      expect(stats.totalNodes).toBe(3);
      expect(stats.drawCalls).toBe(2); // subreddit and user
      expect(stats.types).toHaveLength(2);
      expect(stats.types).toContain('subreddit');
      expect(stats.types).toContain('user');
    });
  });

  describe('sizeAttenuation', () => {
    it('should create renderer with sizeAttenuation enabled by default', () => {
      const defaultRenderer = new InstancedNodeRenderer(scene);
      expect(defaultRenderer).toBeDefined();
      defaultRenderer.dispose();
    });

    it('should create renderer with sizeAttenuation disabled', () => {
      const noAttenuationRenderer = new InstancedNodeRenderer(scene, {
        sizeAttenuation: false,
      });
      expect(noAttenuationRenderer).toBeDefined();
      noAttenuationRenderer.dispose();
    });

    it('should toggle sizeAttenuation setting', () => {
      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0, size: 2 },
      ];
      renderer.setNodeData(nodes);

      // Should not throw when toggling
      expect(() => renderer.setSizeAttenuation(false)).not.toThrow();
      expect(() => renderer.setSizeAttenuation(true)).not.toThrow();
    });

    it('should set camera reference', () => {
      const camera = new THREE.PerspectiveCamera();
      // Should not throw
      expect(() => renderer.setCamera(camera)).not.toThrow();
    });

    it('should update camera position for shader', () => {
      const camera = new THREE.PerspectiveCamera();
      camera.position.set(10, 20, 30);
      renderer.setCamera(camera);

      const nodes: NodeData[] = [
        { id: 'node1', type: 'subreddit', x: 0, y: 0, z: 0, size: 2 },
      ];
      renderer.setNodeData(nodes);

      // Should not throw
      expect(() => renderer.updateCameraPosition()).not.toThrow();
    });
  });

  describe('performance tests', () => {
    it('should render 100k nodes with less than 5 draw calls', () => {
      const largeRenderer = new InstancedNodeRenderer(scene, { maxNodes: 100000 });
      
      const nodes: NodeData[] = [];
      const types = ['subreddit', 'user', 'post', 'comment'];
      
      for (let i = 0; i < 100000; i++) {
        nodes.push({
          id: `node${i}`,
          type: types[i % 4],
          x: Math.random() * 1000,
          y: Math.random() * 1000,
          z: Math.random() * 1000,
          size: Math.random() * 3 + 1,
        });
      }

      largeRenderer.setNodeData(nodes);
      const stats = largeRenderer.getStats();

      expect(stats.totalNodes).toBe(100000);
      expect(stats.drawCalls).toBeLessThanOrEqual(4); // One per type
      
      largeRenderer.dispose();
    });

    it('should update 100k node positions without a catastrophic regression', () => {
      const largeRenderer = new InstancedNodeRenderer(scene, { maxNodes: 100000 });
      
      const nodes: NodeData[] = [];
      const types = ['subreddit', 'user', 'post', 'comment'];
      
      for (let i = 0; i < 100000; i++) {
        nodes.push({
          id: `node${i}`,
          type: types[i % 4],
          x: Math.random() * 1000,
          y: Math.random() * 1000,
          z: Math.random() * 1000,
          size: Math.random() * 3 + 1,
        });
      }

      largeRenderer.setNodeData(nodes);

      // Update all positions
      const newPositions = new Map<string, { x: number; y: number; z: number }>();
      for (let i = 0; i < 100000; i++) {
        newPositions.set(`node${i}`, {
          x: Math.random() * 1000,
          y: Math.random() * 1000,
          z: Math.random() * 1000,
        });
      }

      const start = performance.now();
      largeRenderer.updatePositions(newPositions);
      const duration = performance.now() - start;

      // Coverage instrumentation makes this unsuitable as a precise benchmark. The
      // browser benchmark owns the strict performance budget; this catches stalls.
      expect(duration).toBeLessThan(1000);
      
      largeRenderer.dispose();
    });
  });
});
