#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
compose_file="${repo_root}/backend/docker-compose.yml"
phase=${1:-}
env_file=${CLUSTR_ENV_FILE:-}

usage() {
  cat >&2 <<'EOF'
usage: CLUSTR_ENV_FILE=/secure/path/clustr.env scripts/deploy.sh PHASE

PHASE is one of:
  prepare        validate configuration and build release images only
  migrate        run the migration service (requires CONFIRM_CLUSTR_MIGRATION=yes)
  start-reads    start API, backup, and frontend; never starts the crawler
  start-crawler  start the crawler (requires CONFIRM_CLUSTR_CRAWLER=yes)
  status         show compose service state

Use docs/runbooks/almaz-kvant-launch.md for backup, first publication,
readiness, browser validation, and rollback gates. This script intentionally
does not pull Git, run raw schema SQL, start the local precalculator, or enable
the crawler as a side effect.
EOF
  exit 64
}

if [[ -z ${phase} || -z ${env_file} || ! -r ${env_file} ]]; then
  usage
fi

export CLUSTR_ENV_FILE="${env_file}"
compose=(docker compose --env-file "${env_file}" -f "${compose_file}")

case "${phase}" in
  prepare)
    "${compose[@]}" config --quiet
    "${compose[@]}" build api crawler migrate precalculate backup reddit_frontend
    ;;
  migrate)
    if [[ ${CONFIRM_CLUSTR_MIGRATION:-} != yes ]]; then
      echo "set CONFIRM_CLUSTR_MIGRATION=yes after the verified backup is complete" >&2
      exit 65
    fi
    "${compose[@]}" run --rm migrate
    ;;
  start-reads)
    "${compose[@]}" up -d api backup reddit_frontend
    ;;
  start-crawler)
    if [[ ${CONFIRM_CLUSTR_CRAWLER:-} != yes ]]; then
      echo "set CONFIRM_CLUSTR_CRAWLER=yes only after the clone/staging canary passes" >&2
      exit 65
    fi
    "${compose[@]}" up -d crawler
    ;;
  status)
    "${compose[@]}" ps
    ;;
  *)
    usage
    ;;
esac
