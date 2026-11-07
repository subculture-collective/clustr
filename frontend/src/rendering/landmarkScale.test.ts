import { describe, expect, it } from 'vitest';
import { LANDMARK_REFERENCE_RADIUS, landmarkBodyScale } from './landmarkScale';

describe('landmarkBodyScale', () => {
  it('keeps the base size for small worlds and unknown extents', () => {
    expect(landmarkBodyScale(null)).toBe(1);
    expect(landmarkBodyScale(undefined)).toBe(1);
    expect(landmarkBodyScale(0)).toBe(1);
    expect(landmarkBodyScale(Number.NaN)).toBe(1);
    expect(landmarkBodyScale(LANDMARK_REFERENCE_RADIUS / 2)).toBe(1);
    expect(landmarkBodyScale(LANDMARK_REFERENCE_RADIUS)).toBe(1);
  });

  it('grows linearly with the overview radius so bodies keep their pixel size at the fit distance', () => {
    expect(landmarkBodyScale(LANDMARK_REFERENCE_RADIUS * 10)).toBe(10);
    // The force-world catalog published on 2026-11-07 frames 300 landmarks in a 44,163-unit sphere.
    expect(landmarkBodyScale(44163)).toBeCloseTo(36.8, 1);
  });
});
