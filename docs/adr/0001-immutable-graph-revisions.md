# ADR 0001: Publish immutable graph revisions

- Status: accepted
- Date: 2026-08-12

## Context

The mutable graph tables mix calculation, publication, diffing, and reads. A
failed or concurrent calculation can expose partial topology and advance a
watermark that was never safely published.

## Decision

Every calculation starts from one source watermark and writes revision-scoped
projection, hierarchy, layout, bundles, counts, bounds, and validation results.
Only a validated revision may become `graph_revision_current`; that pointer is
promoted in the transaction containing all revision artifacts. Published rows
are immutable. A failed staging record is retained for diagnosis and cannot
advance the pointer or the reader-visible watermark.

The public Interface is full-publication-equivalent: internal incremental
calculation is permitted only when it produces the same revision as a full
calculation at the same watermark. Diffs compare immutable revisions in SQL.

## Consequences

Readers can pin one coherent world and cache by revision. Publication consumes
additional storage, so retention must never delete a current or referenced
revision. The old mutable tables remain a build workspace during the online
cutover and are not a new read model.

## Rollout and rollback

Shadow revisions are compared before `REVISION_READS_ENABLED` is enabled. The
compatibility `/api/graph` route remains available for one retention window.
Rollback changes the read flag/pointer; it does not mutate a published revision.
