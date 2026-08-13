#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 ENV_FILE CLONE_ID" >&2
  exit 64
fi
env_file=$1
clone_id=$2
[[ -r $env_file ]] || { echo "calculation environment is unreadable: $env_file" >&2; exit 66; }
[[ $clone_id =~ ^clustr-[0-9]{8}T[0-9]{6}Z-[a-z0-9]{6,16}$ ]] || { echo "invalid clone id" >&2; exit 65; }

env_value() {
  awk -F= -v wanted="$1" '$1==wanted {sub(/^[^=]*=/, ""); print; exit}' "$env_file"
}
image=$(env_value CLUSTR_PRECALCULATE_IMAGE)
clone_root=$(env_value KVANT_CLONE_ROOT)
cpu_limit=$(env_value CLUSTR_WORKER_CPU_LIMIT)
memory_limit=$(env_value CLUSTR_WORKER_MEMORY_LIMIT)
: "${clone_root:=/mnt/data2/clustr-clones}" "${cpu_limit:=12}" "${memory_limit:=8g}"
[[ $image =~ ^sha256:[0-9a-f]{64}$ || $image =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]] || {
  echo "CLUSTR_PRECALCULATE_IMAGE must be pinned by immutable digest or local image ID" >&2
  exit 65
}
[[ $clone_root == /* && $clone_root != / ]] || { echo "invalid clone root" >&2; exit 65; }
for command_name in docker flock jq; do
  command -v "$command_name" >/dev/null || { echo "missing command: $command_name" >&2; exit 69; }
done

clone_dir="${clone_root}/${clone_id}"
password_file="${clone_dir}/postgres-password"
[[ -f ${clone_dir}/READY && -r $password_file ]] || { echo "clone is not ready: $clone_id" >&2; exit 69; }
container="${clone_id}-pg17"
[[ $(docker inspect "$container" --format '{{index .Config.Labels "io.clustr.clone-id"}}') == "$clone_id" ]] || {
  echo "clone container identity mismatch" >&2
  exit 69
}
[[ $(docker inspect "$container" --format '{{.State.Health.Status}}') == healthy ]] || { echo "clone is not healthy" >&2; exit 69; }
port=$(docker inspect "$container" | jq -er '.[0].HostConfig.PortBindings["5432/tcp"] | select(length==1) | .[0] | select(.HostIp=="127.0.0.1") | .HostPort')
[[ $port =~ ^[0-9]+$ ]] || { echo "clone does not have one loopback PostgreSQL binding" >&2; exit 69; }

runtime_parent=${XDG_RUNTIME_DIR:-/run/user/1000}
exec 9>"${runtime_parent}/clustr-clone-calculation.lock"
flock -n 9 || { echo "another clone calculation is running" >&2; exit 75; }
password=$(<"$password_file")
docker run --rm \
  --name "clustr-clone-calculate-${clone_id}" \
  --network host \
  --cpus "$cpu_limit" \
  --memory "$memory_limit" \
  --pids-limit 512 \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=256m \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --env-file "$env_file" \
  --env-file <(printf 'DATABASE_URL=postgres://clustr_clone:%s@127.0.0.1:%s/reddit_cluster?sslmode=disable\n' "$password" "$port") \
  "$image" \
  /app/precalculate --once --full --full-catalog
