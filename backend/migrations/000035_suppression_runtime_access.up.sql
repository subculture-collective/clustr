-- Migration 32 may be applied by a database migration owner that differs from
-- the API runtime role. Grant only the privileges required by public reads and
-- authenticated suppression create/restore operations when that role exists.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reddit_cluster_user') THEN
    GRANT SELECT,INSERT,UPDATE ON TABLE public_entity_suppressions TO reddit_cluster_user;
  END IF;
END
$$;
