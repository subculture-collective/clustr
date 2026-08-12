#!/usr/bin/env bash
set -euo pipefail

if [[ ${CLUSTR_CLONE_LIB_LOADED:-0} != 1 ]]; then
  script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  # shellcheck source=lib.sh
  source "${script_dir}/lib.sh"
fi

action=${1:-}
clone_id=${2:-}
export_root=${3:-/mnt/spektr/server/projects/data/clustr-exports}
container=${4:-pg17-clustr}
database=${5:-reddit_cluster}
gpg_recipient=${6:-}
gpg_public_key_b64=${7:-}

clone_absolute_dir "$export_root"
clone_validate_name "$database"
for command_name in docker flock jq sha256sum df mkfifo gpg base64; do
  clone_require_command "$command_name"
done
docker container inspect "$container" >/dev/null 2>&1 || clone_die "source container is unavailable: $container"

database_bytes=$(docker exec "$container" sh -c \
  'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -Atq -c "select pg_database_size(current_database())"' \
  sh "$database")
[[ $database_bytes =~ ^[0-9]+$ ]] || clone_die "could not read source database size"

case "$action" in
  preflight)
    export_parent=$(dirname "$export_root")
    [[ -d $export_parent && -w $export_parent ]] || clone_die "source export parent is unavailable or not writable: $export_parent"
    clone_require_free_bytes "$export_parent" "$((database_bytes + database_bytes / 4))"
    pg_dump_version=$(docker exec "$container" pg_dump --version)
    workload=$(docker exec -i "$container" sh -c \
      'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -Atq' \
      sh "$database" <<'SQL'
SELECT json_build_object(
  'active_pg_dump_sessions', count(*) FILTER (WHERE application_name='pg_dump'),
  'idle_snapshot_transactions', count(*) FILTER (
    WHERE state='idle in transaction' AND query LIKE '%pg_export_snapshot%'
  )
)::text FROM pg_stat_activity WHERE datname=current_database();
SQL
)
    [[ $(jq -er '.active_pg_dump_sessions' <<<"$workload") == 0 ]] || clone_die "another pg_dump session is active"
    [[ $(jq -er '.idle_snapshot_transactions' <<<"$workload") == 0 ]] || clone_die "an exported snapshot transaction is already idle"
    jq -n \
      --arg source_host "$(hostname)" \
      --arg container "$container" \
      --arg database "$database" \
      --arg pg_dump_version "$pg_dump_version" \
      --argjson database_size_bytes "$database_bytes" \
      --argjson workload "$workload" \
      --arg export_root "$export_root" \
      '{status:"preflight-ok",source_host:$source_host,container:$container,database:$database,pg_dump_version:$pg_dump_version,database_size_bytes:$database_size_bytes,export_root:$export_root,workload:$workload}'
    ;;
  status)
    clone_validate_id "$clone_id"
    export_dir="${export_root}/${clone_id}"
    [[ -f "${export_dir}/READY" ]] || clone_die "source export is not ready: $clone_id"
    (cd "$export_dir" && sha256sum --check --strict SHA256SUMS >/dev/null)
    jq -c '{clone_id,status:"ready",archive_sha256,archive_bytes}' "${export_dir}/manifest.json"
    ;;
  export)
    clone_validate_id "$clone_id"
    [[ $gpg_recipient =~ ^[0-9A-Fa-f]{40,64}$ ]] || clone_die "a full GPG recipient fingerprint is required"
    [[ -n $gpg_public_key_b64 ]] || clone_die "the Kvant public encryption key is required"
    install -d -m 700 "$export_root"
    clone_require_free_bytes "$export_root" "$((database_bytes + database_bytes / 4))"
    export_dir="${export_root}/${clone_id}"
    [[ ! -e $export_dir ]] || clone_die "source export already exists: $export_dir"
    install -d -m 700 "$export_dir"
    exec 8>"${export_root}/.export.lock"
    flock -n 8 || clone_die "another source export is running"

    fifo=$(mktemp -u "${export_dir}/snapshot.XXXXXX")
    snapshot_output="${export_dir}/snapshot.out"
    holder_pid=
    cleanup_snapshot() {
      exec 9>&- 2>/dev/null || true
      if [[ -n ${holder_pid} ]] && kill -0 "$holder_pid" 2>/dev/null; then
        kill "$holder_pid" 2>/dev/null || true
        wait "$holder_pid" 2>/dev/null || true
      fi
      rm -f "$fifo" "$snapshot_output"
    }
    fail_export() {
      local code=$?
      cleanup_snapshot
      rm -f "${archive_part:-}" "${archive:-}.part"
      find "$export_dir" -maxdepth 1 -type d -name 'gnupg.*' -exec rm -rf --one-file-system {} + 2>/dev/null || true
      printf 'failed_at=%s\nexit_code=%s\n' "$(date -u +%FT%TZ)" "$code" >"${export_dir}/FAILED"
      exit "$code"
    }
    trap fail_export EXIT INT TERM
    mkfifo -m 600 "$fifo"
    docker exec -i "$container" sh -c \
      'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -Atq' \
      sh "$database" <"$fifo" >"$snapshot_output" 2>"${export_dir}/snapshot.log" &
    holder_pid=$!
    exec 9>"$fifo"
    printf '%s\n' 'BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;' 'SELECT pg_export_snapshot();' >&9
    for _ in $(seq 1 100); do
      [[ -s $snapshot_output ]] && break
      kill -0 "$holder_pid" 2>/dev/null || clone_die "snapshot holder exited early"
      sleep 0.1
    done
    snapshot=$(head -1 "$snapshot_output" | tr -d '[:space:]')
    [[ $snapshot =~ ^[0-9A-Fa-f-]+$ ]] || clone_die "invalid exported snapshot identifier"

    archive_part="${export_dir}/${database}.dump.part"
    archive="${export_dir}/${database}.dump.gpg"
    docker exec -i "$container" sh -c \
      'exec pg_dump -U "$POSTGRES_USER" -d "$1" --format=custom --compress=zstd:3 --no-owner --no-acl --snapshot="$2"' \
      sh "$database" "$snapshot" >"$archive_part" 2>"${export_dir}/pg_dump.log"
    [[ -s $archive_part ]] || clone_die "pg_dump produced an empty archive"
    docker exec -i "$container" pg_restore --list <"$archive_part" >/dev/null
    migration=$(docker exec "$container" sh -c \
      'if psql -X -U "$POSTGRES_USER" -d "$1" -Atq -c "select to_regclass('\''public.clustr_schema_migrations'\'') is not null" | grep -qx t; then psql -X -U "$POSTGRES_USER" -d "$1" -Atq -c "select coalesce(max(filename),'\''empty-ledger'\'') from clustr_schema_migrations"; else echo legacy-untracked; fi' \
      sh "$database")

    docker exec -i "$container" sh -c \
      'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -Atq' \
      sh "$database" >"${export_dir}/source-database.json" <<SQL
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET TRANSACTION SNAPSHOT '${snapshot}';
SELECT json_build_object(
  'captured_at', now(),
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
COMMIT;
SQL
    jq --arg migration "$migration" '. + {current_migration:$migration}' \
      "${export_dir}/source-database.json" >"${export_dir}/source-database.json.tmp"
    mv "${export_dir}/source-database.json.tmp" "${export_dir}/source-database.json"
    cleanup_snapshot

    export_gnupg=$(mktemp -d "${export_dir}/gnupg.XXXXXX")
    chmod 700 "$export_gnupg"
    printf '%s' "$gpg_public_key_b64" | base64 --decode |
      gpg --batch --homedir "$export_gnupg" --import >/dev/null 2>"${export_dir}/gpg-import.log"
    imported_fingerprint=$(gpg --batch --homedir "$export_gnupg" --with-colons --list-keys "$gpg_recipient" |
      awk -F: '$1=="fpr" {print $10; exit}')
    [[ ${imported_fingerprint,,} == ${gpg_recipient,,} ]] || clone_die "imported encryption key fingerprint mismatch"
    gpg --batch --yes --homedir "$export_gnupg" --trust-model always \
      --recipient "$gpg_recipient" --output "${archive}.part" --encrypt "$archive_part"
    rm -rf --one-file-system "$export_gnupg"
    rm -f "$archive_part"
    mv "${archive}.part" "$archive"

    archive_sha256=$(sha256sum "$archive" | awk '{print $1}')
    archive_bytes=$(stat -c %s "$archive")
    source_image=$(docker inspect "$container" --format '{{.Config.Image}}')
    source_image_digest=$(docker image inspect "$source_image" --format '{{index .RepoDigests 0}}' 2>/dev/null || true)
    jq -n \
      --arg clone_id "$clone_id" \
      --arg created_at "$(date -u +%FT%TZ)" \
      --arg source_host "$(hostname)" \
      --arg source_container "$container" \
      --arg source_image "$source_image" \
      --arg source_image_digest "$source_image_digest" \
      --arg database "$database" \
      --arg archive_file "${database}.dump.gpg" \
      --arg archive_sha256 "$archive_sha256" \
      --argjson archive_bytes "$archive_bytes" \
      --arg encryption "gpg" \
      --arg encryption_recipient "$gpg_recipient" \
      --slurpfile source_database "${export_dir}/source-database.json" \
      '{format_version:1,clone_id:$clone_id,created_at:$created_at,source_host:$source_host,source_container:$source_container,source_image:$source_image,source_image_digest:$source_image_digest,database:$database,archive_file:$archive_file,archive_sha256:$archive_sha256,archive_bytes:$archive_bytes,encryption:$encryption,encryption_recipient:$encryption_recipient,source_database:$source_database[0]}' \
      >"${export_dir}/manifest.json"
    chmod 600 "$archive" "${export_dir}/"*.json "${export_dir}/"*.log
    (cd "$export_dir" && sha256sum "${database}.dump.gpg" source-database.json manifest.json >SHA256SUMS)
    printf 'ready_at=%s\n' "$(date -u +%FT%TZ)" >"${export_dir}/READY"
    chmod 600 "${export_dir}/SHA256SUMS" "${export_dir}/READY"
    trap - EXIT INT TERM
    jq -c '{clone_id,status:"ready",archive_sha256,archive_bytes,source_database}' "${export_dir}/manifest.json"
    ;;
  cleanup)
    clone_validate_id "$clone_id"
    [[ ${6:-} == "--confirm-${clone_id}" ]] || clone_die "cleanup requires --confirm-${clone_id}"
    export_dir="${export_root}/${clone_id}"
    [[ -f "${export_dir}/READY" || -f "${export_dir}/FAILED" ]] || clone_die "refusing to remove an unknown/in-progress export"
    rm -rf --one-file-system "$export_dir"
    echo "removed source export $export_dir"
    ;;
  *)
    clone_die "usage: $0 {preflight|status|export|cleanup} [clone-id] [export-root] [container] [database] [cleanup-confirmation]"
    ;;
esac
