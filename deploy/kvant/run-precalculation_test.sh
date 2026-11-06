#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
script="${script_dir}/run-precalculation.sh"
tmp=$(mktemp -d)
trap 'rm -rf "${tmp}"' EXIT

fail=0

assert_contains() {
  local needle=$1 file=$2
  if ! grep -F -- "${needle}" "${file}" >/dev/null; then
    printf 'missing %q in %s\n' "${needle}" "${file}" >&2
    fail=$((fail + 1))
  fi
}

assert_not_contains() {
  local needle=$1 file=$2
  if [[ -e ${file} ]] && grep -F -- "${needle}" "${file}" >/dev/null; then
    printf 'unexpected %q in %s\n' "${needle}" "${file}" >&2
    fail=$((fail + 1))
  fi
}

assert_status() {
  local expected=$1 actual=$2
  if [[ ${expected} != "${actual}" ]]; then
    printf 'expected status %s, got %s\n' "${expected}" "${actual}" >&2
    fail=$((fail + 1))
  fi
}

make_fakes() {
  fake_bin="${tmp}/bin"
  mkdir -p "${fake_bin}"
  cat >"${fake_bin}/timeout" <<'EOF'
#!/usr/bin/env bash
shift
if [[ $1 == bash ]]; then
  [[ -e "${FAKE_STATE}/tunnel-ready" ]] && exit 0
  exit 1
fi
"$@"
EOF
  cat >"${fake_bin}/ssh" <<'EOF'
#!/usr/bin/env bash
printf 'argv:' >>"${FAKE_STATE}/ssh.log"
printf ' <%q>' "$@" >>"${FAKE_STATE}/ssh.log"
printf '\n' >>"${FAKE_STATE}/ssh.log"
if [[ " $* " == *" -N "* ]]; then
  : >"${FAKE_STATE}/tunnel-ready"
  sleep 30
  exit 0
fi
remote_command=${!#}
printf 'remote=%s\n' "${remote_command}" >>"${FAKE_STATE}/ssh.log"
inspect_format='{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Source}}{{end}}{{end}}'
printf -v expected_inspect 'docker inspect --format %q %q' "${inspect_format}" 'pg17-clustr'
if [[ ${remote_command} == docker\ inspect* ]]; then
  [[ ${remote_command} == "${expected_inspect}" ]] || exit 97
  [[ ${remote_command} != *'\n'* && ${remote_command} != *$'\n'* ]] || exit 97
  [[ ${SSH_GATE_FAIL:-0} != 1 ]] || exit 1
  [[ ${SSH_INSPECT_FAIL:-0} != 1 ]] || exit 1
  printf '%s\n' "${MOUNT_SOURCE-/mnt/spektr/clustr-pgdata}"
elif [[ ${remote_command} == df\ -B1* ]]; then
  printf -v expected_df 'df -B1 --output=avail %q' "${MOUNT_SOURCE}"
  [[ ${remote_command} == "${expected_df}" ]] || exit 97
  [[ ${SSH_GATE_FAIL:-0} != 1 ]] || exit 1
  [[ ${SSH_DF_FAIL:-0} != 1 ]] || exit 1
  printf 'Avail\n%s\n' "${AVAILABLE_BYTES:-42949672960}"
fi
EOF
  cat >"${fake_bin}/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${FAKE_STATE}/docker.log"
if [[ $1 == run ]]; then
  while [[ $# -gt 0 ]]; do
    if [[ $1 == --cidfile ]]; then printf 'fake-container\n' >"$2"; break; fi
    shift
  done
fi
EOF
  chmod +x "${fake_bin}/timeout" "${fake_bin}/ssh" "${fake_bin}/docker"
}

write_env() {
  cat >"${tmp}/worker.env" <<EOF
CLUSTR_PRECALCULATE_IMAGE=example.test/precalculate@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
ALMAZ_LOCAL_DB_PORT=15436
ALMAZ_POSTGRES_CONTAINER=${CONTAINER_NAME:-pg17-clustr}
SPATIAL_CATALOG_MIN_FREE_GIB=${MIN_GIB:-40}
EOF
}

run_script() {
  local label=$1
  shift
  FAKE_STATE="${tmp}/${label}" PATH="${fake_bin}:$PATH" HOME="${tmp}/home-${label}" \
    XDG_RUNTIME_DIR="${tmp}/runtime-${label}" SSH_GATE_FAIL="${SSH_GATE_FAIL:-0}" \
    SSH_INSPECT_FAIL="${SSH_INSPECT_FAIL:-0}" SSH_DF_FAIL="${SSH_DF_FAIL:-0}" \
    MOUNT_SOURCE="${MOUNT_SOURCE-/mnt/spektr/clustr-pgdata}" \
    AVAILABLE_BYTES="${AVAILABLE_BYTES:-42949672960}" "$script" "${tmp}/worker.env" "$@" \
    >"${tmp}/${label}.out" 2>"${tmp}/${label}.err"
}

case_run() {
  local label=$1 expected=$2
  shift 2
  mkdir -p "${tmp}/${label}" "${tmp}/runtime-${label}"
  set +e
  run_script "${label}" "$@"
  status=$?
  set -e
  assert_status "${expected}" "${status}"
}

make_fakes
write_env

# Default and explicit modes map to their intended worker arguments.
case_run default 0
assert_contains '/app/precalculate --once' "${tmp}/default/docker.log"
assert_contains 'precalculation mode=graph-revision graph=revision catalog=existing' "${tmp}/default.err"
assert_contains '<10.0.0.200>' "${tmp}/default/ssh.log"
assert_contains '700' <(stat -c %a "${tmp}/home-default/.local/state/clustr")
case_run graph 0 graph-revision
assert_contains '/app/precalculate --once' "${tmp}/graph/docker.log"
case_run catalog 0 catalog-rebuild
assert_contains '/app/precalculate --once --full --full-catalog' "${tmp}/catalog/docker.log"
assert_contains 'precalculation mode=catalog-rebuild graph=full catalog=rebuild' "${tmp}/catalog.err"
assert_contains 'remote=docker inspect --format' "${tmp}/catalog/ssh.log"
assert_not_contains '\n' "${tmp}/catalog/ssh.log"
assert_contains 'remote=df -B1 --output=avail' "${tmp}/catalog/ssh.log"

# Graph work never asks Almaz about mounts or space, even when the gate would fail.
SSH_GATE_FAIL=1 case_run graph-bypass 0 graph-revision
if grep -F 'docker inspect' "${tmp}/graph-bypass/ssh.log" >/dev/null || grep -F 'df -B1' "${tmp}/graph-bypass/ssh.log" >/dev/null; then
  printf 'graph mode ran the catalog disk gate\n' >&2; fail=$((fail + 1))
fi

# Catalog rebuild gate rejects below threshold and accepts equality and excess.
AVAILABLE_BYTES=$((40 * 1024 * 1024 * 1024 - 1)) case_run below 69 catalog-rebuild
AVAILABLE_BYTES=$((40 * 1024 * 1024 * 1024)) case_run equal 0 catalog-rebuild
AVAILABLE_BYTES=$((40 * 1024 * 1024 * 1024 + 1)) case_run above 0 catalog-rebuild
MOUNT_SOURCE=/var/lib/postgresql/data case_run wrong-mount 69 catalog-rebuild
MOUNT_SOURCE= case_run empty-mount 69 catalog-rebuild
MOUNT_SOURCE=$'/mnt/spektr/first\n/mnt/spektr/second' case_run multiline-mount 69 catalog-rebuild
MOUNT_SOURCE='/mnt/spektr/bad path' case_run malformed-mount 69 catalog-rebuild
AVAILABLE_BYTES=not-a-number case_run malformed 69 catalog-rebuild
SSH_INSPECT_FAIL=1 case_run inspect-ssh-failure 69 catalog-rebuild
assert_contains 'could not inspect Almaz PostgreSQL data mount' "${tmp}/inspect-ssh-failure.err"
SSH_DF_FAIL=1 case_run df-ssh-failure 69 catalog-rebuild
assert_contains 'could not inspect Almaz PostgreSQL filesystem space' "${tmp}/df-ssh-failure.err"

# Catalog preflight performs the same gate but never opens a tunnel or runs a worker.
AVAILABLE_BYTES=$((40 * 1024 * 1024 * 1024 - 1)) case_run preflight-below 69 catalog-preflight
assert_contains 'catalog storage preflight: source=/mnt/spektr/clustr-pgdata' "${tmp}/preflight-below.err"
assert_contains 'available=39.999999999 GiB (42949672959 bytes) required=40 GiB' "${tmp}/preflight-below.err"
assert_contains 'below the catalog rebuild threshold' "${tmp}/preflight-below.err"
AVAILABLE_BYTES=$((40 * 1024 * 1024 * 1024)) case_run preflight-equal 0 catalog-preflight
AVAILABLE_BYTES=$((40 * 1024 * 1024 * 1024 + 1)) case_run preflight-above 0 catalog-preflight
assert_contains 'precalculation mode=catalog-preflight graph=none catalog=preflight' "${tmp}/preflight-equal.err"
for label in preflight-below preflight-equal preflight-above; do
  assert_not_contains '<-N>' "${tmp}/${label}/ssh.log"
  assert_not_contains 'run ' "${tmp}/${label}/docker.log"
done

# Literal and actual newlines from inspect must not be accepted as a mount path.
MOUNT_SOURCE='/mnt/spektr/clustr-pgdata\n' case_run literal-backslash-newline 69 catalog-rebuild
MOUNT_SOURCE=$'/mnt/spektr/clustr-pgdata\nextra' case_run newline-mount 69 catalog-rebuild

# Mode validation happens before work; legacy input cannot silently retain its old meaning.
case_run legacy 64 --initial-full
case_run unknown 64 unexpected
MIN_GIB=not-an-integer write_env
case_run invalid-minimum 65 catalog-rebuild
MIN_GIB=0 write_env
case_run zero-minimum 65 catalog-rebuild
assert_contains 'strictly positive integer' "${tmp}/zero-minimum.err"
CONTAINER_NAME='bad/name' MIN_GIB=40 write_env
case_run invalid-container 65 catalog-rebuild
unset CONTAINER_NAME MIN_GIB
write_env

# The exact HOME state lock is shared by every mode and refuses contention.
mkdir -p "${tmp}/home-contended/.local/state/clustr" "${tmp}/contended" "${tmp}/runtime-contended"
flock "${tmp}/home-contended/.local/state/clustr/precalculation.lock" \
  bash -c 'touch "$1"; sleep 30' _ "${tmp}/contended/lock-held" &
lock_holder=$!
while [[ ! -e ${tmp}/contended/lock-held ]]; do sleep 0.01; done
case_run contended 75 catalog-rebuild
case_run contended 75 catalog-preflight
kill "${lock_holder}"
wait "${lock_holder}" || true
assert_contains 'another Clustr calculation is already running' "${tmp}/contended.err"

# Both gate and tunnel retain the hardened, config-free SSH options; cleanup stops the container.
assert_contains '<-F> </dev/null>' "${tmp}/catalog/ssh.log"
assert_contains 'BatchMode=yes' "${tmp}/catalog/ssh.log"
assert_contains 'ControlMaster=no' "${tmp}/catalog/ssh.log"
assert_contains 'ControlPath=none' "${tmp}/catalog/ssh.log"
assert_contains 'ControlPersist=no' "${tmp}/catalog/ssh.log"
assert_contains 'ExitOnForwardFailure=yes' "${tmp}/catalog/ssh.log"
assert_contains 'stop --time 30 fake-container' "${tmp}/catalog/docker.log"

if (( fail > 0 )); then
  printf '%d assertions failed\n' "${fail}" >&2
  exit 1
fi
printf 'run-precalculation shell tests passed\n'
