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
require_cmd bun

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

model_file="${MODEL_FILE:-"$repo_root/models/litert/main/gemma-4-E2B-it.litertlm"}"
model_id="${MODEL_ID:-gemma4-e2b}"

if [[ ! -f "$model_file" ]]; then
  printf 'Model file not found: %s\n' "$model_file" >&2
  exit 1
fi

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/g0litellama-smoke.XXXXXX")"
G0LiteLLaMa_log="$work_dir/G0LiteLLaMa.log"
G0LiteLLaMa_bin="$work_dir/g0litellama"
G0LiteLLaMa_pid=""

if [[ -n "${LITERT_HOME:-}" ]]; then
  mkdir -p "$LITERT_HOME"
  litert_home="$(cd "$LITERT_HOME" && pwd)"
else
  litert_home="$work_dir/litert-home"
  mkdir -p "$litert_home"
fi

cleanup() {
  local status=$?

  if [[ -n "$G0LiteLLaMa_pid" ]] && kill -0 "$G0LiteLLaMa_pid" >/dev/null 2>&1; then
    kill "$G0LiteLLaMa_pid" >/dev/null 2>&1 || true
    wait "$G0LiteLLaMa_pid" >/dev/null 2>&1 || true
  fi

  if [[ "$status" -ne 0 && -f "$G0LiteLLaMa_log" ]]; then
    printf '\nG0LiteLLaMa log:\n' >&2
    sed -n '1,220p' "$G0LiteLLaMa_log" >&2
  fi

  rm -rf "$work_dir"
}
trap cleanup EXIT

G0LiteLLaMa_host="${G0LITELLAMA_HOST:-127.0.0.1}"
G0LiteLLaMa_port="${G0LITELLAMA_PORT:-"$(pick_port 9479)"}"
runtime_host="${RUNTIME_HOST:-127.0.0.1}"
runtime_port="${RUNTIME_PORT:-"$(pick_port 9481)"}"
G0LiteLLaMa_addr="$G0LiteLLaMa_host:$G0LiteLLaMa_port"
G0LiteLLaMa_url="http://$G0LiteLLaMa_addr"
upstream_url="http://$runtime_host:$runtime_port"
ready_timeout="${READY_TIMEOUT_SECONDS:-180}"
chat_timeout="${CHAT_TIMEOUT_SECONDS:-240}"
multimodal_timeout="${MULTIMODAL_TIMEOUT_SECONDS:-360}"

printf 'Building G0LiteLLaMa...\n'
(
  cd "$root"
  go build -o "$G0LiteLLaMa_bin" ./cmd/g0litellama
)

G0LiteLLaMa_args=(
  --headless
  -addr "$G0LiteLLaMa_addr"
  -upstream "$upstream_url"
  -runtime-exe "$runtime_exe"
  -runtime-host "$runtime_host"
  -runtime-port "$runtime_port"
  -model-file "$model_file"
  -model-id "$model_id"
)

if [[ "${RUNTIME_VERBOSE:-0}" == "1" ]]; then
  G0LiteLLaMa_args+=(-runtime-verbose)
fi

printf 'Starting G0LiteLLaMa at %s with isolated HOME=%s...\n' "$G0LiteLLaMa_url" "$litert_home"
(
  cd "$root"
  HOME="$litert_home" "$G0LiteLLaMa_bin" "${G0LiteLLaMa_args[@]}"
) >"$G0LiteLLaMa_log" 2>&1 &
G0LiteLLaMa_pid="$!"

wait_for_available_status() {
  local deadline=$((SECONDS + ready_timeout))
  local status_json

  while (( SECONDS < deadline )); do
    if ! kill -0 "$G0LiteLLaMa_pid" >/dev/null 2>&1; then
      printf 'G0LiteLLaMa exited before becoming available.\n' >&2
      return 1
    fi

    if status_json="$(curl --silent --show-error --max-time 5 "$G0LiteLLaMa_url/g0litellama/v1/status" 2>/dev/null)" &&
      STATUS_JSON="$status_json" bun --eval '
        const status = JSON.parse(process.env.STATUS_JSON);
        process.exit(status.state === "available" ? 0 : 1);
      '; then
      printf '%s\n' "$status_json"
      return 0
    fi

    sleep 1
  done

  printf 'G0LiteLLaMa did not become available within %s seconds.\n' "$ready_timeout" >&2
  return 1
}

status_json="$(wait_for_available_status)"
printf 'G0LiteLLaMa status:\n'
STATUS_JSON="$status_json" bun --eval '
  const status = JSON.parse(process.env.STATUS_JSON);
  console.log(JSON.stringify({
    state: status.state,
    runtime: status.runtime,
    backends: status.backends,
    multimodal: status.capabilities?.multimodal,
  }, null, 2));
'

models_json="$(curl --silent --show-error --max-time 20 "$G0LiteLLaMa_url/v1/models")"
MODELS_JSON="$models_json" MODEL_ID="$model_id" bun --eval '
  const models = JSON.parse(process.env.MODELS_JSON);
  const ids = (models.data || []).map((model) => model.id);
  if (!ids.includes(process.env.MODEL_ID)) {
    console.error(`Model ${process.env.MODEL_ID} was not advertised. IDs: ${ids.join(", ")}`);
    process.exit(1);
  }
  console.log(`Advertised models: ${ids.join(", ")}`);
'

payload="$(
  MODEL_ID="$model_id" bun --eval '
    console.log(JSON.stringify({
      model: process.env.MODEL_ID,
      messages: [{ role: "user", content: "Say OK." }],
      stream: false,
    }));
  '
)"
chat_json="$(
  curl --silent --show-error --max-time "$chat_timeout" \
    "$G0LiteLLaMa_url/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "$payload"
)"
CHAT_JSON="$chat_json" bun --eval '
  const response = JSON.parse(process.env.CHAT_JSON);
  const text = response.choices?.[0]?.message?.content;
  if (typeof text !== "string" || text.length === 0) {
    console.error("Chat response did not include assistant text.");
    process.exit(1);
  }
  console.log(`Assistant: ${text}`);
'

STATUS_JSON="$status_json" bun --eval '
  const status = JSON.parse(process.env.STATUS_JSON);
  if (status.capabilities?.multimodal?.state !== "available") {
    console.error("G0LiteLLaMa did not advertise native multimodal capability.");
    process.exit(1);
  }
'

sample_image="$work_dir/sample.png"
SAMPLE_IMAGE="$sample_image" bun --eval '
  const fs = require("fs");
  const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X+XioAAAAASUVORK5CYII=";
  fs.writeFileSync(process.env.SAMPLE_IMAGE, Buffer.from(png, "base64"));
'
multimodal_payload="$(
  SAMPLE_IMAGE="$sample_image" bun --eval "$(cat <<'BUN'
const fs = require("fs");
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
BUN
)"
)"
multimodal_json="$(
  curl --silent --show-error --max-time "$multimodal_timeout" \
    "$G0LiteLLaMa_url/g0litellama/v1/multimodal" \
    -H 'Content-Type: application/json' \
    -d "$multimodal_payload"
)"
MULTIMODAL_JSON="$multimodal_json" bun --eval '
  const response = JSON.parse(process.env.MULTIMODAL_JSON);
  const text = response.text;
  if (typeof text !== "string" || text.length === 0) {
    console.error("Multimodal response did not include text.");
    process.exit(1);
  }
  console.log(`Multimodal assistant: ${text}`);
'

printf 'Real LiteRT-LM G0LiteLLaMa smoke passed.\n'
