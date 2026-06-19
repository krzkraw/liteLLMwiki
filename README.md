# LiteRT Gemma Local Chat

Text-first local Gemma workbench with two providers:

- **Web**: loads the Gemma 4 E2B web `.litertlm` model in the browser through
  LiteRT-LM WASM/WebGPU.
- **Executable**: connects to a local OpenAI-compatible native sidecar at
  `http://127.0.0.1:9379/v1`.

The app also indexes a selected folder, summarizes text files with the loaded
Gemma provider when available, and renders a deterministic knowledge graph.

## Run

Install dependencies from the repository root:

```bash
npm install
```

`npm install` runs `npm run prepare:wasm`, which copies LiteRT-LM WASM assets to:

```text
public/vendor/litert-lm/core/wasm
```

Start the dev server:

```bash
npm run dev -- --port 5173
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

`launch-webui` starts the Vite web UI with `WEBUI_HOST` and `WEBUI_PORT`
overrides. `launch-sidecar` starts the sidecar from
`native/sidecar-artifacts/` when a matching binary exists and falls back to
`go run ./cmd/litert-sidecar` from `native/sidecar`. It accepts sidecar flags
after the script name and reads common overrides such as `SIDECAR_BIN`,
`LITERT_LM_BIN`, `MODEL_FILE`, `MODEL_ID`, and `SIDECAR_HEADLESS=1`.
`launch-all` starts the web UI plus a headless sidecar and stops both when one
process exits.

For a guided first-time setup, run the interactive installer:

```bash
./install.sh
```

Windows PowerShell:

```powershell
.\install.ps1
```

To use a Nextcloud public share as the model download source instead of the
default Hugging Face URLs, pass the share URL at install time:

```bash
./install.sh modelsNextcloud=https://nextcloud.example/s/share-token
```

```powershell
.\install.ps1 -modelsNextcloud "https://nextcloud.example/s/share-token"
```

The installer checks local tools, npm dependencies, sidecar artifacts, and known
model paths. For every missing system dependency or model download it prints the
command or browser URL plus destination path first, then asks whether it should
run the action. Answer `y` to let the script run it, or `n` to do it yourself;
the installer waits until you press Enter and re-checks the dependency or file.
If an attempted install or download fails, it prints the command or URL again
and waits for you to complete it. Hugging Face tokens are prompted only for
downloads that may need one and are kept in the current process environment.

## Web Model

The browser provider expects the web model under the repository-local model
directory:

```text
models/litert/gemma-4-E2B-it-web.litertlm
```

The default URL inside the app is:

```text
/models/litert/gemma-4-E2B-it-web.litertlm
```

The Vite dev and preview servers serve `/models/*` from `models/`, so the large
`.litertlm` files do not need to be copied into `public/`.

The model source is:

```text
https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm
```

Download it when network/auth is available:

```bash
HF_TOKEN=... npm run download:model
npm run check:model
npm run smoke:model
```

You can also choose the `.litertlm` from disk with the `Choose local .litertlm`
control. Use the `gemma-4-E2B-it-web.litertlm` web model for browser WebGPU.

## Native Model

The native executable model is hosted outside this repository. Place the
externally hosted file at the path expected by the sidecar:

```text
models/litert/gemma-4-E2B-it.litertlm
```

Model binaries, partial downloads, and split model chunks are ignored by Git.

## Executable Provider

The executable provider is wired to an OpenAI-compatible local sidecar. The Go
source lives in `native/sidecar`, and the built native runner used by this web
UI is placed under `native/sidecar-artifacts/`.

Build the webUI-local native runner artifacts from the repository root:

```bash
npm run build:sidecar
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

The sidecar searches for `litert-lm`, imports
`models/litert/gemma-4-E2B-it.litertlm` as `gemma4-e2b` when needed, and starts
`litert-lm serve --host 127.0.0.1 --port 9381`.

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
npm test
npm run build
```

With the dev server running:

```bash
npm run smoke
npm run smoke:model
npm run smoke:executable
```

`smoke` covers the UI without requiring the large web model. `smoke:model`
requires `models/litert/gemma-4-E2B-it-web.litertlm` and checks that `/models/*` serves
the real binary with `application/octet-stream`.

To verify the production preview server catches the same route:

```bash
npm run build
npm run preview -- --port 5174
npm run smoke:model -- --url http://127.0.0.1:5174/
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
npm run e2e:generate
```
