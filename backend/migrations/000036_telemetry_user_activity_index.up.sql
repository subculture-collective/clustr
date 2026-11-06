-- This migration intentionally contains one statement so PostgreSQL can build
-- the large retained-catalog index without blocking catalog writes.
CREATE INDEX CONCURRENTLY spatial_catalog_entities_user_total_activity_idx
  ON spatial_catalog_entities(catalog_id,(COALESCE((metrics->>'activity_count')::bigint,0)) DESC,id)
  WHERE type='user';
