CREATE TABLE automation_classifications (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  automated BOOLEAN NOT NULL,
  reason_code TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('manual','known-list','heuristic')),
  policy_version TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE spatial_catalog_entities
  ADD COLUMN metrics JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE spatial_catalogs
  ADD COLUMN bot_policy_version TEXT NOT NULL DEFAULT 'bot-policy-v1',
  ADD COLUMN metric_policy_version TEXT NOT NULL DEFAULT 'galaxy-metrics-v1',
  ADD COLUMN catalog_checksum TEXT NOT NULL DEFAULT '',
  ADD COLUMN excluded_user_count BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN excluded_post_count BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN excluded_comment_count BIGINT NOT NULL DEFAULT 0;

CREATE INDEX automation_classifications_automated_idx
  ON automation_classifications(automated,user_id);

