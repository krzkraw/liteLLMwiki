# LiteRT Gemma Local Chat

Text-first local Gemma workbench with two providers:

- **Web**: loads the Gemma 4 E2B web `.litertlm` model in the browser through
  LiteRT-LM WASM/WebGPU.
- **Executable**: connects to a local OpenAI-compatible native sidecar at
  `http://127.0.0.1:9379/v1`.

The app also indexes a selected folder, summarizes text files with the loaded
Gemma provider when available, and renders a deterministic knowledge graph.

## Run

This project is Bun-only for JavaScript tooling. Use Bun directly for install,
test, build, smoke, and script commands.

Install dependencies from the repository root:

```bash
bun install
```

`bun install` runs `bun run prepare:wasm`, which copies LiteRT-LM WASM assets to:

```text
public/vendor/litert-lm/core/wasm
```

Start the dev server:

```bash
bun run dev --port 5173
```

Open:

```text
http://127.0.0.1:5173/
```

Chrome or another browser with WebGPU enabled is required for browser inference.
Headless Chromium can render and smoke-test the UI but may not expose a WebGPU
adapter.

Root launcher scripts are available for local development:

```bash
./launch-webui.sh
./launch-sidecar.sh
./launch-all.sh
```

Windows PowerShell equivalents:

```powershell
.\launch-webui.ps1
.\launch-sidecar.ps1
.\launch-all.ps1
```

`launch-webui` opens a new terminal window and starts the Rspack web UI with
`WEBUI_HOST` and `WEBUI_PORT` overrides. `launch-sidecar` opens a new terminal
window and starts the interactive sidecar TUI from
`native/sidecar-artifacts/` when a matching binary exists and falls back to
`go run ./cmd/litert-sidecar` from `native/sidecar`. It accepts sidecar flags
after the script name and reads common overrides such as `SIDECAR_BIN`,
`LITERT_LM_BIN`, `MODEL_FILE`, and `MODEL_ID`.
`launch-all` opens two separate terminal windows: one for the web UI and one
for the sidecar TUI. It starts the web UI first, then opens the sidecar TUI so
the dashboard is the foreground terminal. It forces the sidecar launcher onto
the TUI path even if a headless environment variable was left behind by smoke
testing. On macOS the launchers prefer the invoking terminal from
`TERM_PROGRAM`, including Ghostty, and support an explicit
`LITERT_TERMINAL_APP=Ghostty` override before falling back to Terminal.app. On
Windows the PowerShell scripts prefer Windows Terminal `--window new new-tab`
when available and fall back to starting a new PowerShell console.
For explicit non-interactive sidecar launches, pass `--headless` to
`launch-sidecar.sh` or `-Headless` to `launch-sidecar.ps1`.

For a guided first-time setup, run the interactive installer:

```bash
./install.sh
```

Windows PowerShell:

```powershell
.\install.ps1
```

To use a Nextcloud public share as the model download source instead of the
default Hugging Face URLs, pass the share URL at install time. The share should
point at the Models folder itself, with `litert/` and `llamacpp/` visible at the
share root; the installer maps repository paths such as `models/litert/...` to
share paths such as `litert/...`.

```bash
./install.sh modelsNextcloud=https://nextcloud.example/s/share-token
```

```powershell
.\install.ps1 -modelsNextcloud "https://nextcloud.example/s/share-token"
```

The installer checks local tools, Bun dependencies, sidecar artifacts, and known
model paths. It prints an up-front task list and marks already satisfied items
with a green checkmark when terminal color is available. It then shows checkbox
selectors for llama.cpp runtime folders and model downloads. The default model
selection is `gemma4-litert`, `gemma4-web-litert`,
`embeddinggemma-litert`, and `qwen3-reranker-q4km`; optional llama.cpp main and
embedding models can be toggled on from the same list. Checkbox selections are
treated as consent: selected runtimes and models download as a batch without a
second prompt per item. Missing non-checkbox dependencies still use a boxed task
prompt with the command or browser URL, the expected result, and `Y`/`N`/`M`
choices. Answer `Y` to let the script run the action, `M` to do it manually
while the installer waits and re-checks, or `N` to stop. If an attempted install
or download fails, it prints the command or URL again and waits for you to
complete it. Hugging Face tokens are prompted only for downloads that may need
one and are kept in the current process environment.
The installer also offers selectable local runtime installs for the current
platform. On macOS, the matching LiteRT-LM runtime and Apple Silicon llama.cpp
runtime choice are preselected by default, so pressing Enter installs the local
runtime folders even if Homebrew or a global uv tool already provides
`litert-lm` or `llama-server` on `PATH`.

LiteRT-LM runtimes are installed through uv under `native/litert-runtimes/` into
folders such as `litert-macos-arm64`. llama.cpp runtime archives are verified
by SHA256 and extracted under `native/llama-runtimes/` into folders such as
`llama-win-cpu-x64`, `llama-win-cuda-13.3-x64`, or `llama-macos-arm64`; CUDA
choices also extract the matching CUDA DLL archive into the same folder. The
launch scripts add the selected runtime folders to `PATH` before starting the
sidecar and pass the selected local `litert-lm` with `-runtime-exe`. Override
discovery with `LITERT_RUNTIME=<folder-name>`,
`LITERT_LM_BIN=/path/to/litert-lm`, `LLAMA_RUNTIME=<folder-name>`, or
`LLAMA_SERVER_BIN=/path/to/llama-server`.

## Web Model

The browser provider expects the web model under the repository-local model
directory:

```text
models/litert/browser/gemma-4-E2B-it-web.litertlm
```

The default URL inside the app is:

```text
/models/litert/browser/gemma-4-E2B-it-web.litertlm
```

The Rspack dev and preview servers serve `/models/*` from `models/`, so the large
`.litertlm` files do not need to be copied into `public/`.

The model source is:

```text
https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm
```

Download it when network/auth is available:

```bash
HF_TOKEN=... bun run download:model
bun run check:model
bun run smoke:model
```

You can also choose the `.litertlm` from disk with the `Choose local .litertlm`
control. Use the `gemma-4-E2B-it-web.litertlm` web model for browser WebGPU.

## Native Models

Native executable models are hosted outside this repository. The required model
catalog includes LiteRT, browser LiteRT, llama.cpp main, llama.cpp embedding,
and llama.cpp reranking artifacts. Place selected external files at the paths
expected by the sidecar, for example:

```text
models/litert/main/gemma-4-E2B-it.litertlm
models/litert/browser/gemma-4-E2B-it-web.litertlm
models/litert/embedding/embeddinggemma-300M_seq2048_mixed-precision.tflite
models/llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf
models/llamacpp/main/Qwen3.5-2B-IQ4_NL.gguf
models/llamacpp/main/Qwen3.5-0.8B-UD-Q8_K_XL.gguf
models/llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf
models/llamacpp/embedding/Qwen3-Embedding-0.6B-iq4_nl.gguf
models/llamacpp/reranking/Qwen3-Reranker-0.6B-Q4_K_M.gguf
```

Model binaries, partial downloads, and split model chunks are ignored by Git.

## Executable Provider

The executable provider is wired to an OpenAI-compatible local sidecar. The Go
source lives in `native/sidecar`, and the built native runner used by this web
UI is placed under `native/sidecar-artifacts/`.

Build the webUI-local native runner artifacts from the repository root:

```bash
bun run build:sidecar
```

Then start the sidecar manually from the same directory tree the web UI uses:

```bash
./native/sidecar-artifacts/litert-sidecar-darwin-arm64/litert-sidecar \
  -runtime-exe /path/to/litert-lm
```

The sidecar opens an interactive terminal dashboard by default. Use
`--headless` for browser automation, scripts, CI, or any non-interactive launch:

```bash
./native/sidecar-artifacts/litert-sidecar-darwin-arm64/litert-sidecar \
  --headless \
  -runtime-exe /path/to/litert-lm
```

Interactive TUI launches are lazy: no runtime or runner is started until the
Launch Wizard creates one. Headless launches preserve the automation path: the
sidecar searches for `litert-lm`, imports
`models/litert/main/gemma-4-E2B-it.litertlm` as `gemma4-e2b` when needed, and
starts `litert-lm serve --host 127.0.0.1 --port 9381`.
The TUI Launch Wizard creates runners from downloaded catalog models. It toggles
`litert` versus `llamacpp`; `litert` exposes `cpu`, `gpu`, and `npu` variants,
while `llamacpp` exposes installed `native/llama-runtimes` choices grouped as
`cpu`, `gpu`, `openvino`, `cuda13`, `cuda12`, and `sycl`. It then filters model
choices by role (`main`, `embedding`, or `reranking`) and only shows downloaded
applicable models.

The sidecar still exposes HTTP endpoints for manual checks and smoke scripts,
including status:

```text
http://127.0.0.1:9379/sidecar/v1/status
```

model catalog state:

```text
http://127.0.0.1:9379/sidecar/v1/models
```

and chat completions:

```text
http://127.0.0.1:9379/v1/chat/completions
```

In normal web UI use, executable status/control/logs plus text and multimodal
generation are tunneled through `ws://127.0.0.1:9379/sidecar/v1/ws` with
`api.request` frames. The sidecar routes those frames to `/v1/chat/completions`
or `/sidecar/v1/multimodal` underneath. The sidecar probes upstream `/v1/models`
for backend evidence. The base `gemma4-e2b` model means the default CPU/base
path is available; advertised `gemma4-e2b,gpu` and `gemma4-e2b,npu` IDs enable
those UI selections. CUDA is shown as probe-only and is not formatted into
LiteRT-LM model strings.

The executable sidecar also exposes native multimodal generation at:

```text
http://127.0.0.1:9379/sidecar/v1/multimodal
```

That endpoint accepts a prompt plus base64 image/audio attachments and runs
`litert-lm run` with `--attachment`, `--vision-backend`, and
`--audio-backend`. The browser provider remains text-only, but the chat
composer enables image/audio attachments after the executable sidecar connects
and advertises `capabilities.multimodal.state: "available"`.

## Folder Summary And Graph

Use the right-hand folder panel to choose a folder. Browsers that expose the
File System Access API show a direct `Choose folder` action; other browsers can
use the directory-capable file input fallback. The browser:

1. normalizes file paths across macOS/Windows-style separators;
2. ignores generated, binary, media, and model files;
3. chunks text files;
4. summarizes chunks, files, and the folder with the loaded Gemma provider, or
   uses deterministic preview summaries before a provider is loaded; and
5. renders a stable SVG knowledge graph from summary entities and topics.

## Multimodal Status

The browser provider is intentionally text-only. Native multimodal support
routes through the executable sidecar's WebSocket API tunnel to
`/sidecar/v1/multimodal`. In Executable mode, attached image/audio files are
encoded in the browser and sent only to the local sidecar; text-only prompts use
the same tunnel to reach the OpenAI-compatible streaming `/v1/chat/completions`
path.

## Verify

```bash
bun test
bun run build
```

With the dev server running:

```bash
bun run smoke
bun run smoke:model
bun run smoke:executable
```

`smoke` covers the UI without requiring the large web model. `smoke:model`
requires `models/litert/browser/gemma-4-E2B-it-web.litertlm` and checks that `/models/*` serves
the real binary with `application/octet-stream`.

To verify the production preview server catches the same route:

```bash
bun run build
bun run preview --port 5174
bun run smoke:model -- --url http://127.0.0.1:5174/
```

`smoke:executable` uses a fast mock sidecar. To exercise the real native
runtime, including a tiny PNG attachment through `/sidecar/v1/multimodal`, put
the external native model under `models/` or pass `MODEL_FILE`, then run from
`native/sidecar`:

```bash
LITERT_LM_BIN=/path/to/litert-lm scripts/real-runtime-smoke.sh
```

Optional real-model generation check:

```bash
HEADLESS=0 PLAYWRIGHT_CHANNEL=chrome \
MODEL_PATH=/path/to/gemma-4-E2B-it-web.litertlm \
bun run e2e:generate
```
