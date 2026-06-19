#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
llama_runtime_root="$repo_root/native/llama-runtimes"
llama_selected_file="$llama_runtime_root/.selected"

sidecar_args=()
inline_launch=0

if [[ "${1:-}" == "--litert-launch-inline" ]]; then
  inline_launch=1
  shift
fi

shell_join() {
  local arg
  local quoted
  local result=""

  for arg in "$@"; do
    printf -v quoted "%q" "$arg"
    result+="$quoted "
  done

  printf '%s' "${result% }"
}

escape_applescript() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}

launcher_env_assignments() {
  local name
  local value

  for name in \
    SIDECAR_BIN \
    SIDECAR_ADDR \
    SIDECAR_UPSTREAM \
    LITERT_LM_BIN \
    SIDECAR_RUNTIME_HOST \
    SIDECAR_RUNTIME_PORT \
    MODEL_FILE \
    MODEL_ID \
    SIDECAR_LAUNCH_RUNTIME \
    SIDECAR_IMPORT_MODEL \
    SIDECAR_RUNTIME_VERBOSE \
    LLAMA_RUNTIME \
    LLAMA_SERVER_BIN
  do
    value="${!name:-}"
    if [[ -n "$value" ]]; then
      printf '%s=%s ' "$name" "$(shell_join "$value")"
    fi
  done
}

launch_terminal() {
  local title="$1"
  shift
  local command

  command="cd $(shell_join "$repo_root") && $(launcher_env_assignments)LITERT_SIDECAR_TUI=1 $(shell_join "$@")"

  if [[ "$(uname -s)" == "Darwin" ]] && command -v osascript >/dev/null 2>&1; then
    osascript <<OSA
tell application "Terminal"
  activate
  do script "$(escape_applescript "$command")"
end tell
OSA
    return 0
  fi

  if command -v gnome-terminal >/dev/null 2>&1; then
    gnome-terminal --title="$title" -- bash -lc "$command; exec bash" >/dev/null 2>&1 &
    return 0
  fi

  if command -v konsole >/dev/null 2>&1; then
    konsole -p "tabtitle=$title" -e bash -lc "$command; exec bash" >/dev/null 2>&1 &
    return 0
  fi

  if command -v xterm >/dev/null 2>&1; then
    xterm -T "$title" -e bash -lc "$command; exec bash" >/dev/null 2>&1 &
    return 0
  fi

  printf 'No supported terminal launcher found. Run this command manually:\n%s\n' "$command" >&2
  return 1
}

if [[ "$inline_launch" != "1" ]]; then
  launch_terminal "LiteRT Sidecar TUI" "$repo_root/launch-sidecar.sh" "--litert-launch-inline" "$@"
  exit 0
fi

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

llama_executable_name() {
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) printf 'llama-server.exe\n' ;;
    *) printf 'llama-server\n' ;;
  esac
}

find_llama_server_bin() {
  local executable_name
  local runtime_name
  local candidate

  executable_name="$(llama_executable_name)"

  if [[ -n "${LLAMA_SERVER_BIN:-}" && -f "${LLAMA_SERVER_BIN:-}" ]]; then
    printf '%s\n' "$LLAMA_SERVER_BIN"
    return 0
  fi

  if [[ -n "${LLAMA_RUNTIME:-}" ]]; then
    candidate="$llama_runtime_root/$LLAMA_RUNTIME"
    if [[ -d "$candidate" ]]; then
      find "$candidate" -type f -name "$executable_name" -print -quit
      return 0
    fi
  fi

  if [[ -f "$llama_selected_file" ]]; then
    runtime_name="$(tr -d '\r\n' < "$llama_selected_file")"
    candidate="$llama_runtime_root/$runtime_name"
    if [[ -d "$candidate" ]]; then
      find "$candidate" -type f -name "$executable_name" -print -quit
      return 0
    fi
  fi

  if [[ -d "$llama_runtime_root" ]]; then
    find "$llama_runtime_root" -type f -name "$executable_name" -print -quit
  fi
}

prepend_llama_runtime_path() {
  local llama_server_bin
  local llama_server_dir

  llama_server_bin="$(find_llama_server_bin || true)"
  if [[ -n "$llama_server_bin" ]]; then
    llama_server_dir="$(dirname "$llama_server_bin")"
    export PATH="$llama_server_dir:$PATH"
  fi
}

exec_sidecar() {
  local executable="$1"
  shift

  if [[ "${#sidecar_args[@]}" -gt 0 ]]; then
    exec "$executable" "${sidecar_args[@]}" "$@"
  fi

  exec "$executable" "$@"
}

if [[ "${LITERT_SIDECAR_TUI:-}" != "1" ]]; then
  case "${SIDECAR_HEADLESS:-}" in
    1|true|TRUE|yes|YES) sidecar_args+=("--headless") ;;
  esac
fi

prepend_llama_runtime_path

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
  exec_sidecar "$sidecar_bin" "$@"
fi

cd "$repo_root/native/sidecar"
if [[ "${#sidecar_args[@]}" -gt 0 ]]; then
  exec go run ./cmd/litert-sidecar "${sidecar_args[@]}" "$@"
fi

exec go run ./cmd/litert-sidecar "$@"
