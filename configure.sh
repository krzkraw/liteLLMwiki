#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config_path="${RUNTIME_BACKEND_CONFIG:-$repo_root/G0LiteLLaMa/runtime-config/backends.json}"
litert_test_model="${LITERT_TEST_MODEL:-}"
llama_test_model="${LLAMA_TEST_MODEL:-}"
litert_lm_bin="${LITERT_LM_BIN:-}"
llama_server_bin="${LLAMA_SERVER_BIN:-}"
litert_model_id="${LITERT_TEST_MODEL_ID:-gemma4-e2b}"
litert_prompt="${LITERT_TEST_PROMPT:-Say ok.}"
llama_prompt="${LLAMA_TEST_PROMPT:-Say ok.}"
litert_runtime_root="$repo_root/G0LiteLLaMa/litert-runtimes"
llama_runtime_root="$repo_root/G0LiteLLaMa/llama-runtimes"

cd "$repo_root"

usage() {
  cat <<'USAGE'
Usage: ./configure.sh [--config path] [--litert-model path] [--llama-model path] [--litert-bin path] [--llama-bin path]

Environment overrides:
  RUNTIME_BACKEND_CONFIG  JSON file to update. Defaults to G0LiteLLaMa/runtime-config/backends.json.
  LITERT_TEST_MODEL       LiteRT model used by backend probes.
  LLAMA_TEST_MODEL        llama.cpp GGUF model used by backend probes.
  LITERT_LM_BIN           litert-lm executable override.
  LLAMA_SERVER_BIN        llama-server executable override.
  LLAMA_TEST_MAX_TOKENS   Completion token cap for llama.cpp probes. Defaults to 128.
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        shift
        config_path="${1:-}"
        ;;
      --litert-model)
        shift
        litert_test_model="${1:-}"
        ;;
      --llama-model)
        shift
        llama_test_model="${1:-}"
        ;;
      --litert-bin)
        shift
        litert_lm_bin="${1:-}"
        ;;
      --llama-bin)
        shift
        llama_server_bin="${1:-}"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        printf 'Unknown argument: %s\n' "$1" >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

first_existing_or_default() {
  local fallback="$1"
  shift
  local candidate

  for candidate in "$@"; do
    if [[ -s "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  printf '%s\n' "$fallback"
}

shell_quote() {
  local value="$1"
  printf "'%s'" "$(printf '%s' "$value" | sed "s/'/'\\\\''/g")"
}

print_shell_command() {
  local first=1
  local arg

  for arg in "$@"; do
    if [[ "$first" -eq 0 ]]; then
      printf ' '
    fi
    shell_quote "$arg"
    first=0
  done
  printf '\n'
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

detect_litert_error_output() {
  local output="$1"

  printf '%s\n' "$output" | grep -Eiq \
    '(^An error occurred$|Traceback \(most recent call last\):|RuntimeError:|INVALID_ARGUMENT:|INTERNAL:|Failed to invoke|Failed to allocate|Validation error:)'
}

validate_litert_response() {
  local output="$1"

  if detect_litert_error_output "$output"; then
    return 1
  fi
  printf '%s\n' "$output" | grep -Eq '[[:alnum:]]'
}

validate_llama_response() {
  local response="$1"

  if has_command bun; then
    RESPONSE_TEXT="$response" bun --eval '
const text = process.env.RESPONSE_TEXT || "";
try {
  const data = JSON.parse(text);
  const content = data?.choices?.[0]?.message?.content;
  process.exit(typeof content === "string" && content.trim().length > 0 ? 0 : 1);
} catch {
  process.exit(1);
}
'
    return $?
  fi

  if has_command python3; then
    RESPONSE_TEXT="$response" python3 - <<'PY'
import json
import os
import sys

try:
    data = json.loads(os.environ.get("RESPONSE_TEXT", ""))
    content = data.get("choices", [{}])[0].get("message", {}).get("content", "")
except Exception:
    content = ""
sys.exit(0 if isinstance(content, str) and content.strip() else 1)
PY
    return $?
  fi

  printf '%s\n' "$response" | grep -Eq '"content"[[:space:]]*:[[:space:]]*"[^"]*[[:alnum:]][^"]*"'
}

find_litert_lm_in_dir() {
  local dir="$1"
  find "$dir" -type f \( -name 'litert-lm' -o -name 'litert-lm.exe' \) -print -quit 2>/dev/null
}

find_litert_lm() {
  local selected
  local selected_dir

  if [[ -n "$litert_lm_bin" ]]; then
    printf '%s\n' "$litert_lm_bin"
    return 0
  fi

  if [[ -f "$litert_runtime_root/.selected" ]]; then
    selected="$(tr -d '\r\n' < "$litert_runtime_root/.selected")"
    selected_dir="$litert_runtime_root/$selected"
    if [[ -d "$selected_dir" ]]; then
      if found="$(find_litert_lm_in_dir "$selected_dir")" && [[ -n "$found" ]]; then
        printf '%s\n' "$found"
        return 0
      fi
    fi
  fi

  if [[ -d "$litert_runtime_root" ]]; then
    if found="$(find_litert_lm_in_dir "$litert_runtime_root")" && [[ -n "$found" ]]; then
      printf '%s\n' "$found"
      return 0
    fi
  fi

  if has_command litert-lm; then
    command -v litert-lm
    return 0
  fi

  printf 'litert-lm\n'
}

find_llama_server_in_dir() {
  local dir="$1"
  find "$dir" -type f \( -name 'llama-server' -o -name 'llama-server.exe' \) -print -quit 2>/dev/null
}

llama_runtime_type() {
  local lower
  lower="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$lower" in
    *cuda-13*|*cuda13*) printf 'cuda13\n' ;;
    *cuda-12*|*cuda12*) printf 'cuda12\n' ;;
    *macos*) printf 'metal\n' ;;
    *openvino*) printf 'openvino\n' ;;
    *sycl*) printf 'sycl\n' ;;
    *vulkan*|*hip*|*radeon*|*opencl*) printf 'gpu\n' ;;
    *) printf 'cpu\n' ;;
  esac
}

llama_uses_gpu_backend() {
  case "$1" in
    gpu|cuda|cuda12|cuda13|metal|vulkan|openvino|sycl|npu) return 0 ;;
    *) return 1 ;;
  esac
}

discover_llama_specs() {
  local seen=""
  local dir name backend exe

  if [[ ! -d "$llama_runtime_root" ]]; then
    return 0
  fi

  for dir in "$llama_runtime_root"/*; do
    [[ -d "$dir" ]] || continue
    name="$(basename "$dir")"
    [[ "$name" != .* ]] || continue
    exe="$(find_llama_server_in_dir "$dir" || true)"
    [[ -n "$exe" ]] || continue
    backend="$(llama_runtime_type "$name")"
    case " $seen " in
      *" $backend "*) continue ;;
    esac
    seen="$seen $backend"
    printf '%s|%s|%s\n' "$backend" "$exe" "$name"
  done
}

run_litert_probe() {
  local executable="$1"
  local model_path="$2"
  local model_id="$3"
  local backend="$4"
  local prompt="${5:-$litert_prompt}"
  local max_tokens="${LITERT_TEST_MAX_NUM_TOKENS:-4096}"
  local run_output run_status

  printf 'Runtime command: '
  print_shell_command "$executable" run "$model_id" "--backend=$backend" "--max-num-tokens=$max_tokens" "--prompt=$prompt"

  if "$executable" list | awk '{print $1}' | grep -qx "$model_id"; then
    :
  else
    if ! "$executable" import "$model_path" "$model_id"; then
      return 1
    fi
  fi
  if run_output="$("$executable" run "$model_id" "--backend=$backend" "--max-num-tokens=$max_tokens" "--prompt=$prompt" 2>&1)"; then
    run_status=0
  else
    run_status=$?
  fi
  printf '%s\n' "$run_output"
  if [[ "$run_status" -ne 0 ]]; then
    return "$run_status"
  fi
  validate_litert_response "$run_output"
}

run_llama_probe() {
  local executable="$1"
  local model_path="$2"
  local backend="$3"
  local port log_file response_file pid args=()
  local prompt="${4:-$llama_prompt}"
  local max_tokens="${LLAMA_TEST_MAX_TOKENS:-128}"
  local timeout_seconds="${LLAMA_TEST_TIMEOUT_SECONDS:-180}"
  local request_timeout_seconds="${LLAMA_TEST_REQUEST_TIMEOUT_SECONDS:-120}"
  local deadline http_code payload response last_response

  port="${LLAMA_TEST_PORT:-$((28000 + RANDOM % 10000))}"
  log_file="$(mktemp "${TMPDIR:-/tmp}/llama-backend-${backend}.XXXXXX.log")"
  response_file="$(mktemp "${TMPDIR:-/tmp}/llama-backend-${backend}.XXXXXX.response.json")"
  args=("$executable" -m "$model_path" --alias configure-test --host 127.0.0.1 --port "$port" --reasoning off)
  if llama_uses_gpu_backend "$backend"; then
    args+=(--n-gpu-layers 999)
  fi

  printf 'Runtime command: '
  print_shell_command "${args[@]}"

  "${args[@]}" >"$log_file" 2>&1 &
  pid=$!
  deadline=$(($(date +%s) + timeout_seconds))
  payload='{"model":"configure-test","messages":[{"role":"user","content":"'"$(json_escape "$prompt")"'"}],"max_tokens":'"$max_tokens"',"temperature":0,"stream":false}'
  last_response=""

  while [[ "$(date +%s)" -lt "$deadline" ]]; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      cat "$log_file"
      rm -f "$log_file" "$response_file"
      return 1
    fi

    http_code="$(curl -sS --max-time "$request_timeout_seconds" \
      -o "$response_file" \
      -w '%{http_code}' \
      -H 'Content-Type: application/json' \
      -d "$payload" \
      "http://127.0.0.1:$port/v1/chat/completions" 2>>"$log_file" || true)"
    response="$(cat "$response_file" 2>/dev/null || true)"
    if [[ "$http_code" == "200" ]] && validate_llama_response "$response"; then
      printf 'Completion response: %s\n' "$response"
      kill "$pid" >/dev/null 2>&1 || true
      wait "$pid" >/dev/null 2>&1 || true
      rm -f "$log_file" "$response_file"
      return 0
    fi
    if [[ -n "$response" ]]; then
      last_response="HTTP $http_code: $response"
    elif [[ -n "$http_code" ]]; then
      last_response="HTTP $http_code"
    fi
    sleep 1
  done

  cat "$log_file"
  if [[ -n "$last_response" ]]; then
    printf 'Last completion response: %s\n' "$last_response"
  fi
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" >/dev/null 2>&1 || true
  rm -f "$log_file" "$response_file"
  return 1
}

update_backend_result_with_js() {
  CONFIG_PATH="$config_path" \
  RUNTIME_NAME="$1" \
  BACKEND_NAME="$2" \
  WORKING="$3" \
  COMMAND_TEXT="$4" \
  MODEL_PATH="$5" \
  OUTPUT_TEXT="$6" \
    bun --eval '
const fs = require("fs");
const path = require("path");
const configPath = process.env.CONFIG_PATH;
let config = { version: 1, runtimes: {} };
if (fs.existsSync(configPath)) {
  const raw = fs.readFileSync(configPath, "utf8").trim();
  if (raw) config = JSON.parse(raw);
}
if (!config || typeof config !== "object") config = { version: 1, runtimes: {} };
if (!config.runtimes || typeof config.runtimes !== "object") config.runtimes = {};
const runtime = process.env.RUNTIME_NAME;
const backend = process.env.BACKEND_NAME;
const updatedAt = new Date().toISOString();
config.version = 1;
config.updatedAt = updatedAt;
if (!config.runtimes[runtime] || typeof config.runtimes[runtime] !== "object") {
  config.runtimes[runtime] = {};
}
config.runtimes[runtime][backend] = {
  working: process.env.WORKING === "true",
  command: process.env.COMMAND_TEXT || "",
  model: process.env.MODEL_PATH || "",
  testedAt: updatedAt,
  output: process.env.OUTPUT_TEXT || ""
};
fs.mkdirSync(path.dirname(configPath), { recursive: true });
fs.writeFileSync(configPath, JSON.stringify(config, null, 2) + "\n");
'
}

update_backend_result_with_python() {
  CONFIG_PATH="$config_path" \
  RUNTIME_NAME="$1" \
  BACKEND_NAME="$2" \
  WORKING="$3" \
  COMMAND_TEXT="$4" \
  MODEL_PATH="$5" \
  OUTPUT_TEXT="$6" \
    python3 - <<'PY'
import datetime
import json
import os
from pathlib import Path

config_path = Path(os.environ["CONFIG_PATH"])
config = {"version": 1, "runtimes": {}}
if config_path.exists():
    raw = config_path.read_text(encoding="utf-8").strip()
    if raw:
        config = json.loads(raw)
if not isinstance(config, dict):
    config = {"version": 1, "runtimes": {}}
if not isinstance(config.get("runtimes"), dict):
    config["runtimes"] = {}
runtime = os.environ["RUNTIME_NAME"]
backend = os.environ["BACKEND_NAME"]
updated_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
config["version"] = 1
config["updatedAt"] = updated_at
config["runtimes"].setdefault(runtime, {})
config["runtimes"][runtime][backend] = {
    "working": os.environ["WORKING"] == "true",
    "command": os.environ.get("COMMAND_TEXT", ""),
    "model": os.environ.get("MODEL_PATH", ""),
    "testedAt": updated_at,
    "output": os.environ.get("OUTPUT_TEXT", ""),
}
config_path.parent.mkdir(parents=True, exist_ok=True)
config_path.write_text(json.dumps(config, indent=2) + "\n", encoding="utf-8")
PY
}

update_backend_result() {
  local runtime_name="$1"
  local backend="$2"
  local working="$3"
  local command_text="$4"
  local model_path="$5"
  local output_text="$6"

  if has_command bun; then
    update_backend_result_with_js "$runtime_name" "$backend" "$working" "$command_text" "$model_path" "$output_text"
    return 0
  fi
  if has_command python3; then
    update_backend_result_with_python "$runtime_name" "$backend" "$working" "$command_text" "$model_path" "$output_text"
    return 0
  fi

  printf 'Cannot update %s because neither bun nor python3 is available.\n' "$config_path" >&2
  return 1
}

prompted_command=""

prompt_command() {
  local default_command="$1"
  local override

  printf '%s\n' "$default_command"
  printf 'Edit command and press Enter, or press Enter to run default: '
  IFS= read -r override
  if [[ -n "$override" ]]; then
    prompted_command="$override"
    return 0
  fi
  prompted_command="$default_command"
}

run_backend_test() {
  local display_runtime="$1"
  local config_runtime="$2"
  local backend="$3"
  local model_path="$4"
  local default_command="$5"
  local command output_file exit_code recorded_output choice

  printf '\nwe are testing %s on %s backend, here is command I am going to use\n' "$display_runtime" "$backend"
  prompt_command "$default_command"
  command="$prompted_command"

  while true; do
    output_file="$(mktemp "${TMPDIR:-/tmp}/backend-test-${config_runtime}-${backend}.XXXXXX.log")"
    set +e
    eval "$command" >"$output_file" 2>&1
    exit_code=$?
    set -e

    if [[ "$exit_code" -eq 0 ]]; then
      printf 'Backend %s/%s worked.\n' "$config_runtime" "$backend"
      recorded_output="$(tail -c 4000 "$output_file" 2>/dev/null || true)"
      rm -f "$output_file"
      update_backend_result "$config_runtime" "$backend" "true" "$command" "$model_path" "$recorded_output"
      return 0
    fi

    printf 'Backend %s/%s failed with exit code %d.\n' "$config_runtime" "$backend" "$exit_code"
    printf 'Command output:\n'
    cat "$output_file"
    recorded_output="$(tail -c 4000 "$output_file" 2>/dev/null || true)"
    rm -f "$output_file"

    while true; do
      printf 'Choose [R] retry with edited command, [N] mark runtime backend combo as not working: '
      IFS= read -r choice
      case "$choice" in
        r|R|retry|RETRY)
          printf 'Current command:\n%s\n' "$command"
          prompt_command "$command"
          command="$prompted_command"
          break
          ;;
        n|N|no|NO|mark|MARK)
          update_backend_result "$config_runtime" "$backend" "false" "$command" "$model_path" "$recorded_output"
          printf 'Backend %s/%s marked not working.\n' "$config_runtime" "$backend"
          return 0
          ;;
        *)
          printf 'Choose R or N.\n'
          ;;
      esac
    done
  done
}

main() {
  local litert_bin llama_bin
  local litert_command llama_command
  local backend spec executable runtime_name
  local llama_specs=()
  local known_backend

  litert_test_model="${litert_test_model:-$(first_existing_or_default \
    "$repo_root/models/litert/main/gemma-4-E2B-it.litertlm" \
    "$repo_root/models/litert/main/gemma-4-E2B-it.litertlm")}"
  llama_test_model="${llama_test_model:-$(first_existing_or_default \
    "$repo_root/models/llamacpp/main/Qwen3.5-0.8B-UD-Q8_K_XL.gguf" \
    "$repo_root/models/llamacpp/main/Qwen3.5-0.8B-UD-Q8_K_XL.gguf" \
    "$repo_root/models/llamacpp/main/Qwen3.5-2B-IQ4_NL.gguf" \
    "$repo_root/models/llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf")}"

  litert_bin="$(find_litert_lm)"
  llama_bin="${llama_server_bin:-}"
  if [[ -z "$llama_bin" ]]; then
    if has_command llama-server; then
      llama_bin="$(command -v llama-server)"
    else
      llama_bin="llama-server"
    fi
  fi

  printf 'Backend test results will be written to %s\n' "$config_path"
  printf 'LiteRT test model: %s\n' "$litert_test_model"
  printf 'llama.cpp test model: %s\n' "$llama_test_model"

  for backend in cpu gpu npu; do
    litert_command="run_litert_probe $(shell_quote "$litert_bin") $(shell_quote "$litert_test_model") $(shell_quote "$litert_model_id") $(shell_quote "$backend")"
    run_backend_test "liteRT" "litert" "$backend" "$litert_test_model" "$litert_command"
  done

  while IFS= read -r spec; do
    [[ -n "$spec" ]] && llama_specs+=("$spec")
  done < <(discover_llama_specs)

  if [[ "${#llama_specs[@]}" -eq 0 ]]; then
    for known_backend in cpu gpu metal openvino cuda13 cuda12 sycl; do
      llama_specs+=("$known_backend|$llama_bin|candidate")
    done
  fi

  for spec in "${llama_specs[@]}"; do
    IFS='|' read -r backend executable runtime_name <<< "$spec"
    llama_command="run_llama_probe $(shell_quote "$executable") $(shell_quote "$llama_test_model") $(shell_quote "$backend")"
    run_backend_test "llama" "llamacpp" "$backend" "$llama_test_model" "$llama_command"
  done

  printf '\nConfiguration complete. Results are in %s\n' "$config_path"
}

parse_args "$@"
main
