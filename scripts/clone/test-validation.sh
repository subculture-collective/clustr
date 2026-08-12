#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

clone_validate_id clustr-20260812T203500Z-a1b2c3d4
if (clone_validate_id '../../escape') >/dev/null 2>&1; then
  echo 'invalid clone id passed validation' >&2
  exit 1
fi
clone_validate_name reddit_cluster
if (clone_validate_name 'reddit-cluster;drop') >/dev/null 2>&1; then
  echo 'invalid database name passed validation' >&2
  exit 1
fi
if (clone_absolute_dir /) >/dev/null 2>&1; then
  echo 'root path passed validation' >&2
  exit 1
fi
if (clone_absolute_dir relative/path) >/dev/null 2>&1; then
  echo 'relative path passed validation' >&2
  exit 1
fi
echo 'clone validation tests passed'
