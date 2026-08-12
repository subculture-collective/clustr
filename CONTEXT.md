# Clustr domain language

- **Crawl request**: the durable desire to keep one subreddit fresh; it is due
  repeatedly and is not an individual execution.
- **Crawl attempt**: a leased, append-only execution of a crawl request.
- **Discovery candidate**: a subreddit suggested by a mention or author
  history before the crawler promotes it.
- **Source watermark**: the latest source-data point included in a graph build.
- **Graph projection**: the typed, weighted graph derived from crawled source
  records.
- **Graph revision**: one immutable, coherent published graph projection.
- **Community landmark**: a stable community identity and anchor in the spatial
  world.
- **Spatial world**: the revision-scoped 3D placement used for exploration.
- **Spatial catalog**: an immutable, full-corpus placement containing every
  source entity, its semantic label, parent or anchor, community assignment,
  normalized relationships, and deterministic 3D coordinates. Multiple hourly
  graph revisions may reference one catalog until the next catalog publication.
- **Spatial entity**: one subreddit, user, post, or comment represented in a
  spatial catalog. Every spatial entity has a stable ID, semantic label, LOD
  type, value, coordinate provenance, and finite XYZ position.
- **Semantic label**: the human-readable name selected for a spatial entity:
  subreddit name/title, username, post title, or bounded comment excerpt.
- **LOD level**: the semantic level of detail selected for a view of the
  spatial world.
- **Publication**: the atomic act that makes a validated graph revision active.
