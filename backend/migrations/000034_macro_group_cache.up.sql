-- Compatibility table for previously staged provider-grouping artifacts. The
-- corrected affinity builder does not read or write this table; keep it during
-- the rollback window because production already contains the additive object.
CREATE TABLE macro_group_cache (
  evidence_fingerprint TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  provider_model TEXT NOT NULL,
  policy_version TEXT NOT NULL,
  result JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (evidence_fingerprint, prompt_version, provider_model, policy_version)
);
