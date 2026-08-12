import * as THREE from 'three';
import { Text } from 'troika-three-text';

/**
 * SDFTextRenderer - High-performance text rendering using Signed Distance Fields (SDF)
 *
 * Replaces three-spritetext with GPU-efficient SDF text rendering via troika-three-text.
 * Uses a single texture atlas with SDF-rendered glyphs for all labels.
 *
 * Key features:
 * - Single texture atlas shared across all labels
 * - 1-2 draw calls for all labels (vs 200 textures with SpriteText)
 * - Crisp text at all zoom levels (SDF advantage)
 * - Billboard orientation (always faces camera)
 * - LOD integration with frustum culling
 * - Dynamic label visibility based on viewport and zoom
 *
 * Performance targets:
 * - 500+ labels without FPS impact
 * - Memory usage <20MB for labels
 * - 1-2 draw calls regardless of label count
 *
 * @example
 * ```typescript
 * const renderer = new SDFTextRenderer(scene, {
 *   maxLabels: 500,
 *   fontSize: 8,
 * });
 *
 * // Set label data
 * renderer.setLabels(labelData);
 *
 * // Update positions when nodes move
 * renderer.updatePositions(nodePositions);
 *
 * // Update visibility based on camera
 * renderer.updateVisibility(camera, labelSet);
 *
 * // Clean up
 * renderer.dispose();
 * ```
 */

export interface LabelData {
  id: string;
  text: string;
  position: { x: number; y: number; z: number };
  size?: number; // Optional size multiplier
  priority?: number;
  alwaysVisible?: boolean;
}

export interface SDFTextRendererConfig {
  maxLabels?: number;
  fontSize?: number;
  color?: string;
  backgroundColor?: string;
}

interface TextObject {
  text: Text;
  id: string;
  visible: boolean;
  priority: number;
  alwaysVisible: boolean;
}

// The bundled font deliberately covers the scripts most common in the source
// corpus without pulling Troika's ~300 MB Unicode fallback catalog at runtime.
// Preserve the exact label in the API/search/inspector; only the GPU glyph
// layer substitutes unsupported code points so it never depends on a CDN.
export function renderableSpatialLabel(value: string, maxLength = 28): string {
  const supported = Array.from(value, (character) => {
    const codePoint = character.codePointAt(0) ?? 0;
    return (
      (codePoint >= 0x20 && codePoint <= 0x024f) ||
      (codePoint >= 0x0370 && codePoint <= 0x052f) ||
      (codePoint >= 0x1e00 && codePoint <= 0x1eff) ||
      (codePoint >= 0x2000 && codePoint <= 0x206f) ||
      (codePoint >= 0x20a0 && codePoint <= 0x20cf)
    )
      ? character
      : '?';
  }).join('');
  return supported.length > maxLength
    ? `${supported.slice(0, maxLength - 1)}…`
    : supported;
}

export class SDFTextRenderer {
  private scene: THREE.Scene;
  private group: THREE.Group;
  private textObjects: Map<string, TextObject> = new Map();
  private maxLabels: number;
  private fontSize: number;
  private color: string;
  private backgroundColor: string;
  private frustum: THREE.Frustum = new THREE.Frustum();
  private cameraMatrix: THREE.Matrix4 = new THREE.Matrix4();

  constructor(scene: THREE.Scene, config: SDFTextRendererConfig = {}) {
    this.scene = scene;
    this.maxLabels = config.maxLabels || 500;
    this.fontSize = config.fontSize || 8;
    this.color = config.color || '#ffffff';
    this.backgroundColor = config.backgroundColor || '#000000';

    // Create group to hold all text objects
    this.group = new THREE.Group();
    this.scene.add(this.group);
  }

  /**
   * Set label data and create text objects
   * Enforces maxLabels limit by taking the first N labels
   */
  setLabels(labels: LabelData[]): void {
    // Enforce maxLabels limit
    const limitedLabels = labels.slice(0, this.maxLabels);
    
    // Remove old text objects that are no longer needed
    const newIds = new Set(limitedLabels.map((l) => l.id));
    for (const [id, textObj] of this.textObjects) {
      if (!newIds.has(id)) {
        this.group.remove(textObj.text);
        textObj.text.dispose();
        this.textObjects.delete(id);
      }
    }

    // Create or update text objects
    for (const label of limitedLabels) {
      let textObj = this.textObjects.get(label.id);

      if (!textObj) {
        // Create new text object
        const text = new Text();
        
        // Configure text appearance
        text.text = renderableSpatialLabel(label.text);
        // Keep label rendering deterministic and available offline. Troika's
        // network default font made the scene depend on a third-party CDN.
        (text as Text & { font: string }).font = '/fonts/NotoSans-Latin.woff';
        text.fontSize = this.fontSize * (label.size || 1);
        text.color = this.color;
        text.anchorX = 'center';
        text.anchorY = 'middle';
        
        // Add background for better readability
        text.outlineWidth = 0.15;
        text.outlineColor = this.backgroundColor;
        text.outlineOpacity = 0.35;

        // Billboard orientation - always face camera
        text.material.depthTest = true;
        text.material.depthWrite = false;
        text.material.transparent = true;

        // Set position
        if (label.position) {
          text.position.set(label.position.x, label.position.y, label.position.z);
        }

        // Add to scene
        this.group.add(text);

        textObj = {
          text,
          id: label.id,
          visible: true,
          priority: label.priority ?? 0,
          alwaysVisible: label.alwaysVisible ?? false,
        };
        this.textObjects.set(label.id, textObj);
      } else {
        // Update existing text object
        const text = textObj.text;
        const newText = renderableSpatialLabel(label.text);
        
        if (text.text !== newText) {
          text.text = newText;
        }
        
        const newFontSize = this.fontSize * (label.size || 1);
        if (text.fontSize !== newFontSize) {
          text.fontSize = newFontSize;
        }

        if (label.position) {
          text.position.set(label.position.x, label.position.y, label.position.z);
        }
        textObj.priority = label.priority ?? 0;
        textObj.alwaysVisible = label.alwaysVisible ?? false;
      }

      // Trigger text sync (troika batches updates)
      textObj.text.sync();
    }
  }

  /**
   * Update positions for existing labels
   */
  updatePositions(positions: Map<string, { x: number; y: number; z: number }>): void {
    for (const [id, textObj] of this.textObjects) {
      const pos = positions.get(id);
      if (pos) {
        textObj.text.position.set(pos.x, pos.y, pos.z);
      }
    }
  }

  /**
   * Update visibility based on camera frustum and label set
   * Only renders labels that are:
   * 1. In the provided labelSet (top-N by weight)
   * 2. Visible in the camera frustum
   */
  updateVisibility(
    camera: THREE.Camera,
    labelSet: Set<string>,
    cameraDistance?: number,
    maxDistance?: number
  ): void {
    // Update frustum from camera
    this.cameraMatrix.multiplyMatrices(
      camera.projectionMatrix,
      camera.matrixWorldInverse
    );
    this.frustum.setFromProjectionMatrix(this.cameraMatrix);

    // Check distance-based visibility if provided
    const distanceCheck = maxDistance !== undefined && cameraDistance !== undefined;
    const shouldShowLabels = !distanceCheck || cameraDistance < maxDistance;

    const occupied: Array<{ left: number; right: number; top: number; bottom: number }> = [];
    const candidates = Array.from(this.textObjects.entries()).sort(
      ([idA, a], [idB, b]) => b.priority - a.priority || idA.localeCompare(idB)
    );
    for (const [id, textObj] of candidates) {
      // Check if in label set
      const inLabelSet = labelSet.has(id);
      
      // Check if in frustum
      const pos = textObj.text.position;
      const inFrustum = this.frustum.containsPoint(pos);

      // Update visibility
      let shouldBeVisible = inLabelSet && inFrustum && (shouldShowLabels || textObj.alwaysVisible);
      if (shouldBeVisible) {
        const projected = pos.clone().project(camera);
        const halfWidth = Math.min(0.18, Math.max(0.025, String(textObj.text.text).length * 0.006));
        const halfHeight = 0.035;
        const box = { left: projected.x - halfWidth, right: projected.x + halfWidth, top: projected.y + halfHeight, bottom: projected.y - halfHeight };
        const collides = !textObj.alwaysVisible && occupied.some(existing =>
          box.left < existing.right && box.right > existing.left && box.bottom < existing.top && box.top > existing.bottom
        );
        if (collides) shouldBeVisible = false;
        else occupied.push(box);
      }
      
      if (textObj.visible !== shouldBeVisible) {
        textObj.text.visible = shouldBeVisible;
        textObj.visible = shouldBeVisible;
      }
    }
  }

  /**
   * Make labels always face the camera (billboard effect)
   * Call this in the animation loop
   */
  updateBillboard(camera: THREE.Camera): void {
    for (const textObj of this.textObjects.values()) {
      if (textObj.visible) {
        // Troika-three-text handles billboarding automatically via its shader
        // but we ensure the group orientation is correct
        textObj.text.quaternion.copy(camera.quaternion);
      }
    }
  }

  /**
   * Get statistics about current rendering state
   */
  getStats(): {
    totalLabels: number;
    visibleLabels: number;
    maxLabels: number;
  } {
    let visibleCount = 0;
    for (const textObj of this.textObjects.values()) {
      if (textObj.visible) {
        visibleCount++;
      }
    }

    return {
      totalLabels: this.textObjects.size,
      visibleLabels: visibleCount,
      maxLabels: this.maxLabels,
    };
  }

  /**
   * Clean up resources
   */
  dispose(): void {
    for (const textObj of this.textObjects.values()) {
      this.group.remove(textObj.text);
      textObj.text.dispose();
    }
    this.textObjects.clear();
    this.scene.remove(this.group);
  }
}
