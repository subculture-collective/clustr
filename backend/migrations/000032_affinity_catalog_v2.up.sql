-- Additive schema for the shadow-built affinity world and immutable public
-- Catalog. This migration does not move either current publication pointer.

CREATE TABLE affinity_builds (
  id BIGSERIAL PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('staging','validated','failed','published')),
  source_watermark TIMESTAMPTZ NOT NULL,
  seed BIGINT NOT NULL DEFAULT 0,
  algorithm_version TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  effective_weights JSONB NOT NULL DEFAULT '{}'::jsonb,
  validation_result JSONB NOT NULL DEFAULT '{}'::jsonb,
  failure_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE TABLE affinity_edges (
  build_id BIGINT NOT NULL REFERENCES affinity_builds(id) ON DELETE CASCADE,
  source_subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
  target_subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
  affinity DOUBLE PRECISION NOT NULL CHECK (affinity >= 0 AND affinity <= 1 AND affinity = affinity),
  raw_affinity DOUBLE PRECISION NOT NULL CHECK (raw_affinity >= 0 AND raw_affinity < 'Infinity'::float8),
  observed_evidence DOUBLE PRECISION NOT NULL CHECK (observed_evidence >= 0 AND observed_evidence < 'Infinity'::float8),
  signal_composition JSONB NOT NULL DEFAULT '{}'::jsonb,
  rank_from_source SMALLINT CHECK (rank_from_source BETWEEN 1 AND 24),
  rank_from_target SMALLINT CHECK (rank_from_target BETWEEN 1 AND 24),
  PRIMARY KEY (build_id,source_subreddit_id,target_subreddit_id),
  CHECK (source_subreddit_id < target_subreddit_id),
  CHECK (rank_from_source IS NOT NULL OR rank_from_target IS NOT NULL)
);

CREATE INDEX affinity_edges_source_idx ON affinity_edges(build_id,source_subreddit_id,affinity DESC,target_subreddit_id);
CREATE INDEX affinity_edges_target_idx ON affinity_edges(build_id,target_subreddit_id,affinity DESC,source_subreddit_id);

CREATE TABLE affinity_build_communities (
  build_id BIGINT NOT NULL REFERENCES affinity_builds(id) ON DELETE CASCADE,
  community_id TEXT NOT NULL,
  macro_group_id TEXT,
  display_name TEXT NOT NULL,
  evidence_label TEXT NOT NULL,
  primary_color TEXT NOT NULL,
  secondary_color TEXT,
  is_bridge BOOLEAN NOT NULL DEFAULT false,
  distinctiveness DOUBLE PRECISION NOT NULL DEFAULT 0,
  affinity_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  naming_method TEXT NOT NULL,
  naming_version TEXT NOT NULL,
  naming_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  representative_members JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY(build_id,community_id)
);

CREATE TABLE affinity_build_community_members (
  build_id BIGINT NOT NULL,
  community_id TEXT NOT NULL,
  subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
  PRIMARY KEY(build_id,community_id,subreddit_id),
  FOREIGN KEY(build_id,community_id) REFERENCES affinity_build_communities(build_id,community_id) ON DELETE CASCADE
);

CREATE TABLE affinity_build_macro_groups (
  build_id BIGINT NOT NULL REFERENCES affinity_builds(id) ON DELETE CASCADE,
  macro_group_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  evidence_label TEXT NOT NULL,
  palette_role SMALLINT NOT NULL CHECK (palette_role BETWEEN 0 AND 11),
  primary_color TEXT NOT NULL,
  evidence_metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY(build_id,macro_group_id)
);

CREATE TABLE affinity_build_identity_events (
  build_id BIGINT NOT NULL REFERENCES affinity_builds(id) ON DELETE CASCADE,
  layer TEXT NOT NULL CHECK (layer IN ('fine','macro')),
  event_type TEXT NOT NULL CHECK (event_type IN ('split','merge','new','retired')),
  old_ids TEXT[] NOT NULL DEFAULT '{}',
  new_ids TEXT[] NOT NULL DEFAULT '{}',
  evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY(build_id,layer,event_type,old_ids,new_ids)
);

CREATE TABLE subreddit_cross_references (
  source_post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  source_subreddit_id INTEGER NOT NULL REFERENCES subreddits(id),
  target_post_id TEXT NOT NULL DEFAULT '',
  target_subreddit_name TEXT NOT NULL DEFAULT '',
  target_subreddit_id INTEGER REFERENCES subreddits(id),
  relation TEXT NOT NULL CHECK (relation IN ('crosspost','reference')),
  observed_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (source_post_id,relation,target_post_id)
);

ALTER TABLE posts
  ADD COLUMN source_sensitive BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN source_removed BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN source_deleted BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE comments
  ADD COLUMN source_sensitive BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN source_removed BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN source_deleted BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE historical_default_subreddits (
  subreddit_name TEXT PRIMARY KEY,
  source_version TEXT NOT NULL,
  source_note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE cluster_label_cache (
  evidence_fingerprint TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  provider_model TEXT NOT NULL,
  policy_version TEXT NOT NULL,
  display_name TEXT NOT NULL,
  evidence_label TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1 AND confidence = confidence),
  method TEXT NOT NULL CHECK (method IN ('provider','deterministic-fallback')),
  grounding JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (evidence_fingerprint,prompt_version,provider_model,policy_version)
);

CREATE TABLE graph_revision_macro_groups (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  macro_group_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  evidence_label TEXT NOT NULL,
  palette_role SMALLINT NOT NULL CHECK (palette_role BETWEEN 0 AND 11),
  primary_color TEXT NOT NULL CHECK (primary_color ~ '^#[0-9A-Fa-f]{6}$'),
  member_count INTEGER NOT NULL CHECK (member_count >= 0),
  representative_clusters JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (revision_id,macro_group_id)
);

CREATE TABLE graph_revision_macro_group_links (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  source_macro_group_id TEXT NOT NULL,
  target_macro_group_id TEXT NOT NULL,
  affinity DOUBLE PRECISION NOT NULL CHECK (affinity >= 0 AND affinity <= 1 AND affinity = affinity),
  observed_evidence DOUBLE PRECISION NOT NULL CHECK (observed_evidence >= 0 AND observed_evidence < 'Infinity'::float8),
  PRIMARY KEY (revision_id,source_macro_group_id,target_macro_group_id),
  FOREIGN KEY (revision_id,source_macro_group_id) REFERENCES graph_revision_macro_groups(revision_id,macro_group_id) ON DELETE CASCADE,
  FOREIGN KEY (revision_id,target_macro_group_id) REFERENCES graph_revision_macro_groups(revision_id,macro_group_id) ON DELETE CASCADE,
  CHECK (source_macro_group_id < target_macro_group_id)
);

CREATE TABLE graph_revision_identity_events (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  layer TEXT NOT NULL CHECK (layer IN ('fine','macro')),
  event_type TEXT NOT NULL CHECK (event_type IN ('split','merge','new','retired')),
  old_ids TEXT[] NOT NULL DEFAULT '{}',
  new_ids TEXT[] NOT NULL DEFAULT '{}',
  evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY(revision_id,layer,event_type,old_ids,new_ids)
);

ALTER TABLE graph_revision_communities
  ADD COLUMN display_name TEXT,
  ADD COLUMN evidence_label TEXT,
  ADD COLUMN macro_group_id TEXT,
  ADD COLUMN primary_color TEXT,
  ADD COLUMN secondary_color TEXT,
  ADD COLUMN distinctiveness DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (distinctiveness >= 0 AND distinctiveness <= 1 AND distinctiveness = distinctiveness),
  ADD COLUMN affinity_confidence DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (affinity_confidence >= 0 AND affinity_confidence <= 1 AND affinity_confidence = affinity_confidence),
  ADD COLUMN bridge_affinity DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (bridge_affinity >= 0 AND bridge_affinity <= 1 AND bridge_affinity = bridge_affinity),
  ADD COLUMN is_bridge BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN naming_method TEXT NOT NULL DEFAULT 'legacy',
  ADD COLUMN naming_version TEXT NOT NULL DEFAULT 'legacy-v1',
  ADD COLUMN naming_confidence DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (naming_confidence >= 0 AND naming_confidence <= 1 AND naming_confidence = naming_confidence),
  ADD COLUMN representative_members JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN evidence_metrics JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE graph_revision_community_links
  ADD COLUMN affinity DOUBLE PRECISION,
  ADD COLUMN observed_evidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  ADD COLUMN signal_composition JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE graph_revision_links
  ADD COLUMN affinity DOUBLE PRECISION,
  ADD COLUMN observed_evidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  ADD COLUMN signal_composition JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE spatial_catalog_documents (
  catalog_id BIGINT NOT NULL,
  entity_id TEXT NOT NULL,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('subreddit','user','post','comment')),
  title TEXT,
  body TEXT,
  description TEXT,
  source_permalink TEXT,
  source_sensitive BOOLEAN NOT NULL DEFAULT false,
  administrative_sensitive BOOLEAN NOT NULL DEFAULT false,
  content_created_at TIMESTAMPTZ,
  content_updated_at TIMESTAMPTZ NOT NULL,
  provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
  search_vector TSVECTOR GENERATED ALWAYS AS (
    setweight(to_tsvector('simple',COALESCE(title,'')),'A') ||
    setweight(to_tsvector('simple',COALESCE(description,'')),'B') ||
    setweight(to_tsvector('simple',COALESCE(body,'')),'C')
  ) STORED,
  PRIMARY KEY (catalog_id,entity_id),
  FOREIGN KEY (catalog_id,entity_id) REFERENCES spatial_catalog_entities(catalog_id,id) ON DELETE CASCADE
);

CREATE INDEX spatial_catalog_documents_search_idx ON spatial_catalog_documents USING gin(search_vector);
CREATE INDEX spatial_catalog_documents_type_time_idx ON spatial_catalog_documents(catalog_id,entity_type,content_updated_at DESC,entity_id);
CREATE INDEX spatial_catalog_documents_sensitive_idx ON spatial_catalog_documents(catalog_id,entity_type,entity_id)
  WHERE source_sensitive OR administrative_sensitive;

-- Suppression is deliberately outside immutable artifacts and is joined at
-- read time. Restoring closes the interval without rewriting a catalog.
CREATE TABLE public_entity_suppressions (
  entity_id TEXT PRIMARY KEY,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('community','subreddit','user','post','comment')),
  reason_category TEXT NOT NULL CHECK (reason_category IN ('opt_out','legal','privacy','safety','administrative')),
  operator_note TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  restored_by TEXT,
  restored_at TIMESTAMPTZ,
  CHECK ((restored_at IS NULL) = (restored_by IS NULL))
);

CREATE INDEX public_entity_suppressions_active_idx ON public_entity_suppressions(entity_type,entity_id) WHERE restored_at IS NULL;

ALTER TABLE graph_revisions
  ADD COLUMN affinity_build_id BIGINT REFERENCES affinity_builds(id),
  ADD COLUMN scene_contract TEXT NOT NULL DEFAULT 'spatial-scene-v1';
