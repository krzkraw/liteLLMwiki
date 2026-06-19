#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
smoke_port="${INSTALL_SMOKE_PORT:-5177}"
smoke_url="http://127.0.0.1:${smoke_port}/"
dev_server_pid=""
models_nextcloud="${MODELS_NEXTCLOUD:-}"
models_nextcloud_base=""
models_nextcloud_token=""
summary=()

cd "$repo_root"

add_summary() {
  summary+=("$1")
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

run_logged() {
  local label="$1"
  shift

  printf '\n==> %s\n' "$label"
  "$@"
  add_summary "PASS: $label"
}

usage() {
  cat <<'USAGE'
Usage: ./install.sh [modelsNextcloud=<public-share-url>]

Options:
  modelsNextcloud    Optional Nextcloud public share URL for model downloads.
                     Example: modelsNextcloud=https://nextcloud.example/s/share-token

Environment:
  MODELS_NEXTCLOUD   Same as modelsNextcloud.
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      modelsNextcloud=*)
        models_nextcloud="${1#modelsNextcloud=}"
        ;;
      --modelsNextcloud=*)
        models_nextcloud="${1#--modelsNextcloud=}"
        ;;
      modelsNextcloud|--modelsNextcloud)
        shift
        if [[ $# -eq 0 ]]; then
          printf 'modelsNextcloud requires a public share URL.\n' >&2
          usage >&2
          exit 2
        fi
        models_nextcloud="$1"
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

configure_models_nextcloud() {
  local share="${models_nextcloud%/}"

  if [[ -z "$share" ]]; then
    return 0
  fi

  if [[ "$share" =~ ^(https?://[^/]+)/s/([^/?#]+) ]]; then
    models_nextcloud_base="${BASH_REMATCH[1]}"
    models_nextcloud_token="${BASH_REMATCH[2]}"
    return 0
  fi

  printf 'modelsNextcloud must be a Nextcloud public share URL like https://nextcloud.example/s/share-token\n' >&2
  exit 2
}

nextcloud_model_url() {
  local relative_path="$1"

  printf '%s/public.php/webdav/%s\n' "$models_nextcloud_base" "$relative_path"
}

package_install_command() {
  local package_name="$1"

  if has_command brew; then
    printf 'brew install %s\n' "$package_name"
    return 0
  fi

  if has_command apt-get; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      printf 'apt-get update && apt-get install -y %s\n' "$package_name"
    else
      printf 'sudo apt-get update && sudo apt-get install -y %s\n' "$package_name"
    fi
    return 0
  fi

  if has_command dnf; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      printf 'dnf install -y %s\n' "$package_name"
    else
      printf 'sudo dnf install -y %s\n' "$package_name"
    fi
    return 0
  fi

  if has_command pacman; then
    if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
      printf 'pacman -Sy --needed --noconfirm %s\n' "$package_name"
    else
      printf 'sudo pacman -Sy --needed --noconfirm %s\n' "$package_name"
    fi
    return 0
  fi

  return 1
}

dependency_install_command() {
  local dependency="$1"

  case "$dependency" in
    node)
      if has_command brew; then
        printf 'brew install node\n'
      elif has_command apt-get; then
        if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
          printf 'apt-get update && apt-get install -y nodejs npm\n'
        else
          printf 'sudo apt-get update && sudo apt-get install -y nodejs npm\n'
        fi
      elif has_command dnf; then
        if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
          printf 'dnf install -y nodejs npm\n'
        else
          printf 'sudo dnf install -y nodejs npm\n'
        fi
      elif has_command pacman; then
        if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
          printf 'pacman -Sy --needed --noconfirm nodejs npm\n'
        else
          printf 'sudo pacman -Sy --needed --noconfirm nodejs npm\n'
        fi
      else
        printf 'Install Node.js from https://nodejs.org/\n'
      fi
      ;;
    go)
      if has_command brew; then
        printf 'brew install go\n'
      elif has_command apt-get; then
        if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
          printf 'apt-get update && apt-get install -y golang-go\n'
        else
          printf 'sudo apt-get update && sudo apt-get install -y golang-go\n'
        fi
      elif has_command dnf; then
        if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
          printf 'dnf install -y golang\n'
        else
          printf 'sudo dnf install -y golang\n'
        fi
      elif has_command pacman; then
        if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
          printf 'pacman -Sy --needed --noconfirm go\n'
        else
          printf 'sudo pacman -Sy --needed --noconfirm go\n'
        fi
      else
        printf 'Install Go from https://go.dev/dl/\n'
      fi
      ;;
    uv)
      if has_command brew; then
        printf 'brew install uv\n'
      else
        printf 'curl -LsSf https://astral.sh/uv/install.sh | sh\n'
      fi
      ;;
    *)
      package_install_command "$dependency" || printf 'Install %s with your OS package manager.\n' "$dependency"
      ;;
  esac
}

manual_url_action() {
  local url="$1"

  if has_command open; then
    open "$url" >/dev/null 2>&1 || true
    return 0
  fi

  if has_command xdg-open; then
    xdg-open "$url" >/dev/null 2>&1 || true
    return 0
  fi

  return 1
}

wait_for_user_action() {
  local label="$1"
  local check_cmd="$2"

  printf 'I will wait. Press Enter after you have done it: %s\n' "$label"
  while true; do
    read -r _
    if eval "$check_cmd"; then
      return 0
    fi
    printf 'Still not detected. Press Enter after completing it, or Ctrl-C to stop.\n'
  done
}

confirm_or_wait() {
  local label="$1"
  local command_text="$2"
  local check_cmd="$3"
  local run_cmd="$4"
  local required="${5:-required}"
  local answer

  if eval "$check_cmd"; then
    add_summary "OK: $label"
    return 0
  fi

  printf '\n%s needs to be installed downloaded, here is the command or URL I would use:\n' "$label"
  printf '%s\n' "$command_text"

  while ! eval "$check_cmd"; do
    printf 'Do you want me to do it? [y/N] '
    read -r answer
    case "$answer" in
      y|Y|yes|YES)
        if [[ -n "$run_cmd" ]]; then
          if eval "$run_cmd"; then
            :
          else
            printf 'The task failed. Here is the command or URL again:\n%s\n' "$command_text"
            wait_for_user_action "$label" "$check_cmd"
          fi
        else
          printf 'No automatic command is available for this environment.\n'
          wait_for_user_action "$label" "$check_cmd"
        fi
        ;;
      *)
        wait_for_user_action "$label" "$check_cmd"
        ;;
    esac
  done

  add_summary "OK: $label"
  if [[ "$required" == "optional" ]]; then
    return 0
  fi
}

ensure_dependency() {
  local label="$1"
  local command_name="$2"
  local install_cmd="$3"
  local required="${4:-required}"

  confirm_or_wait "$label" "$install_cmd" \
    "command -v '$command_name' >/dev/null 2>&1" \
    "$install_cmd" "$required"
}

ensure_package_tool() {
  local label="$1"
  local command_name="$2"
  local package_name="$3"
  local install_cmd

  if has_command "$command_name"; then
    add_summary "OK: $label"
    return 0
  fi

  if ! install_cmd="$(package_install_command "$package_name")"; then
    install_cmd="Install $package_name with your OS package manager, then make $command_name available on PATH."
  fi

  ensure_dependency "$label" "$command_name" "$install_cmd"
}

ensure_optional_tool() {
  local label="$1"
  local command_name="$2"
  local command_or_url="$3"
  local run_cmd="${4:-}"

  if has_command "$command_name"; then
    add_summary "OK: $label"
    return 0
  fi

  confirm_or_wait "$label" "$command_or_url" \
    "command -v '$command_name' >/dev/null 2>&1" \
    "$run_cmd" "optional"
}

prompt_hf_token_if_needed() {
  local token_answer
  local token_value

  if [[ -n "${HF_TOKEN:-${HUGGING_FACE_HUB_TOKEN:-}}" ]]; then
    return 0
  fi

  printf 'This Hugging Face download may need a token. Paste one now? [y/N] '
  read -r token_answer
  case "$token_answer" in
    y|Y|yes|YES)
      printf 'HF token: '
      stty -echo
      read -r token_value
      stty echo
      printf '\n'
      export HF_TOKEN="$token_value"
      export HUGGING_FACE_HUB_TOKEN="$token_value"
      ;;
  esac
}

download_model() {
  local url="$1"
  local target="$2"
  local auth_kind="${3:-huggingface}"
  local auth_token="${4:-}"
  local partial="${target}.partial"
  local curl_args=(-L --fail --output "$partial")

  mkdir -p "$(dirname "$target")"
  rm -f "$partial"

  case "$auth_kind" in
    nextcloud)
      curl_args+=(-u "${auth_token}:")
      ;;
    huggingface|*)
      if [[ -n "${HF_TOKEN:-${HUGGING_FACE_HUB_TOKEN:-}}" ]]; then
        curl_args+=(-H "Authorization: Bearer ${HF_TOKEN:-$HUGGING_FACE_HUB_TOKEN}")
      fi
      ;;
  esac

  curl "${curl_args[@]}" "$url"
  mv "$partial" "$target"
}

ensure_model() {
  local label="$1"
  local relative_path="$2"
  local url="$3"
  local may_need_token="${4:-false}"
  local target="$repo_root/$relative_path"
  local download_url="$url"
  local auth_kind="huggingface"
  local auth_token=""
  local command_text

  if [[ -n "$models_nextcloud_base" ]]; then
    download_url="$(nextcloud_model_url "$relative_path")"
    auth_kind="nextcloud"
    auth_token="$models_nextcloud_token"
    may_need_token="false"
  fi

  if [[ "$auth_kind" == "nextcloud" ]]; then
    command_text="URL: $download_url
Path: $relative_path
Command: curl -L --fail -u '<modelsNextcloud-token>:' -o '$relative_path' '$download_url'
Header: Authorization: Basic <modelsNextcloud-token>"
  else
    command_text="URL: $download_url
Path: $relative_path
Command: curl -L --fail -o '$relative_path' '$download_url'"
  fi

  if [[ -s "$target" ]]; then
    add_summary "OK: $label at $relative_path"
    return 0
  fi

  printf '\n%s needs to be installed downloaded, here is the command or URL I would use:\n%s\n' "$label" "$command_text"

  while [[ ! -s "$target" ]]; do
    local answer
    printf 'Do you want me to do it? [y/N] '
    read -r answer
    case "$answer" in
      y|Y|yes|YES)
        if [[ "$may_need_token" == "true" ]]; then
          prompt_hf_token_if_needed
        fi
        if download_model "$download_url" "$target" "$auth_kind" "$auth_token"; then
          :
        else
          printf 'The task failed. Here is the command or URL again:\n%s\n' "$command_text"
          printf 'I will wait. Press Enter after you have put the file at %s\n' "$relative_path"
          read -r _
        fi
        ;;
      *)
        printf 'I will wait. Open the URL in a browser if needed and put the file at:\n%s\n' "$relative_path"
        printf 'Press Enter after the file is there.\n'
        read -r _
        ;;
    esac
  done

  add_summary "OK: $label at $relative_path"
}

ensure_npm_dependencies() {
  confirm_or_wait "npm dependencies" "npm install" \
    "test -d '$repo_root/node_modules' && test -d '$repo_root/public/vendor/litert-lm/core/wasm'" \
    "cd '$repo_root' && npm install"
}

wait_for_url() {
  local url="$1"
  local attempts=0

  until curl -fsS "$url" >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [[ "$attempts" -gt 80 ]]; then
      return 1
    fi
    sleep 0.25
  done
}

cleanup_dev_server() {
  if [[ -n "${dev_server_pid:-}" ]] && kill -0 "$dev_server_pid" 2>/dev/null; then
    kill "$dev_server_pid" 2>/dev/null || true
    wait "$dev_server_pid" 2>/dev/null || true
  fi
}

run_smoke_tests() {
  trap cleanup_dev_server EXIT

  printf '\n==> Starting temporary web UI for smoke tests at %s\n' "$smoke_url"
  npm run dev -- --host 127.0.0.1 --port "$smoke_port" --strictPort >/tmp/litert-wiki-install-vite.log 2>&1 &
  dev_server_pid="$!"

  if ! wait_for_url "$smoke_url"; then
    printf 'Temporary web UI did not become ready. Log:\n'
    cat /tmp/litert-wiki-install-vite.log
    return 1
  fi

  run_logged "smoke UI" env SMOKE_URL="$smoke_url" npm run smoke
  run_logged "smoke executable sidecar" env SMOKE_URL="$smoke_url" npm run smoke:executable

  if [[ -s "$repo_root/models/litert/gemma-4-E2B-it-web.litertlm" ]]; then
    run_logged "smoke web model" env SMOKE_URL="$smoke_url" npm run smoke:model
  else
    add_summary "SKIP: smoke web model, models/litert/gemma-4-E2B-it-web.litertlm missing"
  fi

  cleanup_dev_server
  dev_server_pid=""
  trap - EXIT
}

print_summary() {
  printf '\nSummary\n'
  printf '-------\n'
  for item in "${summary[@]}"; do
    printf '%s\n' "$item"
  done
  printf '\nNext command:\n'
  printf './launch-all.sh\n'
}

main() {
  local node_cmd git_cmd go_cmd curl_cmd uv_cmd llama_url

  node_cmd="$(dependency_install_command node)"
  git_cmd="$(dependency_install_command git)"
  go_cmd="$(dependency_install_command go)"
  curl_cmd="$(dependency_install_command curl)"
  uv_cmd="$(dependency_install_command uv)"
  llama_url="https://github.com/ggml-org/llama.cpp/releases"

  ensure_package_tool "git" "git" "git"
  ensure_dependency "node" "node" "${node_cmd:-Install Node.js from https://nodejs.org/}"
  ensure_dependency "npm" "npm" "${node_cmd:-Install Node.js from https://nodejs.org/}"
  ensure_dependency "go" "go" "${go_cmd:-Install Go from https://go.dev/dl/}"
  ensure_dependency "curl" "curl" "${curl_cmd:-Install curl with your OS package manager.}"
  ensure_dependency "uv" "uv" "${uv_cmd:-curl -LsSf https://astral.sh/uv/install.sh | sh}"
  ensure_optional_tool "litert-lm" "litert-lm" "uv tool install litert-lm" "uv tool install litert-lm"
  ensure_optional_tool "llama-server" "llama-server" "$llama_url" "manual_url_action '$llama_url'; false"

  ensure_npm_dependencies

  ensure_model "Gemma 4 E2B web model" \
    "models/litert/gemma-4-E2B-it-web.litertlm" \
    "https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm/resolve/main/gemma-4-E2B-it-web.litertlm" \
    "true"
  ensure_model "Gemma 4 E2B native LiteRT model" \
    "models/litert/gemma-4-E2B-it.litertlm" \
    "https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm/resolve/main/gemma-4-E2B-it.litertlm" \
    "true"
  ensure_model "Gemma 4 E2B llama.cpp GGUF model" \
    "models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" \
    "https://huggingface.co/unsloth/gemma-4-E2B-it-qat-GGUF/resolve/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" \
    "false"
  ensure_model "Qwen3 embedding GGUF model" \
    "models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf" \
    "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf" \
    "false"
  ensure_model "EmbeddingGemma LiteRT embedding model" \
    "models/litert/embeddinggemma-300M_seq2048_mixed-precision.tflite" \
    "https://huggingface.co/litert-community/embeddinggemma-300m/resolve/main/embeddinggemma-300M_seq2048_mixed-precision.tflite" \
    "true"

  run_logged "npm test" npm test
  run_logged "web production build" npm run build
  run_logged "sidecar artifacts build" npm run build:sidecar
  run_smoke_tests

  print_summary
}

parse_args "$@"
configure_models_nextcloud
main
