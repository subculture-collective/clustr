#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
rehearsal_root=${1:-/mnt/data2/clustr-rehearsal/2026-08-12-launch-readiness}
host_port=${CLUSTR_REHEARSAL_PORT:-55432}
database_name=${CLUSTR_REHEARSAL_DB:-reddit_cluster}
database_user=${CLUSTR_REHEARSAL_USER:-clustr_rehearsal}
password_file="${rehearsal_root}/postgres-password"

if [[ ! -s "${password_file}" ]]; then
  echo "missing rehearsal credential file: ${password_file}" >&2
  exit 1
fi

password=$(<"${password_file}")
export DATABASE_URL="postgres://${database_user}:${password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"
export MIGRATIONS_DIR="${repo_root}/backend/migrations"

cd "${repo_root}/backend"
go run ./cmd/migrate
