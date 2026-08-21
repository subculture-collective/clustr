#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 SOURCE_DIR ENV_FILE IMAGE_DIGEST ROLLBACK_DIGEST BASE_SHA OUTPUT_DIR" >&2
  exit 64
}

[[ $# -eq 6 ]] || usage

source_dir=$1
env_file=$2
image=$3
rollback_image=$4
base_sha=$5
output_dir=$6

[[ -d ${source_dir}/.git || -f ${source_dir}/.git ]] || {
  echo "source directory is not a git checkout: ${source_dir}" >&2
  exit 66
}
[[ -r ${env_file} ]] || {
  echo "cannot read release environment: ${env_file}" >&2
  exit 66
}
[[ ${image} =~ @sha256:[0-9a-f]{64}$ ]] || {
  echo "worker image must be pinned by sha256 digest" >&2
  exit 65
}
[[ ${rollback_image} =~ @sha256:[0-9a-f]{64}$ ]] || {
  echo "rollback image must be pinned by sha256 digest" >&2
  exit 65
}
[[ ! -e ${output_dir} ]] || {
  echo "release output already exists: ${output_dir}" >&2
  exit 73
}

for command_name in docker git sha256sum; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "required command is missing: ${command_name}" >&2
    exit 69
  }
done

source_sha=$(git -C "${source_dir}" rev-parse --verify HEAD)
base_sha=$(git -C "${source_dir}" rev-parse --verify "${base_sha}^{commit}")
if [[ -n $(git -C "${source_dir}" status --porcelain=v1 --untracked-files=all) ]]; then
  echo "release checkout is not clean" >&2
  exit 65
fi
if ! git -C "${source_dir}" merge-base --is-ancestor "${base_sha}" "${source_sha}"; then
  echo "base SHA is not an ancestor of source SHA" >&2
  exit 65
fi

if ! docker image inspect "${image}" >/dev/null 2>&1; then
  docker pull "${image}" >/dev/null
fi
reported_version=$(docker run --rm --network none "${image}" /app/precalculate --version)
case " ${reported_version} " in
  *" commit=${source_sha} "*) ;;
  *)
    echo "worker image provenance mismatch: expected embedded commit ${source_sha}" >&2
    exit 65
    ;;
esac

release_parent=$(dirname -- "${output_dir}")
mkdir -p -m 0750 "${release_parent}"
tmp=$(mktemp -d "${release_parent}/.clustr-worker-release.XXXXXX")
container_id=
cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  if [[ -n ${container_id} ]]; then
    docker rm -f "${container_id}" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmp}"
  exit "${exit_code}"
}
trap cleanup EXIT INT TERM

container_id=$(docker create "${image}")
docker cp "${container_id}:/app/precalculate" "${tmp}/precalculate"
docker rm "${container_id}" >/dev/null
container_id=

install -m 0755 "${source_dir}/deploy/kvant/run-precalculation.sh" "${tmp}/run-precalculation.sh"
install -m 0644 "${source_dir}/deploy/kvant/clustr-precalculate.user.service" "${tmp}/clustr-precalculate.service"
install -m 0644 "${source_dir}/deploy/kvant/clustr-precalculate.user.timer" "${tmp}/clustr-precalculate.timer"
install -m 0644 "${source_dir}/deploy/kvant/clustr-spatial-catalog.user.service" "${tmp}/clustr-spatial-catalog.service"
install -m 0644 "${source_dir}/deploy/kvant/clustr-spatial-catalog-preflight.user.service" "${tmp}/clustr-spatial-catalog-preflight.service"
install -m 0644 "${source_dir}/docs/runbooks/almaz-kvant-launch.md" "${tmp}/almaz-kvant-launch.md"

git -C "${source_dir}" log --reverse --format='%H%x09%s' "${base_sha}..${source_sha}" >"${tmp}/patches.tsv"
patch_count=$(wc -l <"${tmp}/patches.tsv" | tr -d ' ')
worker_binary_sha256=$(sha256sum "${tmp}/precalculate" | awk '{print $1}')
wrapper_sha256=$(sha256sum "${tmp}/run-precalculation.sh" | awk '{print $1}')
environment_sha256=$(sha256sum "${env_file}" | awk '{print $1}')
patch_manifest_sha256=$(sha256sum "${tmp}/patches.tsv" | awk '{print $1}')

{
  printf 'release_schema=clustr-worker-v1\n'
  printf 'source_sha=%s\n' "${source_sha}"
  printf 'base_sha=%s\n' "${base_sha}"
  printf 'worker_image=%s\n' "${image}"
  printf 'rollback_image=%s\n' "${rollback_image}"
  printf 'worker_version=%q\n' "${reported_version}"
  printf 'worker_binary_sha256=%s\n' "${worker_binary_sha256}"
  printf 'wrapper_sha256=%s\n' "${wrapper_sha256}"
  printf 'environment_sha256=%s\n' "${environment_sha256}"
  printf 'patch_count=%s\n' "${patch_count}"
  printf 'patch_manifest_sha256=%s\n' "${patch_manifest_sha256}"
  printf 'created_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"${tmp}/release.env"

(
  cd "${tmp}"
  for artifact in *; do
    [[ ${artifact} == SHA256SUMS ]] || sha256sum "${artifact}"
  done
) >"${tmp}/SHA256SUMS"
chmod -R a-w "${tmp}"
mv "${tmp}" "${output_dir}"
trap - EXIT INT TERM
printf 'worker release created: %s\n' "${output_dir}"
