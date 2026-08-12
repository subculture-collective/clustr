#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

export_dir=${1:-}
clone_root=${2:-/mnt/data2/clustr-clones}
host_port=${3:-55440}
jobs=${4:-8}
gpg_home=${5:-}

clone_absolute_dir "$export_dir"
clone_absolute_dir "$clone_root"
clone_absolute_dir "$gpg_home"
[[ $host_port =~ ^[0-9]+$ ]] && ((host_port >= 1024 && host_port <= 65535)) || clone_die "invalid host port"
[[ $jobs =~ ^[1-9][0-9]*$ ]] || clone_die "restore jobs must be positive"
for command_name in docker jq sha256sum openssl flock gpg timeout; do clone_require_command "$command_name"; done
[[ -d $gpg_home ]] || clone_die "Kvant GPG home is unavailable: $gpg_home"
[[ -f "${export_dir}/READY" && -f "${export_dir}/manifest.json" ]] || clone_die "export is not complete: $export_dir"
(cd "$export_dir" && sha256sum --check --strict SHA256SUMS)

clone_id=$(jq -er '.clone_id' "${export_dir}/manifest.json")
database=$(jq -er '.database' "${export_dir}/manifest.json")
archive_file=$(jq -er '.archive_file' "${export_dir}/manifest.json")
source_image_digest=$(jq -er '.source_image_digest // empty' "${export_dir}/manifest.json")
clone_validate_id "$clone_id"
clone_validate_name "$database"
[[ $archive_file == "${database}.dump.gpg" && -f "${export_dir}/${archive_file}" ]] || clone_die "archive name does not match manifest"
[[ $(jq -er '.encryption' "${export_dir}/manifest.json") == gpg ]] || clone_die "unsupported archive encryption"
expected_recipient=$(jq -er '.encryption_recipient' "${export_dir}/manifest.json")
local_recipient=$(gpg --batch --homedir "$gpg_home" --with-colons --list-secret-keys "$expected_recipient" |
  awk -F: '$1=="fpr" {print $10; exit}')
[[ ${local_recipient,,} == ${expected_recipient,,} ]] || clone_die "Kvant decryption key does not match manifest"
[[ $source_image_digest =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]] || clone_die "source image is not pinned by digest"

install -d -m 700 "$clone_root"
exec 8>"${clone_root}/.restore.lock"
flock -n 8 || clone_die "another clone restore is running"
required_bytes=$(jq -er '.source_database.database_size_bytes' "${export_dir}/manifest.json")
clone_require_free_bytes "$clone_root" "$((required_bytes * 2 + required_bytes / 2))"
clone_dir="${clone_root}/${clone_id}"
data_dir="${clone_dir}/pgdata"
password_file="${clone_dir}/postgres-password"
container_name=${CLUSTR_CLONE_CONTAINER_NAME:-"${clone_id}-pg17"}
[[ $container_name =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]+$ ]] || clone_die "invalid container name"
[[ ! -e $clone_dir ]] || clone_die "clone destination already exists: $clone_dir"
docker container inspect "$container_name" >/dev/null 2>&1 && clone_die "clone container already exists: $container_name"
if timeout 1 bash -c "</dev/tcp/127.0.0.1/${host_port}" >/dev/null 2>&1; then
  clone_die "destination port is already in use: $host_port"
fi

install -d -m 700 "$clone_dir" "$data_dir"
cp --reflink=auto -- "${export_dir}/manifest.json" "${clone_dir}/source-manifest.json"
umask 077
openssl rand -hex 32 >"$password_file"
printf 'restoring_at=%s\n' "$(date -u +%FT%TZ)" >"${clone_dir}/RESTORING"
decrypted_archive="${clone_dir}/${database}.dump"
restore_ok=false
finish() {
  if [[ $restore_ok != true ]]; then
    rm -f "$decrypted_archive"
    printf 'failed_at=%s\n' "$(date -u +%FT%TZ)" >"${clone_dir}/FAILED"
  fi
}
trap finish EXIT
gpg --batch --yes --homedir "$gpg_home" --output "$decrypted_archive" --decrypt "${export_dir}/${archive_file}"
chmod 600 "$decrypted_archive"

docker run -d \
  --name "$container_name" \
  --restart unless-stopped \
  --label "io.clustr.role=database-clone" \
  --label "io.clustr.clone-id=${clone_id}" \
  --cpus "${CLUSTR_CLONE_CPU_LIMIT:-8}" \
  --memory "${CLUSTR_CLONE_MEMORY_LIMIT:-16g}" \
  --memory-swap "${CLUSTR_CLONE_MEMORY_SWAP_LIMIT:-20g}" \
  --health-cmd "pg_isready -U clustr_clone -d ${database}" \
  --health-interval 10s \
  --health-timeout 5s \
  --health-start-period 20s \
  --health-retries 6 \
  --publish "127.0.0.1:${host_port}:5432" \
  --env "POSTGRES_USER=clustr_clone" \
  --env "POSTGRES_DB=${database}" \
  --env POSTGRES_PASSWORD_FILE=/run/secrets/postgres-password \
  --mount "type=bind,src=${data_dir},dst=/var/lib/postgresql/data" \
  --mount "type=bind,src=${password_file},dst=/run/secrets/postgres-password,readonly" \
  --mount "type=bind,src=${clone_dir},dst=/clone/work,readonly" \
  "$source_image_digest" >/dev/null

for _ in $(seq 1 120); do
  docker exec "$container_name" pg_isready -U clustr_clone -d "$database" >/dev/null 2>&1 && break
  sleep 1
done
docker exec "$container_name" pg_isready -U clustr_clone -d "$database" >/dev/null 2>&1 ||
  clone_die "restored PostgreSQL did not become ready"

docker exec "$container_name" pg_restore \
  --username clustr_clone --dbname "$database" --no-owner --no-acl \
  --exit-on-error --jobs "$jobs" "/clone/work/${database}.dump" \
  >"${clone_dir}/pg_restore.log" 2>&1
docker exec "$container_name" psql -X -v ON_ERROR_STOP=1 -U clustr_clone -d "$database" -c 'ANALYZE;' \
  >"${clone_dir}/analyze.log" 2>&1
for _ in $(seq 1 90); do
  [[ $(docker inspect "$container_name" --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}') == healthy ]] && break
  sleep 1
done
[[ $(docker inspect "$container_name" --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}') == healthy ]] ||
  clone_die "restored PostgreSQL container did not become healthy"
"${script_dir}/verify-clone.sh" "$export_dir" "$container_name" >"${clone_dir}/verification.json"
rm -f "$decrypted_archive"

rm -f "${clone_dir}/RESTORING" "${clone_dir}/FAILED"
printf 'ready_at=%s\ncontainer=%s\nport=%s\n' "$(date -u +%FT%TZ)" "$container_name" "$host_port" >"${clone_dir}/READY"
restore_ok=true
trap - EXIT
echo "clone ready: ${clone_dir}"
echo "container: ${container_name}; listener: 127.0.0.1:${host_port}"
