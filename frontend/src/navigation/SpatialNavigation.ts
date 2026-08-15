export type ExplorationMode = 'overview' | 'transit' | 'approach' | 'inspect' | 'return';
export type CameraPose = { x: number; y: number; z: number; targetX: number; targetY: number; targetZ: number };
export type NavigationState = { pose: CameraPose; mode: ExplorationMode; selectedLandmark?: string; heading: number };
export type SavedViewpoint = { name: string; pose: CameraPose; selectedLandmark?: string };

export const DEFAULT_CAMERA_POSE: CameraPose = { x: 0, y: 0, z: 500, targetX: 0, targetY: 0, targetZ: 0 };
const STORAGE_KEY = 'clustr-spatial-viewpoints-v1';

export function poseForTarget(
  target: { x: number; y: number; z: number },
  options: number | { radius?: number; verticalFovDegrees?: number; minimumDistance?: number; margin?: number } = {},
): CameraPose {
  const configured = typeof options === 'number' ? { minimumDistance: options } : options;
  const radius = Math.max(0, configured.radius ?? 0);
  const fov = Math.min(120, Math.max(10, configured.verticalFovDegrees ?? 50));
  const fitDistance = radius > 0 ? radius / Math.tan((fov * Math.PI / 180) / 2) * (configured.margin ?? 1.35) : 0;
  const distance = Math.max(configured.minimumDistance ?? 180, fitDistance);
  const direction = { x: .68, y: .31, z: .66 };
  const length = Math.hypot(direction.x, direction.y, direction.z);
  const stable = (value: number) => Number(value.toFixed(3));
  return { x: stable(target.x + distance * direction.x / length), y: stable(target.y + distance * direction.y / length), z: stable(target.z + distance * direction.z / length), targetX: target.x, targetY: target.y, targetZ: target.z };
}

export function headingForPose(pose: CameraPose): number {
  return Math.atan2(pose.targetY - pose.y, pose.targetX - pose.x);
}

export function navigationState(pose: CameraPose, mode: ExplorationMode = 'overview', selectedLandmark?: string): NavigationState {
  return { pose, mode, selectedLandmark, heading: headingForPose(pose) };
}

export function poseToURL(pose: CameraPose, url = new URL(window.location.href)): URL {
  for (const [key, value] of Object.entries(pose)) url.searchParams.set(key, value.toFixed(3));
  return url;
}

export function poseFromURL(url = new URL(window.location.href)): CameraPose | null {
  const keys = ['x', 'y', 'z', 'targetX', 'targetY', 'targetZ'] as const;
  if (keys.some(key => !url.searchParams.has(key))) return null;
  const values = keys.map(key => Number(url.searchParams.get(key)));
  if (values.some(value => !Number.isFinite(value))) return null;
  return Object.fromEntries(keys.map((key, index) => [key, values[index]])) as CameraPose;
}

export function loadViewpoints(storage: Pick<Storage, 'getItem'> = localStorage): SavedViewpoint[] {
  try {
    const parsed = JSON.parse(storage.getItem(STORAGE_KEY) || '[]') as SavedViewpoint[];
    return Array.isArray(parsed) ? parsed.filter(view => view?.name && Number.isFinite(view.pose?.x)) : [];
  } catch { return []; }
}

export function saveViewpoint(viewpoint: SavedViewpoint, storage: Pick<Storage, 'getItem' | 'setItem'> = localStorage): SavedViewpoint[] {
  const views = loadViewpoints(storage).filter(view => view.name !== viewpoint.name);
  views.push(viewpoint);
  storage.setItem(STORAGE_KEY, JSON.stringify(views));
  return views;
}

export function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}
