-- Fix b-tree index overflow on TEXT[] primary-key columns. When a fine
-- community merges many old communities the concatenated array serialisation
-- can exceed the 8191-byte b-tree row limit.
ALTER TABLE affinity_build_identity_events DROP CONSTRAINT affinity_build_identity_events_pkey;
ALTER TABLE affinity_build_identity_events ADD COLUMN id BIGSERIAL;
ALTER TABLE affinity_build_identity_events ADD PRIMARY KEY (id);
CREATE INDEX affinity_build_identity_events_lookup ON affinity_build_identity_events(build_id, layer, event_type);

ALTER TABLE graph_revision_identity_events DROP CONSTRAINT graph_revision_identity_events_pkey;
ALTER TABLE graph_revision_identity_events ADD COLUMN id BIGSERIAL;
ALTER TABLE graph_revision_identity_events ADD PRIMARY KEY (id);
CREATE INDEX graph_revision_identity_events_lookup ON graph_revision_identity_events(revision_id, layer, event_type);