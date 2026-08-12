#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
rehearsal_root=${1:-/mnt/data2/clustr-rehearsal/2026-08-12-launch-readiness}
host_port=${CLUSTR_REHEARSAL_PORT:-55432}
database_name=${CLUSTR_REHEARSAL_DB:-reddit_cluster}
database_user=${CLUSTR_REHEARSAL_USER:-clustr_rehearsal}
password_file="${rehearsal_root}/postgres-password"
binary="${rehearsal_root}/clustr-precalculate-full-catalog"
metrics="${rehearsal_root}/full-catalog-time.txt"
log_file="${rehearsal_root}/full-catalog.log"

if [[ ! -s ${password_file} ]]; then
  echo "missing rehearsal credential file: ${password_file}" >&2
  exit 1
fi
if [[ -e ${metrics} || -e ${log_file} ]]; then
  echo "full-catalog evidence already exists; refusing to overwrite it" >&2
  exit 1
fi

cd "${repo_root}/backend"
go build -trimpath -o "${binary}" ./cmd/precalculate

password=$(<"${password_file}")
export DATABASE_URL="postgres://${database_user}:${password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"
export PRECALC_DB_MAX_OPEN_CONNS=${PRECALC_DB_MAX_OPEN_CONNS:-5}
export SPATIAL_CATALOG_RETENTION=${SPATIAL_CATALOG_RETENTION:-2}
export SPATIAL_CATALOG_WORK_MEM_MB=${SPATIAL_CATALOG_WORK_MEM_MB:-512}
export GRAPH_REVISION_NODE_CAP=${GRAPH_REVISION_NODE_CAP:-100000}
export GRAPH_REVISION_LINK_CAP=${GRAPH_REVISION_LINK_CAP:-200000}
export GRAPH_REVISION_SUBREDDIT_CAP=${GRAPH_REVISION_SUBREDDIT_CAP:-70000}
export GRAPH_REVISION_USER_CAP=${GRAPH_REVISION_USER_CAP:-30000}
export GRAPH_REVISION_POST_CAP=${GRAPH_REVISION_POST_CAP:-0}
export GRAPH_REVISION_COMMENT_CAP=${GRAPH_REVISION_COMMENT_CAP:-0}

started_ns=$(date +%s%N)
"${binary}" --once --publish-only --full-catalog >"${log_file}" 2>&1 &
worker_pid=$!
cleanup() {
  if kill -0 "${worker_pid}" 2>/dev/null; then
    kill "${worker_pid}" 2>/dev/null || true
    wait "${worker_pid}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM
peak_rss_kib=0
while kill -0 "${worker_pid}" 2>/dev/null; do
  current_rss_kib=$(awk '/^VmRSS:/ {print $2}' "/proc/${worker_pid}/status" 2>/dev/null || echo 0)
  if (( current_rss_kib > peak_rss_kib )); then peak_rss_kib=${current_rss_kib}; fi
  sleep 0.25
done
if ! wait "${worker_pid}"; then
  echo "full catalog failed; inspect ${log_file}" >&2
  exit 1
fi
finished_ns=$(date +%s%N)
{
  echo "elapsed_ms=$(( (finished_ns - started_ns) / 1000000 ))"
  echo "peak_rss_kib=${peak_rss_kib}"
} >"${metrics}"

echo "full catalog log: ${log_file}"
echo "resource metrics: ${metrics}"
