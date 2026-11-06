-- Restore the original TEXT[] composite primary keys.
ALTER TABLE affinity_build_identity_events DROP CONSTRAINT affinity_build_identity_events_pkey;
ALTER TABLE affinity_build_identity_events DROP COLUMN id;
ALTER TABLE affinity_build_identity_events ADD PRIMARY KEY (build_id, layer, event_type, old_ids, new_ids);
DROP INDEX affinity_build_identity_events_lookup;

ALTER TABLE graph_revision_identity_events DROP CONSTRAINT graph_revision_identity_events_pkey;
ALTER TABLE graph_revision_identity_events DROP COLUMN id;
ALTER TABLE graph_revision_identity_events ADD PRIMARY KEY (revision_id, layer, event_type, old_ids, new_ids);
DROP INDEX graph_revision_identity_events_lookup;
