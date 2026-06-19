#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
smoke_port="${INSTALL_SMOKE_PORT:-5177}"
smoke_url="http://127.0.0.1:${smoke_port}/"
dev_server_pid=""
models_nextcloud="${MODELS_NEXTCLOUD:-}"
models_nextcloud_base=""
models_nextcloud_token=""
llama_runtime_root="$repo_root/native/llama-runtimes"
llama_selected_file="$llama_runtime_root/.selected"
llama_release_base="https://github.com/ggml-org/llama.cpp/releases/download/b9724"
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

llama_executable_name() {
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) printf 'llama-server.exe\n' ;;
    *) printf 'llama-server\n' ;;
  esac
}

llama_runtime_spec() {
  local key="$1"

  case "$key" in
    macos-arm64)
      printf 'llama-macos-arm64|macOS Apple Silicon|%s/llama-b9724-bin-macos-arm64.tar.gz|sha256:b4582c69bc58e6b84d16105011d9431eeec9a0d1745d9ca8e48472a285db6b7f||\n' "$llama_release_base"
      ;;
    macos-x64)
      printf 'llama-macos-x64|macOS Intel|%s/llama-b9724-bin-macos-x64.tar.gz|sha256:4fd4228bd23dbc6ae53805a89b1811861c1b9da5d2ff07bfd9a08fb5f0c87f6e||\n' "$llama_release_base"
      ;;
    win-cpu-x64)
      printf 'llama-win-cpu-x64|Windows x64 CPU|%s/llama-b9724-bin-win-cpu-x64.zip|sha256:e06bafb4e1aaf3745be816d5d072cd965e52ef49ef8e9e93f031e196703780bf||\n' "$llama_release_base"
      ;;
    win-cpu-arm64)
      printf 'llama-win-cpu-arm64|Windows arm64 CPU|%s/llama-b9724-bin-win-cpu-arm64.zip|sha256:092191286aa8c1d11e909308358e6ac9bd7b5dc83e01d71d96807f6b0cf948bf||\n' "$llama_release_base"
      ;;
    win-cuda12-x64)
      printf 'llama-win-cuda-12.4-x64|Windows x64 CUDA 12.4|%s/llama-b9724-bin-win-cuda-12.4-x64.zip|sha256:913d47f80a3cad43fe95eda2ed0cf0dbd5fe01d758f66c097fa0a6138021729d|%s/cudart-llama-bin-win-cuda-12.4-x64.zip|sha256:8c79a9b226de4b3cacfd1f83d24f962d0773be79f1e7b75c6af4ded7e32ae1d6\n' "$llama_release_base" "$llama_release_base"
      ;;
    win-cuda13-x64)
      printf 'llama-win-cuda-13.3-x64|Windows x64 CUDA 13.3|%s/llama-b9724-bin-win-cuda-13.3-x64.zip|sha256:c16700717a20daebc12a2de2bf1ac711ba43f9565dac9d6fbcdf04099dde975a|%s/cudart-llama-bin-win-cuda-13.3-x64.zip|sha256:1462a050eb4c684921ba51dcc4cc488a036674c3e73e9945ee705b854808d03e\n' "$llama_release_base" "$llama_release_base"
      ;;
    win-vulkan-x64)
      printf 'llama-win-vulkan-x64|Windows x64 Vulkan|%s/llama-b9724-bin-win-vulkan-x64.zip|sha256:3e245e75f38477f9c99858cf149a3831988701090d156512eb143f2312b76b44||\n' "$llama_release_base"
      ;;
    win-openvino-x64)
      printf 'llama-win-openvino-2026.2-x64|Windows x64 OpenVINO|%s/llama-b9724-bin-win-openvino-2026.2-x64.zip|sha256:da36f6380bbeffddd4db58bfbc09077982c465d92123e943e6af679e8ed5d0ec||\n' "$llama_release_base"
      ;;
    win-sycl-x64)
      printf 'llama-win-sycl-x64|Windows x64 SYCL|%s/llama-b9724-bin-win-sycl-x64.zip|sha256:f660e83887af4a1c62742010a8064ab26aa9befacecaa5c86c6061ae68a3c04f||\n' "$llama_release_base"
      ;;
    win-hip-x64)
      printf 'llama-win-hip-radeon-x64|Windows x64 HIP Radeon|%s/llama-b9724-bin-win-hip-radeon-x64.zip|sha256:2b861729d7b1620a7ee09ebc8681f2534be9da307f93fd68afb6410f160a016b||\n' "$llama_release_base"
      ;;
    win-opencl-adreno-arm64)
      printf 'llama-win-opencl-adreno-arm64|Windows arm64 OpenCL Adreno|%s/llama-b9724-bin-win-opencl-adreno-arm64.zip|sha256:3e465918a49382fd466003e2d1658b261e87c68b8aa77c087a441ef3b7dee62c||\n' "$llama_release_base"
      ;;
    *)
      return 1
      ;;
  esac
}

llama_available_options() {
  local os_name
  local arch_name

  os_name="$(uname -s)"
  arch_name="$(uname -m)"

  case "$os_name:$arch_name" in
    Darwin:arm64|Darwin:aarch64) printf 'macos-arm64\n' ;;
    Darwin:x86_64|Darwin:amd64) printf 'macos-x64\n' ;;
    MINGW*:x86_64|MSYS*:x86_64|CYGWIN*:x86_64)
      printf 'win-cpu-x64\nwin-cuda13-x64\nwin-cuda12-x64\nwin-vulkan-x64\nwin-openvino-x64\nwin-sycl-x64\nwin-hip-x64\n'
      ;;
    MINGW*:aarch64|MSYS*:aarch64|CYGWIN*:aarch64|MINGW*:arm64|MSYS*:arm64|CYGWIN*:arm64)
      printf 'win-cpu-arm64\nwin-opencl-adreno-arm64\n'
      ;;
    *) return 1 ;;
  esac
}

verify_sha256() {
  local file="$1"
  local expected="${2#sha256:}"
  local actual

  actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  if [[ "$actual" != "$expected" ]]; then
    printf 'SHA256 mismatch for %s\nexpected %s\nactual   %s\n' "$file" "$expected" "$actual" >&2
    return 1
  fi
}

extract_llama_archive() {
  local archive="$1"
  local target_dir="$2"
  local archive_name="${3:-$archive}"

  case "$archive_name" in
    *.zip) unzip -q -o "$archive" -d "$target_dir" ;;
    *.tar.gz) tar -xzf "$archive" -C "$target_dir" ;;
    *) printf 'Unsupported llama.cpp archive: %s\n' "$archive" >&2; return 1 ;;
  esac
}

install_llama_asset() {
  local url="$1"
  local sha256="$2"
  local target_dir="$3"
  local archive_name="${url##*/}"
  local archive_path

  archive_path="$(mktemp "${TMPDIR:-/tmp}/${archive_name}.XXXXXX")"
  curl -L --fail --output "$archive_path" "$url"
  verify_sha256 "$archive_path" "$sha256"
  extract_llama_archive "$archive_path" "$target_dir" "$archive_name"
  rm -f "$archive_path"
}

find_llama_server_in_dir() {
  local dir="$1"
  local executable_name

  executable_name="$(llama_executable_name)"
  find "$dir" -type f -name "$executable_name" -print -quit
}

installed_llama_server() {
  local runtime_name
  local selected_dir

  if has_command llama-server; then
    command -v llama-server
    return 0
  fi

  if [[ -f "$llama_selected_file" ]]; then
    runtime_name="$(tr -d '\r\n' < "$llama_selected_file")"
    selected_dir="$llama_runtime_root/$runtime_name"
    if [[ -d "$selected_dir" ]]; then
      find_llama_server_in_dir "$selected_dir"
      return 0
    fi
  fi

  if [[ -d "$llama_runtime_root" ]]; then
    find_llama_server_in_dir "$llama_runtime_root"
  fi
}

install_llama_runtime() {
  local key="$1"
  local spec folder label url sha256 extra_url extra_sha256
  local target_dir tmp_dir

  spec="$(llama_runtime_spec "$key")"
  IFS='|' read -r folder label url sha256 extra_url extra_sha256 <<< "$spec"
  target_dir="$llama_runtime_root/$folder"
  tmp_dir="${target_dir}.tmp"

  printf '\nllama.cpp runtime needs to be installed downloaded, here is the command or URL I would use:\n'
  printf 'Runtime: %s\nFolder: native/llama-runtimes/%s\nURL: %s\nsha256: %s\n' "$label" "$folder" "$url" "${sha256#sha256:}"
  if [[ -n "$extra_url" ]]; then
    printf 'CUDA DLL URL: %s\nsha256: %s\n' "$extra_url" "${extra_sha256#sha256:}"
  fi

  mkdir -p "$llama_runtime_root"
  rm -rf "$tmp_dir"
  mkdir -p "$tmp_dir"
  install_llama_asset "$url" "$sha256" "$tmp_dir"
  if [[ -n "$extra_url" ]]; then
    install_llama_asset "$extra_url" "$extra_sha256" "$tmp_dir"
  fi
  find "$tmp_dir" -type f -name 'llama-server*' -exec chmod +x {} + 2>/dev/null || true
  rm -rf "$target_dir"
  mv "$tmp_dir" "$target_dir"
  printf '%s\n' "$folder" > "$llama_selected_file"
  add_summary "OK: llama.cpp runtime $folder"
}

ensure_llama_runtime() {
  local installed
  local options=()
  local option
  local spec folder label url sha256 extra_url extra_sha256
  local choice

  installed="$(installed_llama_server || true)"
  if [[ -n "$installed" ]]; then
    add_summary "OK: llama-server at $installed"
    return 0
  fi

  while IFS= read -r option; do
    options+=("$option")
  done < <(llama_available_options || true)

  if [[ "${#options[@]}" -eq 0 ]]; then
    ensure_optional_tool "llama-server" "llama-server" \
      "https://github.com/ggml-org/llama.cpp/releases" \
      "manual_url_action 'https://github.com/ggml-org/llama.cpp/releases'; false"
    return 0
  fi

  printf '\nllama.cpp runtime needs to be installed downloaded. Choose one option, or all:\n'
  for option in "${options[@]}"; do
    spec="$(llama_runtime_spec "$option")"
    IFS='|' read -r folder label url sha256 extra_url extra_sha256 <<< "$spec"
    printf '  %s: %s -> native/llama-runtimes/%s\n' "$option" "$label" "$folder"
    printf '      %s\n' "$url"
    if [[ -n "$extra_url" ]]; then
      printf '      CUDA DLLs: %s\n' "$extra_url"
    fi
  done
  printf '  all: install every option listed above\n'
  printf '  skip: I will install llama-server myself and press Enter\n'

  while true; do
    printf 'llama.cpp runtime choice [all/%s/skip]: ' "${options[0]}"
    read -r choice
    choice="${choice:-${options[0]}}"
    case "$choice" in
      all)
        for option in "${options[@]}"; do
          install_llama_runtime "$option"
        done
        return 0
        ;;
      skip)
        wait_for_user_action "llama-server" "command -v llama-server >/dev/null 2>&1 || test -n \"\$(installed_llama_server || true)\""
        add_summary "OK: llama-server"
        return 0
        ;;
      *)
        for option in "${options[@]}"; do
          if [[ "$choice" == "$option" ]]; then
            install_llama_runtime "$option"
            return 0
          fi
        done
        printf 'Unknown llama.cpp runtime choice: %s\n' "$choice"
        ;;
    esac
  done
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
  ensure_llama_runtime

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
