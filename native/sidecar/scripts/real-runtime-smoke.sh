#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_root="$(cd "$root/../.." && pwd)"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

pick_port() {
  local fallback="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
    return
  fi

  printf '%s\n' "$fallback"
}

require_cmd curl
require_cmd go
require_cmd node

if [[ -n "${LITERT_LM_BIN:-}" ]]; then
  runtime_exe="$LITERT_LM_BIN"
elif command -v litert-lm >/dev/null 2>&1; then
  runtime_exe="$(command -v litert-lm)"
else
  printf 'Set LITERT_LM_BIN or install litert-lm on PATH.\n' >&2
  exit 1
fi

if [[ ! -x "$runtime_exe" ]]; then
  printf 'LiteRT-LM executable is not runnable: %s\n' "$runtime_exe" >&2
  exit 1
fi

model_file="${MODEL_FILE:-"$repo_root/models/gemma-4-E2B-it.litertlm"}"
model_id="${MODEL_ID:-gemma4-e2b}"

if [[ ! -f "$model_file" ]]; then
  printf 'Model file not found: %s\n' "$model_file" >&2
  exit 1
fi

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/litert-sidecar-smoke.XXXXXX")"
sidecar_log="$work_dir/sidecar.log"
sidecar_bin="$work_dir/litert-sidecar"
sidecar_pid=""

if [[ -n "${LITERT_HOME:-}" ]]; then
  mkdir -p "$LITERT_HOME"
  litert_home="$(cd "$LITERT_HOME" && pwd)"
else
  litert_home="$work_dir/litert-home"
  mkdir -p "$litert_home"
fi

cleanup() {
  local status=$?

  if [[ -n "$sidecar_pid" ]] && kill -0 "$sidecar_pid" >/dev/null 2>&1; then
    kill "$sidecar_pid" >/dev/null 2>&1 || true
    wait "$sidecar_pid" >/dev/null 2>&1 || true
  fi

  if [[ "$status" -ne 0 && -f "$sidecar_log" ]]; then
    printf '\nSidecar log:\n' >&2
    sed -n '1,220p' "$sidecar_log" >&2
  fi

  rm -rf "$work_dir"
}
trap cleanup EXIT

sidecar_host="${SIDECAR_HOST:-127.0.0.1}"
sidecar_port="${SIDECAR_PORT:-"$(pick_port 9479)"}"
runtime_host="${RUNTIME_HOST:-127.0.0.1}"
runtime_port="${RUNTIME_PORT:-"$(pick_port 9481)"}"
sidecar_addr="$sidecar_host:$sidecar_port"
sidecar_url="http://$sidecar_addr"
upstream_url="http://$runtime_host:$runtime_port"
ready_timeout="${READY_TIMEOUT_SECONDS:-180}"
chat_timeout="${CHAT_TIMEOUT_SECONDS:-240}"
multimodal_timeout="${MULTIMODAL_TIMEOUT_SECONDS:-360}"

printf 'Building sidecar...\n'
(
  cd "$root"
  go build -o "$sidecar_bin" ./cmd/litert-sidecar
)

sidecar_args=(
  -addr "$sidecar_addr"
  -upstream "$upstream_url"
  -runtime-exe "$runtime_exe"
  -runtime-host "$runtime_host"
  -runtime-port "$runtime_port"
  -model-file "$model_file"
  -model-id "$model_id"
)

if [[ "${RUNTIME_VERBOSE:-0}" == "1" ]]; then
  sidecar_args+=(-runtime-verbose)
fi

printf 'Starting sidecar at %s with isolated HOME=%s...\n' "$sidecar_url" "$litert_home"
(
  cd "$root"
  HOME="$litert_home" "$sidecar_bin" "${sidecar_args[@]}"
) >"$sidecar_log" 2>&1 &
sidecar_pid="$!"

wait_for_available_status() {
  local deadline=$((SECONDS + ready_timeout))
  local status_json

  while (( SECONDS < deadline )); do
    if ! kill -0 "$sidecar_pid" >/dev/null 2>&1; then
      printf 'Sidecar exited before becoming available.\n' >&2
      return 1
    fi

    if status_json="$(curl --silent --show-error --max-time 5 "$sidecar_url/sidecar/v1/status" 2>/dev/null)" &&
      STATUS_JSON="$status_json" node -e '
        const status = JSON.parse(process.env.STATUS_JSON);
        process.exit(status.state === "available" ? 0 : 1);
      '; then
      printf '%s\n' "$status_json"
      return 0
    fi

    sleep 1
  done

  printf 'Sidecar did not become available within %s seconds.\n' "$ready_timeout" >&2
  return 1
}

status_json="$(wait_for_available_status)"
printf 'Sidecar status:\n'
STATUS_JSON="$status_json" node -e '
  const status = JSON.parse(process.env.STATUS_JSON);
  console.log(JSON.stringify({
    state: status.state,
    runtime: status.runtime,
    backends: status.backends,
    multimodal: status.capabilities?.multimodal,
  }, null, 2));
'

models_json="$(curl --silent --show-error --max-time 20 "$sidecar_url/v1/models")"
MODELS_JSON="$models_json" MODEL_ID="$model_id" node -e '
  const models = JSON.parse(process.env.MODELS_JSON);
  const ids = (models.data || []).map((model) => model.id);
  if (!ids.includes(process.env.MODEL_ID)) {
    console.error(`Model ${process.env.MODEL_ID} was not advertised. IDs: ${ids.join(", ")}`);
    process.exit(1);
  }
  console.log(`Advertised models: ${ids.join(", ")}`);
'

payload="$(
  MODEL_ID="$model_id" node -e '
    console.log(JSON.stringify({
      model: process.env.MODEL_ID,
      messages: [{ role: "user", content: "Say OK." }],
      stream: false,
    }));
  '
)"
chat_json="$(
  curl --silent --show-error --max-time "$chat_timeout" \
    "$sidecar_url/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "$payload"
)"
CHAT_JSON="$chat_json" node -e '
  const response = JSON.parse(process.env.CHAT_JSON);
  const text = response.choices?.[0]?.message?.content;
  if (typeof text !== "string" || text.length === 0) {
    console.error("Chat response did not include assistant text.");
    process.exit(1);
  }
  console.log(`Assistant: ${text}`);
'

STATUS_JSON="$status_json" node -e '
  const status = JSON.parse(process.env.STATUS_JSON);
  if (status.capabilities?.multimodal?.state !== "available") {
    console.error("Sidecar did not advertise native multimodal capability.");
    process.exit(1);
  }
'

sample_image="$work_dir/sample.png"
SAMPLE_IMAGE="$sample_image" node -e '
  const fs = require("node:fs");
  const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X+XioAAAAASUVORK5CYII=";
  fs.writeFileSync(process.env.SAMPLE_IMAGE, Buffer.from(png, "base64"));
'
multimodal_payload="$(
  SAMPLE_IMAGE="$sample_image" node - <<'NODE'
    const fs = require("node:fs");
    console.log(JSON.stringify({
      prompt: "Describe this image in three words.",
      backend: "cpu",
      visionBackend: "cpu",
      attachments: [
        {
          name: "sample.png",
          mimeType: "image/png",
          dataBase64: fs.readFileSync(process.env.SAMPLE_IMAGE).toString("base64"),
        },
      ],
    }));
NODE
)"
multimodal_json="$(
  curl --silent --show-error --max-time "$multimodal_timeout" \
    "$sidecar_url/sidecar/v1/multimodal" \
    -H 'Content-Type: application/json' \
    -d "$multimodal_payload"
)"
MULTIMODAL_JSON="$multimodal_json" node -e '
  const response = JSON.parse(process.env.MULTIMODAL_JSON);
  const text = response.text;
  if (typeof text !== "string" || text.length === 0) {
    console.error("Multimodal response did not include text.");
    process.exit(1);
  }
  console.log(`Multimodal assistant: ${text}`);
'

printf 'Real LiteRT-LM sidecar smoke passed.\n'
