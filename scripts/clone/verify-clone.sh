#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

export_dir=${1:-}
container=${2:-}
clone_absolute_dir "$export_dir"
[[ -n $container ]] || clone_die "clone container is required"
for command_name in docker jq sha256sum; do clone_require_command "$command_name"; done
(cd "$export_dir" && sha256sum --check --strict SHA256SUMS >/dev/null)
database=$(jq -er '.database' "${export_dir}/manifest.json")
clone_validate_name "$database"

actual_core=$(docker exec -i "$container" psql -X -v ON_ERROR_STOP=1 -U clustr_clone -d "$database" -Atq <<'SQL'
SELECT json_build_object(
  'database', current_database(),
  'server_version_num', current_setting('server_version_num')::integer,
  'database_size_bytes', pg_database_size(current_database()),
  'encoding', pg_encoding_to_char(d.encoding),
  'collation', d.datcollate,
  'extensions', (SELECT json_agg(extname ORDER BY extname) FROM pg_extension),
  'tables', json_build_object(
    'subreddits', (SELECT count(*) FROM subreddits),
    'users', (SELECT count(*) FROM users),
    'posts', (SELECT count(*) FROM posts),
    'comments', (SELECT count(*) FROM comments)
  )
)::text FROM pg_database d WHERE d.datname=current_database();
SQL
)
actual_migration=$(docker exec "$container" sh -c \
  'if psql -X -U clustr_clone -d "$1" -Atq -c "select to_regclass('\''public.clustr_schema_migrations'\'') is not null" | grep -qx t; then psql -X -U clustr_clone -d "$1" -Atq -c "select coalesce(max(filename),'\''empty-ledger'\'') from clustr_schema_migrations"; else echo legacy-untracked; fi' \
  sh "$database")
actual=$(jq -c --arg migration "$actual_migration" '. + {current_migration:$migration}' <<<"$actual_core")
expected=$(jq -c '.source_database' "${export_dir}/manifest.json")
if ! jq -e -n --argjson expected "$expected" --argjson actual "$actual" \
  '$expected.database == $actual.database and $expected.tables == $actual.tables and $expected.current_migration == $actual.current_migration and $expected.encoding == $actual.encoding and $expected.collation == $actual.collation and $expected.extensions == $actual.extensions' >/dev/null; then
  jq -n --argjson expected "$expected" --argjson actual "$actual" '{status:"mismatch",expected:$expected,actual:$actual}' >&2
  exit 1
fi
jq -n \
  --arg clone_id "$(jq -er '.clone_id' "${export_dir}/manifest.json")" \
  --arg verified_at "$(date -u +%FT%TZ)" \
  --argjson expected "$expected" \
  --argjson actual "$actual" \
  '{status:"verified",clone_id:$clone_id,verified_at:$verified_at,expected:$expected,actual:$actual}'
