#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 DATABASE_URL MIGRATIONS_DIR audit [OUTPUT_DIR]" >&2
  echo "       $0 DATABASE_URL MIGRATIONS_DIR reconcile QUALIFIED_SCHEMA_SHA256 OUTPUT_DIR" >&2
  exit 64
}

[[ $# -ge 3 && $# -le 5 ]] || usage
database_url=$1
migrations_dir=$2
mode=$3

if [[ ${mode} == audit ]]; then
  [[ $# -le 4 ]] || usage
  output_dir=${4:-.}
  qualified_schema_sha256=
elif [[ ${mode} == reconcile ]]; then
  [[ $# -eq 5 ]] || usage
  qualified_schema_sha256=$4
  output_dir=$5
  [[ ${qualified_schema_sha256} =~ ^[0-9a-f]{64}$ ]] || {
    echo "qualified schema fingerprint must be a sha256 value" >&2
    exit 65
  }
else
  usage
fi

[[ -d ${migrations_dir} ]] || {
  echo "migration directory does not exist: ${migrations_dir}" >&2
  exit 66
}
for command_name in pg_dump psql sha256sum; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "required command is missing: ${command_name}" >&2
    exit 69
  }
done

mkdir -p -m 0750 "${output_dir}"
normalized_schema=${output_dir}/schema-29-36.normalized.sql
inventory=${output_dir}/schema-29-36.inventory.tsv
pointers=${output_dir}/publication-pointers.tsv

pg_dump "${database_url}" --schema-only --no-owner --no-comments \
  | sed -E '/^--/d; /^SET /d; /^SELECT pg_catalog.set_config/d; /^\\(un)?restrict /d; /^[[:space:]]*$/d' \
  >"${normalized_schema}"
schema_sha256=$(sha256sum "${normalized_schema}" | awk '{print $1}')

psql "${database_url}" -X -v ON_ERROR_STOP=1 -AtF $'\t' >"${inventory}" <<'SQL'
WITH expected_tables(name) AS (VALUES
  ('graph_revisions'),('graph_revision_current'),('graph_revision_nodes'),
  ('graph_revision_links'),('graph_revision_communities'),
  ('graph_revision_community_members'),('graph_revision_community_links'),
  ('graph_revision_bundles'),('spatial_catalogs'),('spatial_catalog_current'),
  ('spatial_catalog_entities'),('spatial_catalog_links'),('affinity_builds'),
  ('affinity_edges'),('affinity_build_communities'),
  ('affinity_build_community_members'),('affinity_build_macro_groups'),
  ('affinity_build_identity_events'),('subreddit_cross_references'),
  ('historical_default_subreddits'),('cluster_label_cache'),
  ('graph_revision_macro_groups'),('graph_revision_macro_group_links'),
  ('graph_revision_identity_events'),('spatial_catalog_documents'),
  ('public_entity_suppressions'),('macro_group_cache')
), expected_columns(table_name,column_name) AS (VALUES
  ('spatial_catalog_entities','metrics'),
  ('spatial_catalogs','catalog_checksum'),
  ('posts','source_sensitive'),('posts','source_removed'),('posts','source_deleted'),
  ('comments','source_sensitive'),('comments','source_removed'),('comments','source_deleted'),
  ('graph_revisions','affinity_build_id'),('graph_revisions','scene_contract'),
  ('affinity_build_identity_events','id'),('graph_revision_identity_events','id')
), expected_indexes(name) AS (VALUES
  ('affinity_build_identity_events_lookup'),
  ('graph_revision_identity_events_lookup'),
  ('public_entity_suppressions_active_idx'),
  ('spatial_catalog_documents_search_idx'),
  ('spatial_catalog_entities_user_total_activity_idx')
), checks AS (
  SELECT 'table' AS kind,name, to_regclass('public.'||name) IS NOT NULL AS ok FROM expected_tables
  UNION ALL
  SELECT 'column',table_name||'.'||column_name, EXISTS (
    SELECT 1 FROM information_schema.columns c
    WHERE c.table_schema='public' AND c.table_name=e.table_name AND c.column_name=e.column_name
  ) FROM expected_columns e
  UNION ALL
  SELECT 'index',name,to_regclass('public.'||name) IS NOT NULL FROM expected_indexes
  UNION ALL
  SELECT 'partitioned','spatial_catalog_documents',EXISTS (
    SELECT 1 FROM pg_partitioned_table p JOIN pg_class c ON c.oid=p.partrelid
    JOIN pg_namespace n ON n.oid=c.relnamespace
    WHERE n.nspname='public' AND c.relname='spatial_catalog_documents' AND p.partstrat='l'
  )
  UNION ALL
  SELECT 'primary_key','affinity_build_identity_events.id',EXISTS (
    SELECT 1 FROM pg_constraint x JOIN pg_class c ON c.oid=x.conrelid
    WHERE c.relname='affinity_build_identity_events' AND x.contype='p'
      AND pg_get_constraintdef(x.oid)='PRIMARY KEY (id)'
  )
  UNION ALL
  SELECT 'primary_key','graph_revision_identity_events.id',EXISTS (
    SELECT 1 FROM pg_constraint x JOIN pg_class c ON c.oid=x.conrelid
    WHERE c.relname='graph_revision_identity_events' AND x.contype='p'
      AND pg_get_constraintdef(x.oid)='PRIMARY KEY (id)'
  )
  UNION ALL
  SELECT 'grant','reddit_cluster_user.public_entity_suppressions',
    NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='reddit_cluster_user') OR
    (has_table_privilege('reddit_cluster_user','public.public_entity_suppressions','SELECT') AND
     has_table_privilege('reddit_cluster_user','public.public_entity_suppressions','INSERT') AND
     has_table_privilege('reddit_cluster_user','public.public_entity_suppressions','UPDATE'))
)
SELECT kind,name,ok FROM checks ORDER BY kind,name;
SQL

if grep -F $'\tf' "${inventory}" >/dev/null; then
  echo "schema 29-36 inventory failed; see ${inventory}" >&2
  exit 65
fi

psql "${database_url}" -X -v ON_ERROR_STOP=1 -AtF $'\t' >"${pointers}" <<'SQL'
SELECT 'graph_revision_current',COALESCE((SELECT revision_id::text FROM graph_revision_current WHERE singleton),'missing');
SELECT 'spatial_catalog_current',COALESCE((SELECT catalog_id::text FROM spatial_catalog_current WHERE singleton),'missing');
SQL

if [[ ${mode} == reconcile ]]; then
  if [[ ${schema_sha256} != "${qualified_schema_sha256}" ]]; then
    echo "schema fingerprint differs from qualified clone: current=${schema_sha256} qualified=${qualified_schema_sha256}" >&2
    exit 65
  fi
  for migration in "${migrations_dir}"/0000{29,30,31,32,33,34,35,36}_*.up.sql; do
    [[ -f ${migration} ]] || {
      echo "missing migration file for reconciliation: ${migration}" >&2
      exit 66
    }
    filename=$(basename -- "${migration}")
    checksum=$(sha256sum "${migration}" | awk '{print $1}')
    recorded_checksum=$(psql "${database_url}" -X -v ON_ERROR_STOP=1 -Atq \
      -v filename="${filename}" -v checksum="${checksum}" <<'SQL'
INSERT INTO clustr_schema_migrations(filename,checksum_sha256)
VALUES (:'filename',:'checksum')
ON CONFLICT (filename) DO UPDATE
SET checksum_sha256=EXCLUDED.checksum_sha256
WHERE clustr_schema_migrations.checksum_sha256=EXCLUDED.checksum_sha256;
SELECT checksum_sha256 FROM clustr_schema_migrations WHERE filename=:'filename';
SQL
    )
    if [[ ${recorded_checksum} != "${checksum}" ]]; then
      echo "migration ledger checksum conflict for ${filename}" >&2
      exit 65
    fi
  done
fi

printf 'schema_sha256=%s\n' "${schema_sha256}"
printf 'inventory=%s\n' "${inventory}"
printf 'publication_pointers=%s\n' "${pointers}"
