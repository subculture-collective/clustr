import * as THREE from 'three';
import { Text, configureTextBuilder } from 'troika-three-text';

// Clustr serves a bounded local font and CSP intentionally disallows Troika's
// worker/bootstrap path. Configure this before the first glyph request.
configureTextBuilder({ useWorker: false });

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
  /**
   * Called when a label's glyph geometry becomes ready. Troika builds text
   * asynchronously, so a renderer that draws on demand must draw again once
   * the label can actually be seen.
   */
  onSync?: () => void;
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
  private viewportHeight = 900;
  private readonly onSync: (() => void) | undefined;
  /** Screen height, in CSS pixels, of a label with size 1. */
  private static readonly BASE_SCREEN_HEIGHT_PX = 12;

  constructor(scene: THREE.Scene, config: SDFTextRendererConfig = {}) {
    this.scene = scene;
    this.maxLabels = config.maxLabels || 500;
    this.fontSize = config.fontSize || 8;
    this.color = config.color || '#ffffff';
    this.backgroundColor = config.backgroundColor || '#000000';
    this.onSync = config.onSync;

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
        // The label sits on its anchor point, which callers place at the top
        // of the node, so a screen-sized label never covers a small node.
        text.anchorY = 'bottom';
        
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

      // Trigger text sync (troika batches updates) and ask for a frame once
      // the glyphs exist.
      textObj.text.sync(this.onSync);
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

  /** Viewport height in CSS pixels, used to keep labels a fixed size on screen. */
  setViewportHeight(height: number): void {
    if (Number.isFinite(height) && height > 0) this.viewportHeight = height;
  }

  /**
   * Face the camera and keep a constant size on screen.
   * Call this on every drawn frame. Published worlds span ±100,000 units
   * while labels are a few units tall, so a world-sized label is invisible
   * from any useful distance; instead each visible label is scaled so a
   * size-1 label is BASE_SCREEN_HEIGHT_PX tall wherever it is.
   */
  updateBillboard(camera: THREE.Camera): void {
    const perspective = camera instanceof THREE.PerspectiveCamera ? camera : null;
    const worldPerPixelAtUnitDistance = perspective
      ? (2 * Math.tan(THREE.MathUtils.degToRad(perspective.fov / 2))) / this.viewportHeight
      : 0;
    for (const textObj of this.textObjects.values()) {
      if (!textObj.visible) continue;
      const text = textObj.text;
      text.quaternion.copy(camera.quaternion);
      if (!perspective) continue;
      const distance = camera.position.distanceTo(text.position);
      // fontSize already carries the label's size factor, so the target
      // height scales with it too.
      const targetWorldHeight = (text.fontSize / this.fontSize) * SDFTextRenderer.BASE_SCREEN_HEIGHT_PX * worldPerPixelAtUnitDistance * distance;
      const scale = THREE.MathUtils.clamp(targetWorldHeight / text.fontSize, 1e-4, 1e6);
      text.scale.setScalar(scale);
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
