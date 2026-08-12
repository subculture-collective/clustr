# ADR 0002: Use semantic 3D with stable landmarks

- Status: accepted
- Date: 2026-08-12

## Context

Planar force coordinates with arbitrary visual Z do not provide a navigable
world. Re-running an unseeded layout also moves familiar communities, making
saved viewpoints and progressive region discovery unreliable.

## Decision

Community landmarks define a persistent global coordinate frame. Communities
are matched to the prior revision by deterministic weighted membership overlap.
Matched landmarks and nodes warm-start from published coordinates; new objects
are seeded from revision-independent hashes near connected landmarks. A seeded
three-axis layout may refine local topology within bounded community volumes.

The layout Module owns octree bounds, coincident-point buckets, deterministic
seed/config, provenance, and validation. Layout precedes centroids, bundles, and
spatial indexes. At least 95% of matched landmarks must move no more than 10% of
the prior median inter-landmark distance unless a validated split or merge says
otherwise.

## Consequences

Continuity takes precedence over a globally optimal force minimum. Structural
splits and merges are explicit publication facts. Invalid, non-finite, planar,
or collapsed worlds fail publication rather than being repaired by the client.
