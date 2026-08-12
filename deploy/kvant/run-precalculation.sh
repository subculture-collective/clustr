#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: $0 ENV_FILE [--initial-full]" >&2
  exit 64
fi

env_file=$1
mode=${2:-}
if [[ ! -r ${env_file} ]]; then
  echo "cannot read worker environment: ${env_file}" >&2
  exit 66
fi
if [[ -n ${mode} && ${mode} != "--initial-full" ]]; then
  echo "unknown mode: ${mode}" >&2
  exit 64
fi

env_value() {
  local key=$1
  awk -F= -v wanted="${key}" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}' "${env_file}"
}

ssh_target=$(env_value ALMAZ_SSH_TARGET)
local_db_port=$(env_value ALMAZ_LOCAL_DB_PORT)
worker_image=$(env_value CLUSTR_PRECALCULATE_IMAGE)
cpu_limit=$(env_value CLUSTR_WORKER_CPU_LIMIT)
memory_limit=$(env_value CLUSTR_WORKER_MEMORY_LIMIT)

: "${ssh_target:=almaz}"
: "${local_db_port:=15436}"
: "${cpu_limit:=12}"
: "${memory_limit:=8g}"

if [[ ! ${local_db_port} =~ ^[0-9]+$ ]] || (( local_db_port < 1024 || local_db_port > 65535 )); then
  echo "ALMAZ_LOCAL_DB_PORT must be an unprivileged TCP port" >&2
  exit 65
fi
if [[ ! ${worker_image} =~ @sha256:[0-9a-f]{64}$ ]]; then
  echo "CLUSTR_PRECALCULATE_IMAGE must be pinned by a 64-character sha256 digest" >&2
  exit 65
fi
for command_name in docker flock ssh timeout; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "required command is missing: ${command_name}" >&2
    exit 69
  fi
done

runtime_parent=${XDG_RUNTIME_DIR:-/tmp}
runtime_dir=$(mktemp -d "${runtime_parent}/clustr-precalculate.XXXXXX")
cid_file="${runtime_dir}/container.cid"
ssh_pid=

cleanup() {
  local exit_code=$?
  trap - EXIT INT TERM
  if [[ -s ${cid_file} ]]; then
    container_id=$(<"${cid_file}")
    docker stop --time 30 "${container_id}" >/dev/null 2>&1 || true
  fi
  if [[ -n ${ssh_pid} ]] && kill -0 "${ssh_pid}" 2>/dev/null; then
    kill "${ssh_pid}" 2>/dev/null || true
    wait "${ssh_pid}" 2>/dev/null || true
  fi
  rm -f "${cid_file}"
  rmdir "${runtime_dir}" 2>/dev/null || true
  exit "${exit_code}"
}
trap cleanup EXIT INT TERM

exec 9>"${runtime_parent}/clustr-precalculate.lock"
if ! flock -n 9; then
  echo "another Clustr calculation is already running" >&2
  exit 75
fi

if timeout 1 bash -c "</dev/tcp/127.0.0.1/${local_db_port}" >/dev/null 2>&1; then
  echo "local tunnel port ${local_db_port} is already in use; refusing an ambiguous database target" >&2
  exit 75
fi

ssh \
  -N -T \
  -o BatchMode=yes \
  -o ControlMaster=no \
  -o ControlPath=none \
  -o ControlPersist=no \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -L "127.0.0.1:${local_db_port}:127.0.0.1:15435" \
  "${ssh_target}" &
ssh_pid=$!

for _ in $(seq 1 50); do
  if ! kill -0 "${ssh_pid}" 2>/dev/null; then
    wait "${ssh_pid}"
    echo "database tunnel exited before becoming ready" >&2
    exit 69
  fi
  if timeout 1 bash -c "</dev/tcp/127.0.0.1/${local_db_port}" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
if ! timeout 1 bash -c "</dev/tcp/127.0.0.1/${local_db_port}" >/dev/null 2>&1; then
  echo "database tunnel did not become ready" >&2
  exit 69
fi

# Every published world must include source rows newer than the prior catalog.
# The measured full-corpus pass fits inside the hourly window; retain the
# explicit initial mode as an operator-facing cutover signal even though both
# paths intentionally use the same complete publication contract.
worker_args=(--once --full --full-catalog)
if [[ ${mode} == "--initial-full" ]]; then
  worker_args=(--once --full --full-catalog)
fi

docker run --rm \
  --name "clustr-precalculate-${$}" \
  --cidfile "${cid_file}" \
  --network host \
  --cpus "${cpu_limit}" \
  --memory "${memory_limit}" \
  --pids-limit 512 \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=256m \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --env-file "${env_file}" \
  "${worker_image}" \
  /app/precalculate "${worker_args[@]}"
