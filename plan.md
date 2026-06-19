# Sidecar TUI Multi-Runner Plan

## Summary

Build the Go sidecar into a Bubble Tea TUI that defaults to an interactive
dashboard, with `--headless` preserving current browser and CI behavior. The
implementation should replace the current single LiteRT manager with a
first-class multi-runner supervisor: it manages long-lived LiteRT-LM and
llama.cpp processes, preserves the existing local HTTP/WebSocket API at the
server boundary, and adds model downloads, backend detection, runner creation,
logs, and a TUI chat tab.

This is doable on macOS, Linux, and Windows Terminal, but the right v1 is a
sidecar supervisor/TUI rather than an additive wrapper around the current single
`litert-lm serve` process. The selected implementation direction is Approach B:
replace the internal runtime authority with the supervisor while preserving
compatibility at the HTTP/WebSocket contract boundary.

Authoritative design spec:

```text
docs/superpowers/specs/2026-06-19-sidecar-tui-multi-runner-design.md
```

## Key Changes

- Add `native/sidecar` packages for runner supervision, model catalog/downloads,
  system/backend probing, and Bubble Tea UI.
- Keep `/v1/*`, `/sidecar/v1/status`, `/sidecar/v1/ws`, and
  `/sidecar/v1/multimodal` compatible with the current React app and smoke
  tests.
- Add runner concepts: `runtime=litert|llamacpp`, `role=main|embedding|reranking`,
  `backend=cpu|gpu|npu|metal|vulkan|cuda|openvino|sycl`, status, port, model
  artifact, process health, logs, metrics, and capabilities.
- Add API/WebSocket operations for runner list/create/start/stop/restart,
  system probes, model catalog, downloads, and active route selection.
- Route `/v1/chat/completions`, `/v1/embeddings`, and `/v1/rerank` to selected
  runner aliases.
- Runtime binary strategy: detect configured/PATH binaries first; then offer
  official/prebuilt downloads where available; then source-build fallback for
  CUDA/SYCL/OpenVINO cases.
- HF auth must be consumed as `HF_TOKEN` or `HUGGING_FACE_HUB_TOKEN`, redacted
  from logs, never committed, and never passed as a CLI argument.

## Exact Model Artifacts

Download these exact requested model artifacts into ignored local model storage
under `models/` or a sidecar-managed ignored model cache:

- `unsloth/gemma-4-E2B-it-qat-GGUF`
  - `gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf`
- `Mungert/Qwen3-Embedding-0.6B-GGUF`
  - `Qwen3-Embedding-0.6B-q8_0.gguf`
- `litert-community/gemma-4-E2B-it-litert-lm`
  - `gemma-4-E2B-it.litertlm`
- `litert-community/embeddinggemma-300m`
  - `embeddinggemma-300M_seq2048_mixed-precision.tflite`

When each minimal runner path exists, test the downloaded artifact with the real
runtime binary instead of stopping at download success.

## HF Token And Download Procedure

The local Hugging Face token is stored outside the repository at:

```bash
/Users/krz/.codex/HF_TOKEN
```

Before downloading licensed/gated artifacts, load it into the environment
without printing it:

```bash
export HF_TOKEN="$(tr -d '\n' < /Users/krz/.codex/HF_TOKEN)"
export HUGGING_FACE_HUB_TOKEN="$HF_TOKEN"
```

Use an authenticated download helper that writes partial files outside Git and
renames them only after a successful transfer:

```bash
download_hf() {
  repo="$1"
  file="$2"
  out="$3"
  tmp="${out}.part"

  mkdir -p "$(dirname "$out")"
  curl --fail --location --continue-at - \
    --header "Authorization: Bearer ${HF_TOKEN}" \
    "https://huggingface.co/${repo}/resolve/main/${file}" \
    --output "$tmp"
  mv "$tmp" "$out"
}

download_hf \
  "unsloth/gemma-4-E2B-it-qat-GGUF" \
  "gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" \
  "models/llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf"

download_hf \
  "Mungert/Qwen3-Embedding-0.6B-GGUF" \
  "Qwen3-Embedding-0.6B-q8_0.gguf" \
  "models/llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf"

download_hf \
  "litert-community/gemma-4-E2B-it-litert-lm" \
  "gemma-4-E2B-it.litertlm" \
  "models/litert/main/gemma-4-E2B-it.litertlm"

download_hf \
  "litert-community/embeddinggemma-300m" \
  "embeddinggemma-300M_seq2048_mixed-precision.tflite" \
  "models/litert/embedding/embeddinggemma-300M_seq2048_mixed-precision.tflite"
```

Do not commit `models/`, `.part` files, downloaded runtimes, logs, or token
files. Redact `HF_TOKEN` and `HUGGING_FACE_HUB_TOKEN` from all sidecar and
runner logs.

## Runner Behavior

- LiteRT-LM main runner: launch `litert-lm serve` for Gemma 4 E2B with CPU/GPU/NPU
  options, MTP via `--enable-speculative-decoding=true`, and dynamic
  `--help`-based parameter discovery.
- LiteRT embedding runner: catalog/download `litert-community/embeddinggemma-300m`
  TFLite artifacts, but mark execution as adapter-backed because the current
  sidecar has no Go LiteRT embedding runtime. Implement via a small LiteRT
  CompiledModel helper or Python helper only after validating cross-platform
  packaging.
- llama.cpp main runner: launch `llama-server` with Gemma 4 E2B GGUF, backend
  profile, device, context, batch, threads, GPU layers, flash attention,
  reasoning/thinking parsing, and MTP draft settings.
- llama.cpp embedding runner: use `llama-server --embedding` with the requested
  Qwen3 Embedding 0.6B GGUF.
- llama.cpp reranking runner: use `llama-server --embedding --pooling rank
  --reranking`. First try the requested Qwen embedding artifact because it was
  requested for reranking too; if `/v1/rerank` health checks fail, report that
  clearly and offer a proper Qwen3 reranker GGUF fallback.
- Backend recommendation defaults:
  - macOS: Metal first.
  - NVIDIA: CUDA first, Vulkan fallback.
  - AMD/general GPU: Vulkan.
  - Intel iGPU/NPU: OpenVINO first for unified CPU/GPU/NPU; SYCL as advanced
    Intel GPU fallback.

## TUI Flow

- Tabs: Dashboard, Runners, Launch Wizard, Chat, Models, Logs, Settings.
- Dashboard shows OS/arch, CPU/RAM, GPU/NPU probes, installed runtimes,
  available backends, recommended backend per runtime, and missing dependency
  actions.
- Launch Wizard chooses role, runtime, model preset/custom path/HF repo, backend
  profile, port, context, sampler/server settings, raw advanced args, and
  dry-run command preview before start.
- Chat tab selects a running main runner, context profile, context budget,
  MTP/thinking display, and optional embedding-backed retrieval context when an
  embedding runner is active.
- Models tab downloads to ignored paths with resumable partials, size/progress
  display, license-gated HF auth, and no tracked artifacts.
- Logs tab streams sidecar and child process logs with token redaction.

## Test Plan

- Go unit tests for command builders, backend probes, runner supervisor state,
  downloader redaction/resume behavior, API/WS compatibility, and Bubble Tea
  update-model behavior.
- Existing checks:
  - `bun test`
  - `bun run build`
  - `cd native/sidecar && go test ./...`
  - `bun run build:sidecar`
  - `bun run smoke:executable`
- Add fake `litert-lm` and `llama-server` binaries to verify main/embed/rerank
  launch commands, health probes, log streaming, port allocation, route
  selection, and shutdown on host tests and cross-builds.
- Real model checks:
  - Download each exact requested artifact.
  - Start a real LiteRT-LM main runner against `gemma-4-E2B-it.litertlm`.
  - Start a real llama.cpp main runner against
    `gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf`.
  - Start a real llama.cpp embedding runner against
    `Qwen3-Embedding-0.6B-q8_0.gguf` and call `/v1/embeddings`.
  - Attempt the requested reranking setup and document whether the embedding
    artifact can serve `/v1/rerank`; use a proper reranker fallback only after
    the requested artifact fails validation.
- Final acceptance must include a real working TUI instance, not only unit tests:
  use the TUI to view dashboard probes, download or detect models, create
  runners, start/stop/restart runners, chat with a main runner, call embedding
  and rerank routes where supported, inspect logs, and verify the browser-facing
  sidecar API still works.
- Reiterate on every real-TUI failure until the implemented feature set passes
  or a concrete external blocker is documented.

## Implementation Handoff Prompt

```text
Implement /Users/krz/Dev/liteLLMwiki/plan.md end to end.

Critical requirements:
- Read and follow the authoritative Approach B design spec:
  docs/superpowers/specs/2026-06-19-sidecar-tui-multi-runner-design.md
- Replace the current single LiteRT manager as the internal runtime authority
  with a first-class multi-runner supervisor; preserve compatibility at the
  existing HTTP/WebSocket server boundary.
- Preserve existing sidecar HTTP/WebSocket API compatibility.
- Make Bubble Tea TUI the default sidecar mode, with --headless preserving
  current automation/browser behavior.
- Implement basics first: runner model, process supervisor, TUI
  dashboard/runners/logs/chat skeleton, model catalog/downloads, and one working
  llama.cpp main runner path.
- Download the exact requested models into ignored local model storage:
  - unsloth/gemma-4-E2B-it-qat-GGUF: gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf
  - Mungert/Qwen3-Embedding-0.6B-GGUF: Qwen3-Embedding-0.6B-q8_0.gguf
  - litert-community/gemma-4-E2B-it-litert-lm: gemma-4-E2B-it.litertlm
  - litert-community/embeddinggemma-300m:
    embeddinggemma-300M_seq2048_mixed-precision.tflite
- Use HF_TOKEN/HUGGING_FACE_HUB_TOKEN from the local environment or secret setup;
  redact it from logs and never commit it.
- Test downloaded models with real runtime commands as soon as each minimal
  runner works.
- At the end, run a real TUI instance and manually/automatically test all
  implemented features through the TUI, not only unit tests.
- Reiterate on every failure until the implemented feature set passes real TUI
  testing or a concrete external blocker is documented.

Verification required:
- git status --short before edits.
- Read README.md, package.json, native/sidecar/README.md, native/sidecar/go.mod
  before edits.
- bun test
- bun run build
- cd native/sidecar && go test ./...
- bun run build:sidecar
- bun run smoke:executable
- Real sidecar/TUI smoke with fake binaries.
- Real TUI smoke with available real LiteRT-LM and llama.cpp binaries/models.
```

## Assumptions And Sources

- Defaults chosen: TUI default with `--headless`; server runners first; binary
  strategy is detect existing, then download prebuilts, then source-build
  fallback.
- Bubble Tea: https://github.com/charmbracelet/bubbletea
- LiteRT-LM CLI: https://developers.google.com/edge/litert-lm/cli
- LiteRT-LM usage: https://developers.google.com/edge/litert-lm/cli/usage
- llama.cpp server: https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md
- llama.cpp build/backends: https://github.com/ggml-org/llama.cpp/blob/master/docs/build.md
- llama.cpp OpenVINO: https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/OPENVINO.md
- llama.cpp SYCL: https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/SYCL.md
