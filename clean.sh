#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! git -C "$repo_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  printf 'clean.sh must be run from inside the liteLLMwiki Git checkout.\n' >&2
  exit 1
fi

printf 'Cleaning ignored and untracked files from %s\n' "$repo_root"
printf 'Preserving models/\n'
printf 'Running: git clean -xdf -e models/\n'

git -C "$repo_root" clean -xdf -e models/
