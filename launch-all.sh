#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

sidecar_env_args() {
  local name
  local value

  printf '%s\n' "LITERT_SIDECAR_TUI=1"
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
      printf '%s\n' "$name=$value"
    fi
  done
}

launch_terminal() {
  local title="$1"
  shift
  local command

  command="cd $(shell_join "$repo_root") && $(shell_join "$@")"

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

webui_env_args=(
  "WEBUI_HOST=${WEBUI_HOST:-127.0.0.1}"
  "WEBUI_PORT=${WEBUI_PORT:-5173}"
)
sidecar_env_args=()
while IFS= read -r env_arg; do
  sidecar_env_args+=("$env_arg")
done < <(sidecar_env_args)

launch_terminal \
  "LiteRT Web UI" \
  env \
  "${webui_env_args[@]}" \
  "$repo_root/launch-webui.sh" \
  "--litert-launch-inline"

launch_terminal \
  "LiteRT Sidecar TUI" \
  env \
  "${sidecar_env_args[@]}" \
  "$repo_root/launch-sidecar.sh" \
  "--litert-launch-inline" \
  "$@"

printf 'Opened LiteRT Web UI in a separate terminal.\n'
printf 'Opened LiteRT Sidecar TUI in a separate terminal.\n'
