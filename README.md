# G0LiteLLaMa

Local Gemma runner workbench built around one Go executable. The old React web
UI is gone; root scripts now launch, install, configure, clean, and build the
native TUI/API process directly.

## Layout

- `G0LiteLLaMa/` - Go source, TUI, HTTP/WebSocket API, runtime supervisor.
- `G0LiteLLaMa/dist/` - ignored release artifacts; keep only `README.md`.
- `G0LiteLLaMa/litert-runtimes/` - ignored repo-local LiteRT-LM installs.
- `G0LiteLLaMa/llama-runtimes/` - ignored repo-local llama.cpp installs.
- `G0LiteLLaMa/runtime-config/` - ignored backend probe results.
- `models/` - ignored external model files; keep only `README.md`.

## Run

```bash
./launch-g0litellama.sh
```

Windows:

```powershell
.\launch-g0litellama.ps1
```

The launcher uses a built binary from `G0LiteLLaMa/dist/` when present and falls
back to:

```bash
cd G0LiteLLaMa
go run ./cmd/g0litellama
```

Pass `--headless` or `-Headless` for non-interactive runs. The process serves
OpenAI-compatible `/v1/*` routes and native `/g0litellama/v1/*` control routes.

## Setup

```bash
./install.sh
```

Windows:

```powershell
.\install.ps1
```

The installer checks Git, Go, curl, uv, local runtimes, selected models, runs Go
tests, builds release artifacts, and optionally runs backend probes.

Backend probes can be run directly:

```bash
./configure.sh
```

Windows:

```powershell
.\configure.ps1
```

Probe results are written to `G0LiteLLaMa/runtime-config/backends.json` and are
ignored by Git.

## Build And Test

```bash
cd G0LiteLLaMa
go test ./...
scripts/build-release.sh dist
```

Windows release build:

```powershell
cd G0LiteLLaMa
.\scripts\build-release.ps1 -OutDir dist
```

From the repo root, script syntax checks:

```bash
bash -n configure.sh install.sh launch-g0litellama.sh clean.sh
```

## Models

Model binaries are external artifacts. Put them under `models/`, for example:

```text
models/litert/main/gemma-4-E2B-it.litertlm
models/llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf
models/llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf
models/llamacpp/reranking/Qwen3-Reranker-0.6B-Q4_K_M.gguf
```

The installer can download the default selected models:
`gemma4-litert`, `embeddinggemma-litert`, and `qwen3-reranker-q4km`.
