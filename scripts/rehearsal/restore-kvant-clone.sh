#!/usr/bin/env bash
set -euo pipefail

dump_path=${1:-}
rehearsal_root=${2:-/mnt/data2/clustr-rehearsal/2026-08-12-launch-readiness}
container_name=${CLUSTR_REHEARSAL_CONTAINER:-clustr-rehearsal-pg17}
host_port=${CLUSTR_REHEARSAL_PORT:-55432}
database_name=${CLUSTR_REHEARSAL_DB:-reddit_cluster}
database_user=${CLUSTR_REHEARSAL_USER:-clustr_rehearsal}
data_dir="${rehearsal_root}/pgdata"
password_file="${rehearsal_root}/postgres-password"

if [[ -z "${dump_path}" || ! -f "${dump_path}" ]]; then
  echo "usage: $0 /absolute/path/to/reddit_cluster.dump [rehearsal-root]" >&2
  exit 2
fi

if [[ "${dump_path}" != /* || "${rehearsal_root}" != /* ]]; then
  echo "dump and rehearsal root must be absolute paths" >&2
  exit 2
fi

if docker container inspect "${container_name}" >/dev/null 2>&1; then
  echo "container ${container_name} already exists; refusing to replace it" >&2
  exit 1
fi

if [[ -e "${data_dir}" ]]; then
  echo "data directory ${data_dir} already exists; refusing to overwrite it" >&2
  exit 1
fi

install -d -m 700 "${rehearsal_root}" "${data_dir}"
if [[ ! -s "${password_file}" ]]; then
  umask 077
  # Hex avoids URL-escaping problems when the same credential is used in a
  # lib/pq DATABASE_URL during the migration and publication rehearsal.
  openssl rand -hex 32 > "${password_file}"
fi
chmod 600 "${password_file}"

docker run -d \
  --name "${container_name}" \
  --restart no \
  --cpus 8 \
  --memory 16g \
  --memory-swap 20g \
  --publish "127.0.0.1:${host_port}:5432" \
  --env "POSTGRES_USER=${database_user}" \
  --env "POSTGRES_DB=${database_name}" \
  --env POSTGRES_PASSWORD_FILE=/run/secrets/postgres-password \
  --mount "type=bind,src=${data_dir},dst=/var/lib/postgresql/data" \
  --mount "type=bind,src=${password_file},dst=/run/secrets/postgres-password,readonly" \
  pgvector/pgvector:pg17 >/dev/null

for _ in $(seq 1 60); do
  if docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1; then
  echo "PostgreSQL did not become ready; inspect with: docker logs ${container_name}" >&2
  exit 1
fi

PGPASSWORD=$(<"${password_file}") pg_restore \
  --host 127.0.0.1 \
  --port "${host_port}" \
  --username "${database_user}" \
  --dbname "${database_name}" \
  --no-owner \
  --no-acl \
  --exit-on-error \
  --jobs 8 \
  "${dump_path}"

PGPASSWORD=$(<"${password_file}") psql \
  --host 127.0.0.1 \
  --port "${host_port}" \
  --username "${database_user}" \
  --dbname "${database_name}" \
  --no-psqlrc \
  --set ON_ERROR_STOP=1 \
  --command "ANALYZE;"

echo "restored ${database_name} into ${container_name} on 127.0.0.1:${host_port}"
echo "credential file: ${password_file} (mode 0600)"
