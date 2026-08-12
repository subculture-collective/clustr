#!/usr/bin/env bash

clone_die() {
  echo "clustr-clone: $*" >&2
  exit 1
}

clone_require_command() {
  command -v "$1" >/dev/null 2>&1 || clone_die "required command is missing: $1"
}

clone_validate_id() {
  [[ $1 =~ ^clustr-[0-9]{8}T[0-9]{6}Z-[a-z0-9]{6,16}$ ]] ||
    clone_die "invalid clone id: $1"
}

clone_validate_name() {
  [[ $1 =~ ^[a-zA-Z_][a-zA-Z0-9_]{0,62}$ ]] ||
    clone_die "invalid PostgreSQL identifier: $1"
}

clone_absolute_dir() {
  [[ $1 == /* && $1 != / ]] || clone_die "path must be an absolute non-root directory: $1"
}

clone_env_value() {
  local file=$1 key=$2
  awk -F= -v wanted="$key" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}' "$file"
}

clone_require_free_bytes() {
  local path=$1 required=$2 available
  available=$(df -B1 --output=avail "$path" | tail -1 | tr -d ' ')
  [[ $available =~ ^[0-9]+$ ]] || clone_die "could not determine free space for $path"
  (( available >= required )) ||
    clone_die "insufficient free space at $path: need $required bytes, have $available"
}
