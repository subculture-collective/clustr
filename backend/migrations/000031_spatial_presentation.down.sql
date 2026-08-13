DROP INDEX IF EXISTS automation_classifications_automated_idx;

ALTER TABLE spatial_catalogs
  DROP COLUMN IF EXISTS excluded_comment_count,
  DROP COLUMN IF EXISTS excluded_post_count,
  DROP COLUMN IF EXISTS excluded_user_count,
  DROP COLUMN IF EXISTS catalog_checksum,
  DROP COLUMN IF EXISTS metric_policy_version,
  DROP COLUMN IF EXISTS bot_policy_version;

ALTER TABLE spatial_catalog_entities
  DROP COLUMN IF EXISTS metrics;

DROP TABLE IF EXISTS automation_classifications;
