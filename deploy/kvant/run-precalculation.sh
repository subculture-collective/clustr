#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: $0 ENV_FILE [graph-revision|catalog-rebuild|catalog-preflight]" >&2
  exit 64
fi

env_file=$1
mode=${2:-graph-revision}
if [[ ! -r ${env_file} ]]; then
  echo "cannot read worker environment: ${env_file}" >&2
  exit 66
fi
if [[ ${mode} == "--initial-full" ]]; then
  echo "legacy mode is no longer supported: ${mode}" >&2
  exit 64
fi
if [[ ${mode} != "graph-revision" && ${mode} != "catalog-rebuild" && ${mode} != "catalog-preflight" ]]; then
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
postgres_container=$(env_value ALMAZ_POSTGRES_CONTAINER)
catalog_min_free_gib=$(env_value SPATIAL_CATALOG_MIN_FREE_GIB)

: "${ssh_target:=10.0.0.200}"
: "${local_db_port:=15436}"
: "${cpu_limit:=12}"
: "${memory_limit:=8g}"
: "${postgres_container:=pg17-clustr}"
: "${catalog_min_free_gib:=40}"

if [[ ! ${local_db_port} =~ ^[0-9]+$ ]] || (( local_db_port < 1024 || local_db_port > 65535 )); then
  echo "ALMAZ_LOCAL_DB_PORT must be an unprivileged TCP port" >&2
  exit 65
fi
if [[ ! ${worker_image} =~ @sha256:[0-9a-f]{64}$ ]]; then
  echo "CLUSTR_PRECALCULATE_IMAGE must be pinned by a 64-character sha256 digest" >&2
  exit 65
fi
if [[ ${mode} != "graph-revision" && ! ${postgres_container} =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]]; then
  echo "ALMAZ_POSTGRES_CONTAINER must be a valid container name" >&2
  exit 65
fi
if [[ ${mode} != "graph-revision" && ( ! ${catalog_min_free_gib} =~ ^[0-9]+$ || ${#catalog_min_free_gib} -gt 10 || ( ${#catalog_min_free_gib} -eq 10 && ${catalog_min_free_gib} > 8589934591 ) ) ]]; then
  echo "SPATIAL_CATALOG_MIN_FREE_GIB must be a strictly positive integer within the supported range" >&2
  exit 65
fi
if [[ ${mode} != "graph-revision" && 10#${catalog_min_free_gib} -eq 0 ]]; then
  echo "SPATIAL_CATALOG_MIN_FREE_GIB must be a strictly positive integer within the supported range" >&2
  exit 65
fi
for command_name in flock ssh timeout; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "required command is missing: ${command_name}" >&2
    exit 69
  fi
done
if [[ ${mode} != "catalog-preflight" ]] && ! command -v docker >/dev/null 2>&1; then
  echo "required command is missing: docker" >&2
  exit 69
fi

if [[ ${mode} == "graph-revision" ]]; then
  worker_args=(--once)
  graph_behavior=revision
  catalog_behavior=existing
elif [[ ${mode} == "catalog-rebuild" ]]; then
  worker_args=(--once --full --full-catalog)
  graph_behavior=full
  catalog_behavior=rebuild
else
  worker_args=()
  graph_behavior=none
  catalog_behavior=preflight
fi
echo "precalculation mode=${mode} graph=${graph_behavior} catalog=${catalog_behavior}" >&2

lock_parent="${HOME}/.local/state/clustr"
lock_file="${lock_parent}/precalculation.lock"
mkdir -p -m 0700 "${lock_parent}"
chmod 0700 "${lock_parent}"

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

exec 9>"${lock_file}"
if ! flock -n 9; then
  echo "another Clustr calculation is already running" >&2
  exit 75
fi

ssh_options=(
  -F /dev/null
  -T
  -o BatchMode=yes
  -o ControlMaster=no
  -o ControlPath=none
  -o ControlPersist=no
  -o ServerAliveInterval=30
  -o ServerAliveCountMax=3
)

catalog_storage_gate() {
  local inspect_format inspect_command df_command available_gib

  inspect_format='{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Source}}{{end}}{{end}}'
  printf -v inspect_command 'docker inspect --format %q %q' "${inspect_format}" "${postgres_container}"
  if ! mount_source=$(timeout 15 ssh "${ssh_options[@]}" "${ssh_target}" \
    "${inspect_command}"); then
    echo "could not inspect Almaz PostgreSQL data mount; refusing catalog operation" >&2
    exit 69
  fi
  if [[ ${mount_source} != /mnt/spektr/* || ! ${mount_source} =~ ^/mnt/spektr/[A-Za-z0-9._/-]+$ || ${mount_source} =~ (^|/)\.\.(/|$) ]]; then
    echo "Almaz PostgreSQL data mount must be one safe path under /mnt/spektr/; refusing catalog operation" >&2
    exit 69
  fi
  printf -v df_command 'df -B1 --output=avail %q' "${mount_source}"
  if ! available_bytes=$(timeout 15 ssh "${ssh_options[@]}" "${ssh_target}" \
    "${df_command}"); then
    echo "could not inspect Almaz PostgreSQL filesystem space; refusing catalog operation" >&2
    exit 69
  fi
  available_bytes=$(printf '%s\n' "${available_bytes}" | awk 'NR == 2 {print $1}')
  if [[ ! ${available_bytes} =~ ^[0-9]+$ || ${#available_bytes} -gt 19 || ( ${#available_bytes} -eq 19 && ${available_bytes} > 9223372036854775807 ) ]]; then
    echo "Almaz PostgreSQL filesystem space was malformed; refusing catalog operation" >&2
    exit 69
  fi
  required_bytes=$((10#${catalog_min_free_gib} * 1024 * 1024 * 1024))
  available_gib=$(awk -v bytes="${available_bytes}" 'BEGIN { printf "%.9f", bytes / 1024 / 1024 / 1024 }')
  printf 'catalog storage preflight: source=%s available=%s GiB (%s bytes) required=%s GiB\n' \
    "${mount_source}" "${available_gib}" "${available_bytes}" "${catalog_min_free_gib}" >&2
  if (( 10#${available_bytes} < required_bytes )); then
    echo "Almaz PostgreSQL filesystem space is below the catalog rebuild threshold; refusing catalog operation" >&2
    exit 69
  fi
}

if [[ ${mode} != "graph-revision" ]]; then
  catalog_storage_gate
fi

if [[ ${mode} == "catalog-preflight" ]]; then
  exit 0
fi

if timeout 1 bash -c "</dev/tcp/127.0.0.1/${local_db_port}" >/dev/null 2>&1; then
  echo "local tunnel port ${local_db_port} is already in use; refusing an ambiguous database target" >&2
  exit 75
fi

ssh \
  -N \
  "${ssh_options[@]}" \
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
