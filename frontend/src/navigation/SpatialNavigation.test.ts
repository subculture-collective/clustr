import { describe, expect, it } from 'vitest';
import { headingForPose, poseForTarget, poseFromURL, poseToURL, saveViewpoint } from './SpatialNavigation';

describe('poseForTarget', () => {
  it('preserves the semantic target while creating an inspect pose', () => {
    const pose = poseForTarget({ x: 12, y: -4, z: 6 });
    expect(pose).toMatchObject({ targetX: 12, targetY: -4, targetZ: 6 });
    expect(pose.z).toBeGreaterThan(6);
  });

  it('round trips the complete camera pose through a URL', () => {
    const pose = poseForTarget({ x: 12, y: -4, z: 6 });
    expect(poseFromURL(poseToURL(pose, new URL('https://clustr.test/')))).toEqual(pose);
    expect(Number.isFinite(headingForPose(pose))).toBe(true);
  });

  it('does not invent a camera pose when the URL has no pose', () => {
    expect(poseFromURL(new URL('https://clustr.test/'))).toBeNull();
  });

  it('replaces a saved viewpoint with the same name', () => {
    const values = new Map<string, string>();
    const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); } };
    saveViewpoint({ name: 'home', pose: poseForTarget({ x: 0, y: 0, z: 0 }) }, storage);
    const result = saveViewpoint({ name: 'home', pose: poseForTarget({ x: 2, y: 3, z: 4 }) }, storage);
    expect(result).toHaveLength(1);
    expect(result[0].pose.targetZ).toBe(4);
  });
});
