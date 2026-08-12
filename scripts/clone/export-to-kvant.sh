#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

config=${1:-}
mode=${2:---preflight}
clone_id=${3:-}
[[ -r $config ]] || clone_die "usage: $0 CONFIG {--preflight|--execute|--resume|--cleanup-source} [clone-id] [confirmation]"
for command_name in ssh rsync jq sha256sum flock timeout base64 gpg openssl; do
  clone_require_command "$command_name"
done

source_host=$(clone_env_value "$config" SOURCE_SSH_TARGET)
source_root=$(clone_env_value "$config" SOURCE_EXPORT_ROOT)
source_container=$(clone_env_value "$config" SOURCE_POSTGRES_CONTAINER)
database=$(clone_env_value "$config" SOURCE_DATABASE)
clone_root=$(clone_env_value "$config" KVANT_CLONE_ROOT)
incoming_root=$(clone_env_value "$config" KVANT_INCOMING_ROOT)
host_port=$(clone_env_value "$config" KVANT_CLONE_PORT)
restore_jobs=$(clone_env_value "$config" KVANT_RESTORE_JOBS)
gpg_home=$(clone_env_value "$config" KVANT_GPG_HOMEDIR)
gpg_recipient=$(clone_env_value "$config" KVANT_GPG_RECIPIENT)
: "${source_host:=almaz}" "${source_root:=/mnt/spektr/server/projects/data/clustr-exports}"
: "${source_container:=pg17-clustr}" "${database:=reddit_cluster}"
: "${clone_root:=/mnt/data2/clustr-clones}" "${incoming_root:=/mnt/data2/clustr-exports}"
: "${host_port:=55440}" "${restore_jobs:=8}"
: "${gpg_home:=/etc/clustr/clone-gpg}"
clone_absolute_dir "$source_root"
clone_absolute_dir "$clone_root"
clone_absolute_dir "$incoming_root"
clone_absolute_dir "$gpg_home"
clone_validate_name "$database"

remote_source() {
  {
    cat "${script_dir}/lib.sh"
    echo 'export CLUSTR_CLONE_LIB_LOADED=1'
    cat "${script_dir}/source-export.sh"
  } | ssh -o BatchMode=yes -o ExitOnForwardFailure=yes "$source_host" \
    "bash -s -- $(printf '%q ' "$@")"
}

if [[ $mode == --cleanup-source ]]; then
  clone_validate_id "$clone_id"
  [[ ${4:-} == "--confirm-${clone_id}" ]] || clone_die "source cleanup requires --confirm-${clone_id}"
  remote_source cleanup "$clone_id" "$source_root" "$source_container" "$database" "${4}"
  exit 0
fi

[[ $mode == --preflight || $mode == --execute || $mode == --resume ]] ||
  clone_die "mode must be --preflight, --execute, --resume, or --cleanup-source"
[[ $gpg_recipient =~ ^[0-9A-Fa-f]{40,64}$ ]] || clone_die "KVANT_GPG_RECIPIENT must be a full fingerprint"
gpg_public_key_b64=$(gpg --batch --homedir "$gpg_home" --export "$gpg_recipient" | base64 -w0)
[[ -n $gpg_public_key_b64 ]] || clone_die "could not export the Kvant clone public key"

remote_preflight=$(remote_source preflight '' "$source_root" "$source_container" "$database")
source_bytes=$(jq -er '.database_size_bytes' <<<"$remote_preflight")
incoming_parent=$(dirname "$incoming_root")
clone_parent=$(dirname "$clone_root")
incoming_capacity_path=$incoming_parent
clone_capacity_path=$clone_parent
[[ ! -d $incoming_root ]] || incoming_capacity_path=$incoming_root
[[ ! -d $clone_root ]] || clone_capacity_path=$clone_root
[[ -d $incoming_capacity_path && -w $incoming_capacity_path ]] ||
  clone_die "incoming storage is unavailable or not writable: $incoming_capacity_path"
[[ -d $clone_capacity_path && -w $clone_capacity_path ]] ||
  clone_die "clone storage is unavailable or not writable: $clone_capacity_path"
clone_require_free_bytes "$incoming_capacity_path" "$source_bytes"
clone_require_free_bytes "$clone_capacity_path" "$((source_bytes * 2 + source_bytes / 2))"
if timeout 1 bash -c "</dev/tcp/127.0.0.1/${host_port}" >/dev/null 2>&1; then
  clone_die "configured clone port is already in use: $host_port"
fi

if [[ $mode == --preflight ]]; then
  jq -n --argjson source "$remote_preflight" --arg destination_host "$(hostname)" \
    --arg incoming_root "$incoming_root" --arg clone_root "$clone_root" \
    '{status:"preflight-ok",source:$source,destination_host:$destination_host,incoming_root:$incoming_root,clone_root:$clone_root}'
  exit 0
fi
install -d -m 700 "$incoming_root" "$clone_root"
if [[ -z $clone_id ]]; then
  clone_id="clustr-$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"
fi
clone_validate_id "$clone_id"

exec 8>"${incoming_root}/.pipeline.lock"
flock -n 8 || clone_die "another clone pipeline is running"
if [[ $mode == --execute ]]; then
  if remote_source status "$clone_id" "$source_root" "$source_container" "$database" >/dev/null 2>&1; then
    echo "resuming existing verified source export: $clone_id"
  else
    remote_source export "$clone_id" "$source_root" "$source_container" "$database" "$gpg_recipient" "$gpg_public_key_b64"
  fi
else
  remote_source status "$clone_id" "$source_root" "$source_container" "$database"
fi

incoming="${incoming_root}/${clone_id}"
install -d -m 700 "$incoming"
rsync -a --partial --append-verify --protect-args \
  "${source_host}:${source_root}/${clone_id}/" "$incoming/"
(cd "$incoming" && sha256sum --check --strict SHA256SUMS)
"${script_dir}/restore-clone.sh" "$incoming" "$clone_root" "$host_port" "$restore_jobs" "$gpg_home"
echo "source export retained for explicit cleanup: ${source_host}:${source_root}/${clone_id}"
