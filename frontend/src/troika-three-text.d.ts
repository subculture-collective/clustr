declare module 'troika-three-text' {
  import { Mesh, Material } from 'three';

  export function configureTextBuilder(config: { useWorker?: boolean }): void;

  export class Text extends Mesh {
    text: string;
    font: string;
    fontSize: number;
    color: string | number;
    anchorX: 'left' | 'center' | 'right' | number;
    anchorY: 'top' | 'middle' | 'bottom' | 'top-baseline' | 'top-cap' | 'top-ex' | number;
    outlineWidth: number | string;
    outlineColor: string | number;
    outlineOpacity: number;
    material: Material;
    sync(callback?: () => void): void;
    dispose(): void;
  }

  /**
   * Renders every Text added to it in one draw call per pass. Members keep
   * their own text, font size, colour, outline and local transform; the
   * batch's material is used for all of them.
   */
  export class BatchedText extends Text {}
}
