#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
host="${WEBUI_HOST:-127.0.0.1}"
port="${WEBUI_PORT:-5173}"

cd "$repo_root"
exec npm run dev -- --host "$host" --port "$port" "$@"
