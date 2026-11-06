-- Cache for LLM macro-grouping results, keyed by the evidence fingerprint so
-- re-running a shadow build at the same watermark reuses a validated grouping.
CREATE TABLE macro_group_cache (
  evidence_fingerprint TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  provider_model TEXT NOT NULL,
  policy_version TEXT NOT NULL,
  result JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (evidence_fingerprint, prompt_version, provider_model, policy_version)
);