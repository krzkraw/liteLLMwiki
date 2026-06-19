#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$repo_root/launch-sidecar.sh" "$@"
"$repo_root/launch-webui.sh"

printf 'Opened LiteRT Sidecar TUI in a separate terminal.\n'
printf 'Opened LiteRT Web UI in a separate terminal.\n'
