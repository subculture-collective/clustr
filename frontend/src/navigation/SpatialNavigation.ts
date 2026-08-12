export type ExplorationMode = 'overview' | 'transit' | 'approach' | 'inspect' | 'return';
export type CameraPose = { x: number; y: number; z: number; targetX: number; targetY: number; targetZ: number };
export type NavigationState = { pose: CameraPose; mode: ExplorationMode; selectedLandmark?: string; heading: number };
export type SavedViewpoint = { name: string; pose: CameraPose; selectedLandmark?: string };

export const DEFAULT_CAMERA_POSE: CameraPose = { x: 0, y: 0, z: 500, targetX: 0, targetY: 0, targetZ: 0 };
const STORAGE_KEY = 'clustr-spatial-viewpoints-v1';

export function poseForTarget(target: { x: number; y: number; z: number }, distance = 180): CameraPose {
  return { x: target.x + distance, y: target.y + distance * 0.45, z: target.z + distance, targetX: target.x, targetY: target.y, targetZ: target.z };
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
