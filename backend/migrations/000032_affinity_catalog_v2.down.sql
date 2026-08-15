ALTER TABLE graph_revisions DROP COLUMN IF EXISTS scene_contract, DROP COLUMN IF EXISTS affinity_build_id;
DROP INDEX IF EXISTS public_entity_suppressions_active_idx;
DROP TABLE IF EXISTS public_entity_suppressions;
DROP INDEX IF EXISTS spatial_catalog_documents_sensitive_idx;
DROP INDEX IF EXISTS spatial_catalog_documents_type_time_idx;
DROP INDEX IF EXISTS spatial_catalog_documents_search_idx;
DROP TABLE IF EXISTS spatial_catalog_documents;
ALTER TABLE graph_revision_links DROP COLUMN IF EXISTS signal_composition, DROP COLUMN IF EXISTS observed_evidence, DROP COLUMN IF EXISTS affinity;
ALTER TABLE graph_revision_community_links DROP COLUMN IF EXISTS signal_composition, DROP COLUMN IF EXISTS observed_evidence, DROP COLUMN IF EXISTS affinity;
ALTER TABLE graph_revision_communities
  DROP COLUMN IF EXISTS evidence_metrics,
  DROP COLUMN IF EXISTS representative_members,
  DROP COLUMN IF EXISTS naming_confidence,
  DROP COLUMN IF EXISTS naming_version,
  DROP COLUMN IF EXISTS naming_method,
  DROP COLUMN IF EXISTS is_bridge,
  DROP COLUMN IF EXISTS bridge_affinity,
  DROP COLUMN IF EXISTS affinity_confidence,
  DROP COLUMN IF EXISTS distinctiveness,
  DROP COLUMN IF EXISTS secondary_color,
  DROP COLUMN IF EXISTS primary_color,
  DROP COLUMN IF EXISTS macro_group_id,
  DROP COLUMN IF EXISTS evidence_label,
  DROP COLUMN IF EXISTS display_name;
DROP TABLE IF EXISTS graph_revision_macro_group_links;
DROP TABLE IF EXISTS graph_revision_identity_events;
DROP TABLE IF EXISTS graph_revision_macro_groups;
DROP TABLE IF EXISTS cluster_label_cache;
DROP TABLE IF EXISTS historical_default_subreddits;
ALTER TABLE comments DROP COLUMN IF EXISTS source_deleted, DROP COLUMN IF EXISTS source_removed, DROP COLUMN IF EXISTS source_sensitive;
ALTER TABLE posts DROP COLUMN IF EXISTS source_deleted, DROP COLUMN IF EXISTS source_removed, DROP COLUMN IF EXISTS source_sensitive;
DROP TABLE IF EXISTS subreddit_cross_references;
DROP TABLE IF EXISTS affinity_build_macro_groups;
DROP TABLE IF EXISTS affinity_build_identity_events;
DROP TABLE IF EXISTS affinity_build_community_members;
DROP TABLE IF EXISTS affinity_build_communities;
DROP INDEX IF EXISTS affinity_edges_target_idx;
DROP INDEX IF EXISTS affinity_edges_source_idx;
DROP TABLE IF EXISTS affinity_edges;
DROP TABLE IF EXISTS affinity_builds;
