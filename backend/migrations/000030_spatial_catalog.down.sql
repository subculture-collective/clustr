ALTER TABLE graph_revisions DROP COLUMN IF EXISTS spatial_catalog_id;
DROP TABLE IF EXISTS spatial_catalog_links;
DROP TABLE IF EXISTS spatial_catalog_entities;
DROP TABLE IF EXISTS spatial_catalog_current;
DROP TABLE IF EXISTS spatial_catalogs;
