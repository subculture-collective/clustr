# ADR 0008: Place subreddits with a force simulation over shared users

- Status: accepted
- Date: 2026-11-07

## Context

ADR 0006 placed every subreddit by a fallback chain: the previous catalog's
position, the current revision's position, the strongest placed neighbour
plus a hash offset, or a hash spread over ±100,000 units on each axis. Only
about a hundred thousand subreddits were ever resident in a revision, so the
rest entered through the hash and then warm-started forever. On the November
2026 clone every one of 326,670 subreddits carries a `warm-start` position
whose per-axis spread matches a uniform cube, and community landmarks, seeded
by their own hash inside ±1,000 units, sit near the origin while their members
are scattered across the whole world. A community scene therefore draws links
spanning the universe, and the overview is a box of points with no regions.

## Decision

Subreddits that share users are laid out by a force simulation, the way a
graph view lays out notes. The model is ForceAtlas2 with LinLog attraction:
every body repels every other through a Barnes-Hut octree with a
`mass_a·mass_b/d` force, where mass is degree plus one so hubs claim space;
each overlap edge attracts its two subreddits with `k·log(1 + d/k)` scaled by
the cosine-normalised overlap (shared users over the geometric mean of both
subreddits' totals), so a hub's many weak links do not outweigh a niche
subreddit's few strong ones; and a constant gravity per unit mass keeps the
world bounded. The result is rescaled so 95% of linked subreddits lie inside
the world radius (60,000 units by default), and subreddits that share no
users sit on a deterministic shell outside it. Users, posts and comments keep
their anchors: they follow the subreddit, post or parent they already attach
to.

On the clone's 31,428 linked subreddits this model was chosen over plain
Fruchterman-Reingold, linear ForceAtlas2 and edge pruning by the share of a
body's ten nearest neighbours that are graph neighbours: 0.3% today, 4.4%
under Fruchterman-Reingold, 6.9% under linear ForceAtlas2 and 13.8% under
LinLog, whose rendering also shows distinct community islands around the
mainstream core instead of one uniform ball.

The catalog records its placement rule in `config.placement`. A catalog built
under a different rule than the current one is a cold relayout: nothing
warm-starts, every entity follows its new anchor, and the revision built on
that catalog moves each landmark to the centroid of its member subreddits.
Every later catalog is warm: placed subreddits are pinned, a new linked
subreddit starts beside its strongest placed neighbour and settles under the
same forces, and a new isolated subreddit joins the shell. Hourly revisions
take node coordinates from the catalog they reference, so the overview and
the region reads describe the same world.

The simulation is deterministic for the same inputs, runs on one goroutine
inside the catalog transaction, and writes its result through a temporary
table that the entity insert joins. `SPATIAL_CATALOG_COLD_PLACEMENT` forces a
relayout; `SPATIAL_CATALOG_LAYOUT_ITERATIONS`,
`SPATIAL_CATALOG_WARM_LAYOUT_ITERATIONS` and `SPATIAL_CATALOG_WORLD_RADIUS`
tune it.

## Consequences

The first catalog built under `force-world-3d-v2` moves every published
coordinate. Saved camera poses point at empty space once, and the landmark
continuity gate in the Catalog v2 runbook does not apply to that build; it
applies again to every build after it. Because the hourly job publishes
catalogs, deploying a backend with this rule relays out the world on its next
catalog build.

A cold build of the clone's 31,428 linked subreddits and 1.62 million
deduplicated overlap edges takes about 55 seconds of simulation on Kvant; a
warm build settles only new subreddits. `TestWorldLayoutLocalityProbe`
measures link and community locality on a clone export before and after the
layout, and writes positions for rendering, for rehearsals.
