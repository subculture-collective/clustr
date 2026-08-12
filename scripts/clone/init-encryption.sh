#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${script_dir}/lib.sh"

gpg_home=${1:-}
clone_absolute_dir "$gpg_home"
clone_require_command gpg
if [[ -d $gpg_home ]]; then
  [[ -z $(find "$gpg_home" -mindepth 1 -maxdepth 1 -print -quit) ]] ||
    clone_die "refusing to modify nonempty GPG home: $gpg_home"
  chmod 700 "$gpg_home"
else
  [[ ! -e $gpg_home ]] || clone_die "GPG home exists and is not a directory: $gpg_home"
  install -d -m 700 "$gpg_home"
fi
gpg --batch --homedir "$gpg_home" --passphrase '' \
  --quick-generate-key "Clustr Kvant Clone <clustr-clone@localhost>" rsa4096 encr 2y
fingerprint=$(gpg --batch --homedir "$gpg_home" --with-colons --list-secret-keys |
  awk -F: '$1=="fpr" {print $10; exit}')
[[ $fingerprint =~ ^[0-9A-Fa-f]{40,64}$ ]] || clone_die "failed to create encryption key"
gpg --batch --homedir "$gpg_home" --armor --export "$fingerprint" >"${gpg_home}/public-key.asc"
chmod 600 "${gpg_home}/public-key.asc"
echo "clone encryption key created in $gpg_home"
echo "KVANT_GPG_RECIPIENT=$fingerprint"
