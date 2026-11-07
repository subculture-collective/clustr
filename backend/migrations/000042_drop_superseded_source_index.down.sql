CREATE INDEX CONCURRENTLY IF NOT EXISTS spatial_catalog_links_source_idx
  ON spatial_catalog_links(catalog_id,source,weight DESC,target);
