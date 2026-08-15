declare module 'troika-three-text' {
  import { Mesh, Material } from 'three';

  export function configureTextBuilder(config: { useWorker?: boolean }): void;

  export class Text extends Mesh {
    text: string;
    fontSize: number;
    color: string | number;
    anchorX: 'left' | 'center' | 'right' | number;
    anchorY: 'top' | 'middle' | 'bottom' | 'top-baseline' | 'top-cap' | 'top-ex' | number;
    outlineWidth: number | string;
    outlineColor: string | number;
    outlineOpacity: number;
    material: Material;
    sync(): void;
    dispose(): void;
  }
}
