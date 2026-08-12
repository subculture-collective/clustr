# ADR 0003: Keep rendering web-first and instanced

- Status: accepted
- Date: 2026-08-12

## Context

Clustr is a web application with existing React views and APIs. The legacy graph
renderer is selected by default and performs poorly at the desired scale, while
an incomplete custom Three.js Adapter already uses GPU instancing.

## Decision

The default experience is a full-screen instanced Three.js spatial explorer.
Render Adapters consume one spatial-scene Interface containing a revision,
typed positions, visible entities, attention, and camera pose. Published worlds
are never re-simulated in the browser. Semantic LOD is far communities, medium
subreddits/significant users, near local relationships, and inspect content.

`VITE_USE_LEGACY_RENDERER=true` is a temporary rollback. The old renderer is
removed only after hardware-accelerated browser qualification and one retention
window. A future Unity Adapter may consume the same service contracts, but Unity
is not part of this program.

## Consequences

Performance evidence must include frame time, meaningful visible entities,
scene residency, memory, and interaction latency on the documented reference
desktop. Canvas existence is not acceptance evidence.
