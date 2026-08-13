#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "run as root: $0 [repository-root]" >&2
  exit 77
fi

repo_root=${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
[[ $repo_root == /* && -d ${repo_root}/scripts/clone ]] || {
  echo "invalid repository root: $repo_root" >&2
  exit 64
}
getent passwd onnwee >/dev/null || {
  echo "required service user does not exist: onnwee" >&2
  exit 65
}

install -d -m 0755 /opt/clustr/scripts/clone /opt/clustr/docs/runbooks /opt/clustr/deploy/clone /opt/clustr/deploy/kvant
install -d -m 0750 -o onnwee -g onnwee /mnt/data2/clustr-exports /mnt/data2/clustr-clones
install -d -m 0755 /etc/clustr
install -m 0755 "${repo_root}"/scripts/clone/*.sh /opt/clustr/scripts/clone/
install -m 0644 "${repo_root}/docs/runbooks/production-database-clone.md" /opt/clustr/docs/runbooks/
install -m 0644 "${repo_root}/deploy/clone/clustr-clone.env.example" /opt/clustr/deploy/clone/
install -m 0644 "${repo_root}/deploy/kvant/clustr-clone-export@.service" /etc/systemd/system/
install -m 0755 "${repo_root}/deploy/kvant/run-clone-calculation.sh" /opt/clustr/deploy/kvant/
install -m 0644 "${repo_root}/deploy/kvant/clustr-clone-calculate@.service" /etc/systemd/system/
install -m 0644 "${repo_root}/deploy/kvant/clone-calculation.env.example" /opt/clustr/deploy/kvant/

install -m 0640 -o root -g onnwee \
  "${repo_root}/deploy/clone/clustr-clone.env.example" /etc/clustr/clustr-clone.env.example

if [[ ! -d /etc/clustr/clone-gpg ]]; then
  install -d -m 0700 -o onnwee -g onnwee /etc/clustr/clone-gpg
fi
if [[ -z $(find /etc/clustr/clone-gpg -mindepth 1 -maxdepth 1 -print -quit) ]]; then
  runuser -u onnwee -- /opt/clustr/scripts/clone/init-encryption.sh /etc/clustr/clone-gpg >/dev/null
fi
fingerprint=$(runuser -u onnwee -- gpg --batch --homedir /etc/clustr/clone-gpg \
  --with-colons --list-secret-keys | awk -F: '$1=="fpr" {print $10; exit}')
[[ $fingerprint =~ ^[0-9A-Fa-f]{40,64}$ ]] || {
  echo "Kvant clone key is unavailable or invalid" >&2
  exit 65
}

if [[ ! -e /etc/clustr/clustr-clone.env ]]; then
  rendered_env=$(mktemp /run/clustr-clone.env.XXXXXX)
  trap 'rm -f "$rendered_env"' EXIT
  awk -v fingerprint="$fingerprint" \
    '$1 == "KVANT_GPG_RECIPIENT=replace_with_full_fingerprint" {$0="KVANT_GPG_RECIPIENT=" fingerprint} {print}' \
    "${repo_root}/deploy/clone/clustr-clone.env.example" >"$rendered_env"
  install -m 0640 -o root -g onnwee "$rendered_env" /etc/clustr/clustr-clone.env
fi
if [[ ! -e /etc/clustr/clone-calculation.env ]]; then
  install -m 0640 -o root -g onnwee \
    "${repo_root}/deploy/kvant/clone-calculation.env.example" /etc/clustr/clone-calculation.env.example
fi

systemctl daemon-reload
echo "installed Clustr clone export/calculation services; no job was started"
echo "Kvant clone encryption fingerprint: $fingerprint"
