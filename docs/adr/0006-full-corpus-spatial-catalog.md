# ADR 0006: Share immutable full-corpus spatial catalogs across revisions

- Status: accepted
- Date: 2026-08-12

## Context

The launch graph revision deliberately caps resident topology at 100,000 nodes,
but the source contains millions of users, posts, and comments. Copying every
coordinate, label, and semantic relationship into every hourly graph revision
would multiply mostly unchanged data by the retention window. Raising renderer
or revision caps would also make the browser responsible for storage policy.

## Decision

A Spatial Catalog is an immutable full-corpus placement published independently
of the hourly graph revision. It contains every source entity at its source
watermark, its semantic label, parent or anchor, community assignment,
coordinate provenance, deterministic XYZ position, and normalized semantic
relationships. A graph revision atomically references one published catalog.
Several graph revisions may share a catalog until a newer catalog is validated.

The Spatial Catalog Module owns full-corpus extraction, deterministic anchored
placement, semantic relationship projection, counts, bounds, validation,
publication, and retention behind one Interface. PostgreSQL is the durable
Adapter. The browser never consumes the full catalog: revision-pinned overview,
region, community, search, and inspect reads select bounded LOD slices.
The region, community, and label-prefix indexes cover navigational subreddits,
users, and posts. Comments remain fully placed, directly addressable, and
semantically labeled but are revealed through a selected post/entity
neighborhood. This avoids multi-million-row secondary-index write
amplification for objects that are never useful at free-camera LOD.
Parent, post anchor, and author are canonical entity references. Read Adapters
project them as `published_in`, `comment_on`, `reply`, and `authorship` links;
only cross-cutting user activity and subreddit overlap are materialized in the
link table. This removes duplicate rows without weakening the scene contract.
Link endpoints are validated in one set-based publication check instead of two
per-row foreign-key probes during the multi-million-row insert. No catalog can
be promoted with an orphan endpoint.

Existing entities warm-start from the preceding catalog. New subreddits use a
published community or related-subreddit anchor where available. Users use
their strongest activity anchor. Posts use subreddit anchors and comments use
their post or parent. Deterministic hash offsets make placement reproducible.

## Consequences

Every crawled entity can be found and visited without making every entity
resident or labeled simultaneously. Catalog builds are heavier than hourly
revision builds and run as a bounded Kvant job. Catalog retention excludes the
current catalog and any catalog referenced by a retained graph revision.

Catalog publication and graph-revision publication are separate atomic acts.
Failure cannot replace either current pointer. A newly published catalog is not
reader-visible until a graph revision referencing it is promoted.

## Rollout and rollback

Migration creates catalog tables and a nullable catalog reference on graph
revisions. Readers fall back to revision-scoped nodes and links when the
reference is null. The first full catalog is built and qualified on the Almaz
clone, then a shadow graph revision references it. Rollback repoints the graph
revision or disables revision reads; catalog rows remain immutable.
