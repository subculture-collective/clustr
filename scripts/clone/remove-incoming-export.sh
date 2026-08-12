#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

for command_name in find readlink sha256sum; do clone_require_command "$command_name"; done

incoming_root=${1:-}
clone_id=${2:-}
confirmation=${3:-}
clone_absolute_dir "$incoming_root"
clone_validate_id "$clone_id"
[[ $confirmation == "--confirm-${clone_id}" ]] || clone_die "removal requires --confirm-${clone_id}"
export_dir="${incoming_root}/${clone_id}"
[[ -f "${export_dir}/READY" ]] || clone_die "incoming export is not complete"
(cd "$export_dir" && sha256sum --check --strict SHA256SUMS >/dev/null)
resolved_root=$(readlink -f "$incoming_root")
resolved_export=$(readlink -f "$export_dir")
[[ $resolved_export == "${resolved_root}/${clone_id}" ]] || clone_die "export path escaped its configured root"
find "$resolved_export" -xdev -depth -delete
echo "removed encrypted incoming export $clone_id"
