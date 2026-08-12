CREATE TABLE spatial_catalogs (
  id BIGSERIAL PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('staging','published','failed')),
  source_watermark TIMESTAMPTZ NOT NULL,
  algorithm_version TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  failure_reason TEXT,
  entity_count BIGINT NOT NULL DEFAULT 0,
  link_count BIGINT NOT NULL DEFAULT 0,
  subreddit_count BIGINT NOT NULL DEFAULT 0,
  user_count BIGINT NOT NULL DEFAULT 0,
  post_count BIGINT NOT NULL DEFAULT 0,
  comment_count BIGINT NOT NULL DEFAULT 0,
  min_x DOUBLE PRECISION,
  max_x DOUBLE PRECISION,
  min_y DOUBLE PRECISION,
  max_y DOUBLE PRECISION,
  min_z DOUBLE PRECISION,
  max_z DOUBLE PRECISION,
  validation_result JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE spatial_catalog_current (
  singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
  catalog_id BIGINT NOT NULL REFERENCES spatial_catalogs(id)
);

CREATE TABLE spatial_catalog_entities (
  catalog_id BIGINT NOT NULL REFERENCES spatial_catalogs(id) ON DELETE CASCADE,
  id TEXT NOT NULL,
  label TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('subreddit','user','post','comment')),
  value BIGINT NOT NULL DEFAULT 0,
  x DOUBLE PRECISION NOT NULL,
  y DOUBLE PRECISION NOT NULL,
  z DOUBLE PRECISION NOT NULL,
  parent_id TEXT,
  anchor_id TEXT,
  author_id TEXT,
  community_id TEXT,
  position_provenance TEXT NOT NULL,
  source_updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (catalog_id,id)
);

CREATE TABLE spatial_catalog_links (
  catalog_id BIGINT NOT NULL REFERENCES spatial_catalogs(id) ON DELETE CASCADE,
  source TEXT NOT NULL,
  target TEXT NOT NULL,
  relation TEXT NOT NULL,
  directed BOOLEAN NOT NULL,
  weight BIGINT NOT NULL CHECK (weight > 0),
  PRIMARY KEY (catalog_id,source,target,relation)
);

ALTER TABLE graph_revisions
  ADD COLUMN spatial_catalog_id BIGINT REFERENCES spatial_catalogs(id);

CREATE INDEX spatial_catalogs_published_idx
  ON spatial_catalogs(published_at DESC) WHERE status='published';
CREATE INDEX spatial_catalog_entities_region_idx
  ON spatial_catalog_entities USING gist (catalog_id,x,y,z)
  WHERE type IN ('subreddit','user','post');
CREATE INDEX spatial_catalog_entities_type_value_idx
  ON spatial_catalog_entities(catalog_id,type,value DESC,id)
  WHERE type IN ('subreddit','user','post');
CREATE INDEX spatial_catalog_entities_community_idx
  ON spatial_catalog_entities(catalog_id,community_id,value DESC,id)
  WHERE community_id IS NOT NULL AND type IN ('subreddit','user','post');
CREATE INDEX spatial_catalog_entities_parent_idx
  ON spatial_catalog_entities(catalog_id,parent_id)
  WHERE parent_id IS NOT NULL;
CREATE INDEX spatial_catalog_entities_anchor_idx
  ON spatial_catalog_entities(catalog_id,anchor_id)
  WHERE anchor_id IS NOT NULL;
CREATE INDEX spatial_catalog_entities_author_idx
  ON spatial_catalog_entities(catalog_id,author_id)
  WHERE author_id IS NOT NULL;
CREATE INDEX spatial_catalog_entities_label_idx
  ON spatial_catalog_entities(catalog_id,lower(left(label,128)) text_pattern_ops)
  WHERE type IN ('subreddit','user','post');
CREATE INDEX spatial_catalog_links_source_idx
  ON spatial_catalog_links(catalog_id,source,weight DESC,target);
CREATE INDEX spatial_catalog_links_target_idx
  ON spatial_catalog_links(catalog_id,target,weight DESC,source);
