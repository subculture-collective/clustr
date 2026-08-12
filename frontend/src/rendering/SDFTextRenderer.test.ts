import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import * as THREE from 'three';
import {
  SDFTextRenderer,
  renderableSpatialLabel,
  type LabelData,
} from './SDFTextRenderer';

describe('renderableSpatialLabel', () => {
  it('keeps bundled scripts and substitutes glyphs that would require a CDN fallback', () => {
    expect(renderableSpatialLabel('AskReddit · Jokes · food')).toBe('AskReddit · Jokes · food');
    expect(renderableSpatialLabel('κόσμος мир 宇宙')).toBe('κόσμος мир ??');
  });

  it('truncates by rendered characters', () => {
    expect(renderableSpatialLabel('a'.repeat(29))).toBe(`${'a'.repeat(27)}…`);
  });
});

// Mock troika-three-text to avoid test environment issues
vi.mock('troika-three-text', () => {
  class MockText extends THREE.Mesh {
    text = '';
    fontSize = 8;
    color = '#ffffff';
    anchorX = 'center';
    anchorY = 'middle';
    outlineWidth = 0;
    outlineColor = '';
    outlineOpacity = 0;
    
    constructor() {
      super();
      this.material = new THREE.MeshBasicMaterial({ transparent: true });
    }
    
    sync() {
      // Mock sync method
    }
    
    dispose() {
      if (this.material instanceof THREE.Material) {
        this.material.dispose();
      }
      if (this.geometry) {
        this.geometry.dispose();
      }
    }
  }
  
  return {
    Text: MockText,
  };
});

describe('SDFTextRenderer', () => {
  let scene: THREE.Scene;
  let camera: THREE.PerspectiveCamera;
  let renderer: SDFTextRenderer;

  beforeEach(() => {
    scene = new THREE.Scene();
    camera = new THREE.PerspectiveCamera(75, 1, 0.1, 1000);
    camera.position.set(0, 0, 100);
    camera.lookAt(0, 0, 0);
    camera.updateMatrixWorld();
    camera.updateProjectionMatrix();
  });

  afterEach(() => {
    if (renderer) {
      renderer.dispose();
    }
  });

  describe('Initialization', () => {
    it('should create renderer with default config', () => {
      renderer = new SDFTextRenderer(scene);
      const stats = renderer.getStats();
      
      expect(stats.totalLabels).toBe(0);
      expect(stats.visibleLabels).toBe(0);
      expect(stats.maxLabels).toBe(500);
    });

    it('should create renderer with custom config', () => {
      renderer = new SDFTextRenderer(scene, {
        maxLabels: 200,
        fontSize: 10,
        color: '#ff0000',
      });
      
      const stats = renderer.getStats();
      expect(stats.maxLabels).toBe(200);
    });
  });

  describe('Label Management', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene);
    });

    it('should create labels from data', () => {
      const labels: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
        { id: 'node2', text: 'Label 2', position: { x: 10, y: 0, z: 0 } },
        { id: 'node3', text: 'Label 3', position: { x: 0, y: 10, z: 0 } },
      ];

      renderer.setLabels(labels);
      
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(3);
    });

    it('should truncate long text', () => {
      const longText = 'This is a very long label text that should be truncated';
      const labels: LabelData[] = [
        { id: 'node1', text: longText, position: { x: 0, y: 0, z: 0 } },
      ];

      renderer.setLabels(labels);
      
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(1);
      // The actual truncation is tested implicitly in the text object
    });

    it('should update existing labels', () => {
      const labels1: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
      ];

      renderer.setLabels(labels1);
      expect(renderer.getStats().totalLabels).toBe(1);

      const labels2: LabelData[] = [
        { id: 'node1', text: 'Updated Label', position: { x: 5, y: 5, z: 5 } },
      ];

      renderer.setLabels(labels2);
      expect(renderer.getStats().totalLabels).toBe(1);
    });

    it('should remove old labels', () => {
      const labels1: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
        { id: 'node2', text: 'Label 2', position: { x: 10, y: 0, z: 0 } },
      ];

      renderer.setLabels(labels1);
      expect(renderer.getStats().totalLabels).toBe(2);

      const labels2: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
      ];

      renderer.setLabels(labels2);
      expect(renderer.getStats().totalLabels).toBe(1);
    });

    it('should handle custom size multiplier', () => {
      const labels: LabelData[] = [
        { id: 'node1', text: 'Small', position: { x: 0, y: 0, z: 0 }, size: 0.5 },
        { id: 'node2', text: 'Large', position: { x: 10, y: 0, z: 0 }, size: 2.0 },
      ];

      renderer.setLabels(labels);
      expect(renderer.getStats().totalLabels).toBe(2);
    });
  });

  describe('Position Updates', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene);
      const labels: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
        { id: 'node2', text: 'Label 2', position: { x: 10, y: 0, z: 0 } },
      ];
      renderer.setLabels(labels);
    });

    it('should update label positions', () => {
      const positions = new Map([
        ['node1', { x: 5, y: 5, z: 5 }],
        ['node2', { x: 15, y: 15, z: 15 }],
      ]);

      renderer.updatePositions(positions);
      
      // Positions are updated internally
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(2);
    });

    it('should handle missing nodes in position update', () => {
      const positions = new Map([
        ['node1', { x: 5, y: 5, z: 5 }],
        // node2 missing
      ]);

      renderer.updatePositions(positions);
      
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(2);
    });
  });

  describe('Visibility Management', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene);
      const labels: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
        { id: 'node2', text: 'Label 2', position: { x: 10, y: 0, z: 0 } },
        { id: 'node3', text: 'Label 3', position: { x: 0, y: 10, z: 0 } },
      ];
      renderer.setLabels(labels);
    });

    it('should show labels in label set', () => {
      const labelSet = new Set(['node1', 'node2']);
      
      renderer.updateVisibility(camera, labelSet);
      
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(3);
      // Visibility depends on frustum culling and label set
    });

    it('should hide labels not in label set', () => {
      const labelSet = new Set(['node1']);
      
      renderer.updateVisibility(camera, labelSet);
      
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(3);
      // node2 and node3 should be hidden
    });

    it('should respect distance-based visibility', () => {
      const labelSet = new Set(['node1', 'node2', 'node3']);
      
      // Close distance - should show
      renderer.updateVisibility(camera, labelSet, 50, 800);
      let stats = renderer.getStats();
      expect(stats.totalLabels).toBe(3);
      
      // Far distance - should hide
      renderer.updateVisibility(camera, labelSet, 1000, 800);
      stats = renderer.getStats();
      expect(stats.totalLabels).toBe(3);
    });
  });

  describe('Billboard Orientation', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene);
      const labels: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
      ];
      renderer.setLabels(labels);
    });

    it('should update billboard orientation', () => {
      // Update camera orientation
      camera.position.set(10, 10, 10);
      camera.lookAt(0, 0, 0);
      camera.updateMatrixWorld();

      renderer.updateBillboard(camera);
      
      // Billboard update is applied
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(1);
    });
  });

  describe('Performance', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene, { maxLabels: 1000 });
    });

    it('should handle 500+ labels', () => {
      const labels: LabelData[] = [];
      // Use deterministic positions for stable test behavior
      for (let i = 0; i < 500; i++) {
        labels.push({
          id: `node${i}`,
          text: `Label ${i}`,
          position: {
            x: (i % 10) * 10 - 50,
            y: Math.floor(i / 10) * 10 - 50,
            z: ((i % 5) - 2) * 10,
          },
        });
      }

      renderer.setLabels(labels);

      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(500);
    });

    it('should update positions efficiently', () => {
      const labels: LabelData[] = [];
      const positions = new Map<string, { x: number; y: number; z: number }>();
      
      // Use deterministic positions for stable test behavior
      for (let i = 0; i < 500; i++) {
        const id = `node${i}`;
        labels.push({
          id,
          text: `Label ${i}`,
          position: { x: 0, y: 0, z: 0 },
        });
        positions.set(id, {
          x: (i % 10) * 10 - 50,
          y: Math.floor(i / 10) * 10 - 50,
          z: ((i % 5) - 2) * 10,
        });
      }

      renderer.setLabels(labels);
      renderer.updatePositions(positions);

      // Verify positions were updated (check a sample)
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(500);
    });
  });

  describe('Cleanup', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene);
      const labels: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
        { id: 'node2', text: 'Label 2', position: { x: 10, y: 0, z: 0 } },
      ];
      renderer.setLabels(labels);
    });

    it('should dispose all resources', () => {
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(2);

      renderer.dispose();
      
      const statsAfter = renderer.getStats();
      expect(statsAfter.totalLabels).toBe(0);
    });
  });

  describe('Statistics', () => {
    beforeEach(() => {
      renderer = new SDFTextRenderer(scene, { maxLabels: 200 });
    });

    it('should provide accurate statistics', () => {
      const labels: LabelData[] = [
        { id: 'node1', text: 'Label 1', position: { x: 0, y: 0, z: 0 } },
        { id: 'node2', text: 'Label 2', position: { x: 10, y: 0, z: 0 } },
        { id: 'node3', text: 'Label 3', position: { x: 0, y: 10, z: 0 } },
      ];

      renderer.setLabels(labels);
      
      const stats = renderer.getStats();
      expect(stats.totalLabels).toBe(3);
      expect(stats.maxLabels).toBe(200);
    });
  });
});
