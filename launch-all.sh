#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
webui_pid=""
sidecar_pid=""

cleanup() {
  local pid

  for pid in "${webui_pid:-}" "${sidecar_pid:-}"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}

trap cleanup EXIT INT TERM

SIDECAR_HEADLESS="${SIDECAR_HEADLESS:-1}" "$repo_root/launch-sidecar.sh" "$@" &
sidecar_pid="$!"

"$repo_root/launch-webui.sh" &
webui_pid="$!"

printf 'Web UI started with PID %s\n' "$webui_pid"
printf 'Sidecar started with PID %s\n' "$sidecar_pid"

while true; do
  if ! kill -0 "$sidecar_pid" 2>/dev/null; then
    status=0
    wait "$sidecar_pid" || status="$?"
    exit "$status"
  fi

  if ! kill -0 "$webui_pid" 2>/dev/null; then
    status=0
    wait "$webui_pid" || status="$?"
    exit "$status"
  fi

  sleep 1
done
