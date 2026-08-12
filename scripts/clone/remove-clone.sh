#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

for command_name in docker jq readlink rmdir; do clone_require_command "$command_name"; done

clone_root=${1:-}
clone_id=${2:-}
confirmation=${3:-}
clone_absolute_dir "$clone_root"
clone_validate_id "$clone_id"
[[ $confirmation == "--confirm-${clone_id}" ]] || clone_die "removal requires --confirm-${clone_id}"
clone_dir="${clone_root}/${clone_id}"
[[ -d $clone_dir ]] || clone_die "clone directory is unavailable: $clone_dir"
resolved_root=$(readlink -f "$clone_root")
resolved_clone=$(readlink -f "$clone_dir")
[[ $resolved_clone == "${resolved_root}/${clone_id}" ]] || clone_die "clone path escaped its configured root"
[[ -f "${clone_dir}/READY" || -f "${clone_dir}/FAILED" ]] || clone_die "refusing to remove an in-progress or unknown clone"
container=$(awk -F= '$1=="container" {print $2; exit}' "${clone_dir}/READY" 2>/dev/null || true)
: "${container:=${clone_id}-pg17}"
image=
if docker container inspect "$container" >/dev/null 2>&1; then
  image=$(docker inspect "$container" --format '{{.Image}}')
  docker stop --time 30 "$container" >/dev/null
  docker rm "$container" >/dev/null
elif [[ -f "${clone_dir}/source-manifest.json" ]]; then
  image=$(jq -er '.source_image_digest // empty' "${clone_dir}/source-manifest.json")
  [[ $image =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]] || clone_die "clone manifest does not contain a pinned cleanup image"
fi
if [[ -n $image ]]; then
  docker run --rm --network none --entrypoint find \
    --mount "type=bind,src=${resolved_clone},dst=/clone" \
    "$image" /clone -mindepth 1 -delete
fi
rmdir "$resolved_clone" 2>/dev/null || clone_die "clone directory still contains privileged files; recover using the original clone image"
echo "removed clone $clone_id from $clone_root"
