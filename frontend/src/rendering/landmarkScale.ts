/**
 * Community landmarks are sized from their member count, which gives bodies of
 * a dozen world units. The published overview is framed on its bounding
 * sphere, so a world whose landmarks spread over tens of thousands of units is
 * viewed from far enough away that those bodies shrink below a pixel and only
 * their labels remain. Scaling landmark bodies with the overview's radius keeps
 * them a few pixels wide at the fit distance regardless of the world's extent.
 */

/** Overview radius, in world units, at and below which landmarks keep their base size. */
export const LANDMARK_REFERENCE_RADIUS = 1200;

/** Multiplier for community landmark bodies and the labels and travel poses that sit on them. */
export function landmarkBodyScale(overviewRadius: number | null | undefined): number {
  if (typeof overviewRadius !== 'number' || !Number.isFinite(overviewRadius) || overviewRadius <= 0) return 1;
  return Math.max(1, overviewRadius / LANDMARK_REFERENCE_RADIUS);
}
