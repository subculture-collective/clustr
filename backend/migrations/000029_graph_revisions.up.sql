-- Immutable, published graph snapshots.  The live graph_* tables remain the build
-- workspace; readers only follow graph_revision_current.
CREATE TABLE graph_revisions (
  id BIGSERIAL PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('staging','published','failed')),
  source_watermark TIMESTAMPTZ NOT NULL,
	algorithm_version TEXT NOT NULL DEFAULT 'projection-v1',
	config JSONB NOT NULL DEFAULT '{}'::jsonb,
  layout_algorithm TEXT NOT NULL,
  layout_seed BIGINT NOT NULL,
  layout_dimensions SMALLINT NOT NULL CHECK (layout_dimensions = 3),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
	failure_reason TEXT,
	node_count BIGINT NOT NULL DEFAULT 0,
	link_count BIGINT NOT NULL DEFAULT 0,
	community_count BIGINT NOT NULL DEFAULT 0,
	min_x DOUBLE PRECISION,
	max_x DOUBLE PRECISION,
	min_y DOUBLE PRECISION,
	max_y DOUBLE PRECISION,
	min_z DOUBLE PRECISION,
	max_z DOUBLE PRECISION,
	validation_result JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE TABLE graph_revision_current (singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton), revision_id BIGINT NOT NULL REFERENCES graph_revisions(id));
CREATE TABLE graph_revision_nodes (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  id TEXT NOT NULL, name TEXT NOT NULL, value BIGINT NOT NULL DEFAULT 0, type TEXT NOT NULL,
  x DOUBLE PRECISION NOT NULL, y DOUBLE PRECISION NOT NULL, z DOUBLE PRECISION NOT NULL,
	position_provenance TEXT NOT NULL DEFAULT 'deterministic-3d',
  PRIMARY KEY (revision_id,id)
);
CREATE TABLE graph_revision_links (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  source TEXT NOT NULL, target TEXT NOT NULL, relation TEXT NOT NULL DEFAULT 'coactivity', directed BOOLEAN NOT NULL, weight BIGINT NOT NULL DEFAULT 1 CHECK (weight > 0),
  PRIMARY KEY (revision_id,source,target,relation),
  FOREIGN KEY (revision_id,source) REFERENCES graph_revision_nodes(revision_id,id) ON DELETE CASCADE,
  FOREIGN KEY (revision_id,target) REFERENCES graph_revision_nodes(revision_id,id) ON DELETE CASCADE
);
CREATE TABLE graph_revision_communities (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  community_id TEXT NOT NULL, parent_id TEXT, level SMALLINT NOT NULL DEFAULT 0, label TEXT NOT NULL, size INTEGER NOT NULL,
  x DOUBLE PRECISION NOT NULL DEFAULT 0, y DOUBLE PRECISION NOT NULL DEFAULT 0, z DOUBLE PRECISION NOT NULL DEFAULT 0,
  PRIMARY KEY (revision_id,community_id)
);
CREATE TABLE graph_revision_community_members (revision_id BIGINT NOT NULL, community_id TEXT NOT NULL, node_id TEXT NOT NULL, PRIMARY KEY(revision_id,community_id,node_id), FOREIGN KEY(revision_id,community_id) REFERENCES graph_revision_communities(revision_id,community_id) ON DELETE CASCADE, FOREIGN KEY(revision_id,node_id) REFERENCES graph_revision_nodes(revision_id,id) ON DELETE CASCADE);
CREATE TABLE graph_revision_community_links (revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE, source_community_id TEXT NOT NULL, target_community_id TEXT NOT NULL, weight BIGINT NOT NULL CHECK(weight>0), PRIMARY KEY(revision_id,source_community_id,target_community_id));
CREATE TABLE graph_revision_bundles (
  revision_id BIGINT NOT NULL REFERENCES graph_revisions(id) ON DELETE CASCADE,
  source_community_id TEXT NOT NULL,
  target_community_id TEXT NOT NULL,
  weight BIGINT NOT NULL CHECK(weight > 0),
  control_x DOUBLE PRECISION NOT NULL,
  control_y DOUBLE PRECISION NOT NULL,
  control_z DOUBLE PRECISION NOT NULL,
  PRIMARY KEY(revision_id,source_community_id,target_community_id),
  FOREIGN KEY(revision_id,source_community_id) REFERENCES graph_revision_communities(revision_id,community_id) ON DELETE CASCADE,
  FOREIGN KEY(revision_id,target_community_id) REFERENCES graph_revision_communities(revision_id,community_id) ON DELETE CASCADE
);
CREATE INDEX graph_revision_nodes_region_idx ON graph_revision_nodes(revision_id,x,y,z,id);
CREATE INDEX graph_revision_links_revision_idx ON graph_revision_links(revision_id,source,target);
CREATE INDEX graph_revisions_published_idx ON graph_revisions(published_at DESC) WHERE status='published';
