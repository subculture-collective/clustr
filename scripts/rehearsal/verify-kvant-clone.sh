#!/usr/bin/env bash
set -euo pipefail

rehearsal_root=${1:-/mnt/data2/clustr-rehearsal/2026-08-12-launch-readiness}
host_port=${CLUSTR_REHEARSAL_PORT:-55432}
database_name=${CLUSTR_REHEARSAL_DB:-reddit_cluster}
database_user=${CLUSTR_REHEARSAL_USER:-clustr_rehearsal}
password_file="${rehearsal_root}/postgres-password"

if [[ ! -s "${password_file}" ]]; then
  echo "missing rehearsal credential file: ${password_file}" >&2
  exit 1
fi

export PGPASSWORD
PGPASSWORD=$(<"${password_file}")

psql \
  --host 127.0.0.1 \
  --port "${host_port}" \
  --username "${database_user}" \
  --dbname "${database_name}" \
  --no-psqlrc \
  --tuples-only \
  --set ON_ERROR_STOP=1 <<'SQL'
SELECT 'database_size', pg_size_pretty(pg_database_size(current_database()));
SELECT 'subreddits', count(*) FROM subreddits;
SELECT 'posts', count(*) FROM posts;
SELECT 'comments', count(*) FROM comments;
SELECT 'graph_nodes', count(*) FROM graph_nodes;
SELECT 'graph_links', count(*) FROM graph_links;
SELECT 'crawl_jobs', count(*) FROM crawl_jobs;
SQL
