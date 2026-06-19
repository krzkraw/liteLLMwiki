#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

sidecar_args=()

add_value_flag() {
  local env_name="$1"
  local flag_name="$2"
  local value="${!env_name:-}"

  if [[ -n "$value" ]]; then
    sidecar_args+=("$flag_name" "$value")
  fi
}

add_bool_flag() {
  local env_name="$1"
  local flag_name="$2"
  local value="${!env_name:-}"

  if [[ -n "$value" ]]; then
    sidecar_args+=("$flag_name=$value")
  fi
}

default_sidecar_bin() {
  local os_name
  local arch_name
  local os_suffix
  local arch_suffix

  os_name="$(uname -s)"
  arch_name="$(uname -m)"

  case "$os_name" in
    Darwin) os_suffix="darwin" ;;
    Linux) os_suffix="linux" ;;
    *) return 1 ;;
  esac

  case "$arch_name" in
    arm64|aarch64) arch_suffix="arm64" ;;
    x86_64|amd64) arch_suffix="amd64" ;;
    *) return 1 ;;
  esac

  printf '%s/native/sidecar-artifacts/litert-sidecar-%s-%s/litert-sidecar\n' \
    "$repo_root" "$os_suffix" "$arch_suffix"
}

case "${SIDECAR_HEADLESS:-}" in
  1|true|TRUE|yes|YES) sidecar_args+=("--headless") ;;
esac

add_value_flag "SIDECAR_ADDR" "-addr"
add_value_flag "SIDECAR_UPSTREAM" "-upstream"
add_value_flag "LITERT_LM_BIN" "-runtime-exe"
add_value_flag "SIDECAR_RUNTIME_HOST" "-runtime-host"
add_value_flag "SIDECAR_RUNTIME_PORT" "-runtime-port"
add_value_flag "MODEL_FILE" "-model-file"
add_value_flag "MODEL_ID" "-model-id"
add_bool_flag "SIDECAR_LAUNCH_RUNTIME" "-launch-runtime"
add_bool_flag "SIDECAR_IMPORT_MODEL" "-import-model"
add_bool_flag "SIDECAR_RUNTIME_VERBOSE" "-runtime-verbose"

sidecar_bin="${SIDECAR_BIN:-}"
if [[ -z "$sidecar_bin" ]]; then
  sidecar_bin="$(default_sidecar_bin || true)"
fi

if [[ -n "$sidecar_bin" && -x "$sidecar_bin" ]]; then
  exec "$sidecar_bin" "${sidecar_args[@]}" "$@"
fi

cd "$repo_root/native/sidecar"
exec go run ./cmd/litert-sidecar "${sidecar_args[@]}" "$@"
