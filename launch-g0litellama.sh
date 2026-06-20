#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
litert_runtime_root="$repo_root/G0LiteLLaMa/litert-runtimes"
litert_selected_file="$litert_runtime_root/.selected"
llama_runtime_root="$repo_root/G0LiteLLaMa/llama-runtimes"
llama_selected_file="$llama_runtime_root/.selected"

G0LiteLLaMa_args=()
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

macos_terminal_app() {
  local candidate="${LITERT_TERMINAL_APP:-${TERM_PROGRAM:-}}"
  case "$candidate" in
    Ghostty|ghostty|Ghostty.app) printf 'Ghostty' ;;
    Apple_Terminal|Terminal|Terminal.app|"") printf 'Terminal' ;;
    *) printf '%s' "$candidate" ;;
  esac
}

launch_macos_ghostty() {
  local command="$1"

  osascript <<OSA
tell application "Ghostty"
  activate
  set cfg to new surface configuration
  set initial working directory of cfg to "$(escape_applescript "$repo_root")"
  set initial input of cfg to "$(escape_applescript "$command")" & linefeed
  set win to new window with configuration cfg
end tell
OSA
}

launch_macos_terminal_app() {
  local command="$1"

  osascript <<OSA
tell application "Terminal"
  activate
  do script "$(escape_applescript "$command")"
end tell
OSA
}

launcher_env_assignments() {
  local name
  local value

  for name in \
    G0LITELLAMA_BIN \
    G0LITELLAMA_ADDR \
    G0LITELLAMA_UPSTREAM \
    LITERT_LM_BIN \
    G0LITELLAMA_RUNTIME_HOST \
    G0LITELLAMA_RUNTIME_PORT \
    MODEL_FILE \
    MODEL_ID \
    G0LITELLAMA_LAUNCH_RUNTIME \
    G0LITELLAMA_IMPORT_MODEL \
    G0LITELLAMA_RUNTIME_VERBOSE \
    LITERT_RUNTIME \
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

  command="cd $(shell_join "$repo_root") && $(launcher_env_assignments)G0LITELLAMA_TUI=1 $(shell_join "$@")"

  if [[ "$(uname -s)" == "Darwin" ]] && command -v osascript >/dev/null 2>&1; then
    case "$(macos_terminal_app)" in
      Ghostty) launch_macos_ghostty "$command" ;;
      *) launch_macos_terminal_app "$command" ;;
    esac
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
  launch_terminal "G0LiteLLaMa TUI" "$repo_root/launch-g0litellama.sh" "--litert-launch-inline" "$@"
  exit 0
fi

add_value_flag() {
  local env_name="$1"
  local flag_name="$2"
  local value="${!env_name:-}"

  if [[ -n "$value" ]]; then
    G0LiteLLaMa_args+=("$flag_name" "$value")
  fi
}

add_bool_flag() {
  local env_name="$1"
  local flag_name="$2"
  local value="${!env_name:-}"

  if [[ -n "$value" ]]; then
    G0LiteLLaMa_args+=("$flag_name=$value")
  fi
}

default_G0LiteLLaMa_bin() {
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

  printf '%s/G0LiteLLaMa/dist/g0litellama-%s-%s/g0litellama\n' \
    "$repo_root" "$os_suffix" "$arch_suffix"
}

llama_executable_name() {
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) printf 'llama-server.exe\n' ;;
    *) printf 'llama-server\n' ;;
  esac
}

litert_executable_name() {
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) printf 'litert-lm.exe\n' ;;
    *) printf 'litert-lm\n' ;;
  esac
}

find_litert_lm_bin() {
  local executable_name
  local runtime_name
  local candidate

  executable_name="$(litert_executable_name)"

  if [[ -n "${LITERT_LM_BIN:-}" && -f "${LITERT_LM_BIN:-}" ]]; then
    printf '%s\n' "$LITERT_LM_BIN"
    return 0
  fi

  if [[ -n "${LITERT_RUNTIME:-}" ]]; then
    candidate="$litert_runtime_root/$LITERT_RUNTIME"
    if [[ -d "$candidate" ]]; then
      find "$candidate" -type f -name "$executable_name" -print -quit
      return 0
    fi
  fi

  if [[ -f "$litert_selected_file" ]]; then
    runtime_name="$(tr -d '\r\n' < "$litert_selected_file")"
    candidate="$litert_runtime_root/$runtime_name"
    if [[ -d "$candidate" ]]; then
      find "$candidate" -type f -name "$executable_name" -print -quit
      return 0
    fi
  fi

  if [[ -d "$litert_runtime_root" ]]; then
    find "$litert_runtime_root" -type f -name "$executable_name" -print -quit
  fi
}

configure_litert_runtime_bin() {
  local litert_lm_bin
  local litert_lm_dir

  litert_lm_bin="$(find_litert_lm_bin || true)"
  if [[ -n "$litert_lm_bin" ]]; then
    export LITERT_LM_BIN="$litert_lm_bin"
    litert_lm_dir="$(dirname "$litert_lm_bin")"
    export PATH="$litert_lm_dir:$PATH"
  fi
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

exec_G0LiteLLaMa() {
  local executable="$1"
  shift

  if [[ "${#G0LiteLLaMa_args[@]}" -gt 0 ]]; then
    exec "$executable" "${G0LiteLLaMa_args[@]}" "$@"
  fi

  exec "$executable" "$@"
}

if [[ "${G0LITELLAMA_TUI:-}" != "1" ]]; then
  case "${G0LITELLAMA_HEADLESS:-}" in
    1|true|TRUE|yes|YES) G0LiteLLaMa_args+=("--headless") ;;
  esac
fi

prepend_llama_runtime_path

configure_litert_runtime_bin

add_value_flag "G0LITELLAMA_ADDR" "-addr"
add_value_flag "G0LITELLAMA_UPSTREAM" "-upstream"
add_value_flag "LITERT_LM_BIN" "-runtime-exe"
add_value_flag "G0LITELLAMA_RUNTIME_HOST" "-runtime-host"
add_value_flag "G0LITELLAMA_RUNTIME_PORT" "-runtime-port"
add_value_flag "MODEL_FILE" "-model-file"
add_value_flag "MODEL_ID" "-model-id"
add_bool_flag "G0LITELLAMA_LAUNCH_RUNTIME" "-launch-runtime"
add_bool_flag "G0LITELLAMA_IMPORT_MODEL" "-import-model"
add_bool_flag "G0LITELLAMA_RUNTIME_VERBOSE" "-runtime-verbose"

G0LiteLLaMa_bin="${G0LITELLAMA_BIN:-}"
if [[ -z "$G0LiteLLaMa_bin" ]]; then
  G0LiteLLaMa_bin="$(default_G0LiteLLaMa_bin || true)"
fi

if [[ -n "$G0LiteLLaMa_bin" && -x "$G0LiteLLaMa_bin" ]]; then
  exec_G0LiteLLaMa "$G0LiteLLaMa_bin" "$@"
fi

cd "$repo_root/G0LiteLLaMa"
if [[ "${#G0LiteLLaMa_args[@]}" -gt 0 ]]; then
  exec go run ./cmd/g0litellama "${G0LiteLLaMa_args[@]}" "$@"
fi

exec go run ./cmd/g0litellama "$@"
