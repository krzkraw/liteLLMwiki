#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
host="${WEBUI_HOST:-127.0.0.1}"
port="${WEBUI_PORT:-5173}"
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

launch_terminal() {
  local title="$1"
  shift
  local command

  command="cd $(shell_join "$repo_root") && WEBUI_HOST=$(shell_join "$host") WEBUI_PORT=$(shell_join "$port") $(shell_join "$@")"

  if [[ "$(uname -s)" == "Darwin" ]] && command -v osascript >/dev/null 2>&1; then
    osascript <<OSA
tell application "Terminal"
  activate
  set terminalWindow to (make new window)
  do script "$(escape_applescript "$command")" in selected tab of terminalWindow
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
  launch_terminal "LiteRT Web UI" "$repo_root/launch-webui.sh" "--litert-launch-inline" "$@"
  exit 0
fi

cd "$repo_root"
exec bun run dev --host "$host" --port "$port" "$@"
