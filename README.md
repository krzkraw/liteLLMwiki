# liteLLMwiki

Local Gemma workbench for browser and native LiteRT-LM experiments.

This repository contains the web UI and the Go sidecar only. Large Gemma
`.litertlm` model files are intentionally not tracked and are ignored by Git;
place them under `demo/models/` locally or choose them from disk in the UI.

## Contents

- `demo/` - React/Vite web UI for local chat, folder summarization, and a
  knowledge graph.
- `native/sidecar/` - Go sidecar that exposes an OpenAI-compatible endpoint and
  WebSocket runtime control for `litert-lm`.
- `demo/native/sidecar/` - ignored build-output location for webUI-adjacent
  sidecar release artifacts.

Pitch material was moved out of this repo to:

```text
/Users/krz/Documents/Local-llm-pitch
```

## Models

Models are external artifacts. The expected local paths are:

```text
demo/models/gemma-4-E2B-it-web.litertlm
demo/models/gemma-4-E2B-it.litertlm
```

The web model can be downloaded when Hugging Face access is available:

```bash
cd demo
HF_TOKEN=... npm run download:model
npm run check:model
```

Do not commit models, model chunks, or generated sidecar binaries.

## Web UI

```bash
cd demo
npm install
npm run dev -- --port 5173
```

Open `http://127.0.0.1:5173/`.

Verification:

```bash
cd demo
npm test
npm run build
```

With the dev server running:

```bash
npm run smoke
npm run smoke:model
npm run smoke:executable
```

`smoke:model` requires the external web model under `demo/models/`.

## Sidecar

Build the sidecar release artifacts into the webUI-adjacent ignored location:

```bash
cd demo
npm run build:sidecar
```

Run the macOS arm64 sidecar manually:

```bash
cd demo
./native/sidecar/litert-sidecar-darwin-arm64/litert-sidecar \
  -runtime-exe /path/to/litert-lm
```

The web UI then controls the sidecar over:

```text
ws://127.0.0.1:9379/sidecar/v1/ws
```

Go verification:

```bash
cd native/sidecar
go test ./...
```
