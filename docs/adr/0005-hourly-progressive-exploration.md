# ADR 0005: Publish hourly and discover progressively

- Status: accepted
- Date: 2026-08-12

## Context

Loading and simulating the entire graph delays first meaning and makes server
spatial indexes irrelevant. Progressive requests can still mix worlds unless a
revision is part of every payload, cursor, and cache identity.

## Decision

Clustr targets an hourly immutable publication. Clients first fetch a manifest,
then a compact landmark overview, and request camera regions or selected
communities as they travel. Every payload names its revision. Opaque cursors
embed that revision and are rejected if reused against another world. Ordering
is deterministic and links are emitted only when both endpoints are present.

The routes are `/api/graph/manifest`, `/api/graph/overview`,
`/api/graph/region`, `/api/graph/community/{stable_id}`, and revision-to-revision
diff. `/api/graph` remains the rollback compatibility route.

## Consequences

Selection and pinned relationships outrank generic regional caps. Readiness
checks migration compatibility, active publication age, crawl lease health, and
the renderer scene-contract version. Region latency and scene integration are
release gates, not documentation targets.
