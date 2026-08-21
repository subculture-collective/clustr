DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reddit_cluster_user') THEN
    REVOKE SELECT,INSERT,UPDATE ON TABLE public_entity_suppressions FROM reddit_cluster_user;
  END IF;
END
$$;
