-- appendLinks reads each visible source's links through the source index
-- (catalog_id,source,weight DESC,target). relation and directed still had to
-- come from the heap, so the heaviest links were fetched a second time by
-- primary key (about 1 s cold on production for a 5,000-node hub scene).
-- Carrying both columns in the index makes that read index-only and the
-- second fetch unnecessary. Built concurrently: production holds 20.9M rows.
-- One statement per file: the migration runner sends a concurrent index
-- build outside a transaction, and a multi-statement body would reopen one.
CREATE INDEX CONCURRENTLY IF NOT EXISTS spatial_catalog_links_source_covering_idx
  ON spatial_catalog_links(catalog_id,source,weight DESC,target)
  INCLUDE (relation,directed);
