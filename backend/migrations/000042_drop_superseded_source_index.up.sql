-- spatial_catalog_links_source_covering_idx (migration 41) has the same key
-- columns as spatial_catalog_links_source_idx plus the included attributes,
-- so every scan that used the old index can use the new one. Keeping both
-- would cost 2.2 GB on production and a third index to maintain on every
-- catalog build. Dropped concurrently so readers are not blocked.
DROP INDEX CONCURRENTLY IF EXISTS spatial_catalog_links_source_idx;
