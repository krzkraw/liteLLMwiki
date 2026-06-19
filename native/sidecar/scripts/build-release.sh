#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_root="$(cd "$root/../.." && pwd)"
out="${1:-"$repo_root/native/sidecar-artifacts"}"

targets=(
  "darwin/arm64"
  "darwin/amd64"
  "windows/amd64"
  "windows/arm64"
)

mkdir -p "$out"

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  dir="$out/litert-sidecar-$goos-$goarch"
  bin="litert-sidecar"
  if [[ "$goos" == "windows" ]]; then
    bin="$bin.exe"
  fi

  rm -rf "$dir"
  mkdir -p "$dir"

  (
    cd "$root"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "-s -w" -o "$dir/$bin" ./cmd/litert-sidecar
  )

  cp "$root/README.md" "$dir/README.md"
done

printf 'Built sidecar release artifacts in %s\n' "$out"
