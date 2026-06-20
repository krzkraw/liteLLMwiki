# LiteRT-LM Sidecar

Small Go executable for the web UI executable provider.

The sidecar listens on `127.0.0.1:9379`, exposes
`/sidecar/v1/status`, streams browser control over `/sidecar/v1/ws`, and proxies
OpenAI-compatible `/v1/*` requests to a managed `litert-lm serve` child process
on `127.0.0.1:9381`.

## Run Locally

```bash
go test ./...
go build -o litert-sidecar ./cmd/litert-sidecar
./litert-sidecar
```

`./litert-sidecar` starts the HTTP/WebSocket sidecar and opens the interactive
terminal dashboard. Use `./litert-sidecar --headless` for browser automation,
smoke tests, CI, or any process without a TTY.

The interactive TUI starts lazy: it does not create or start a default runtime
runner before the user chooses one in the Launch Wizard. Headless mode preserves
the legacy default LiteRT runner startup for browser smoke tests and automation.

The TUI is currently focused on the native runner basics. It shows only a
Dashboard tab, a Launch Wizard tab, and runner tabs created from the wizard.
Chat, Models, Logs, and Settings tabs are intentionally hidden while the native
runner workflow is being stabilized. The status header reports LiteRT and
llama.cpp independently as `active` or `idle` based on whether each runtime has
any running runner.

The Dashboard contains a single status widget: live runner counts by runtime,
live runner counts by role, and model-file counts by role. Clicking the Main,
Embedding, or Reranking model count opens a small local-model list for that
role. The old system-health, topology, signal-board, route-map, backend-card,
recent-activity, and command-rail panels are no longer part of the dashboard.

The bottom line is the action surface, htop-style. It always shows global
actions and the current tab's actions. `F1` or clicking the F1 area opens a
bottom-left global menu with navigation and quit actions.

The Launch Wizard is a compact configuration screen. It lets the user click or
key-select the runtime (`litert` or `llamacpp`), a runtime variant, model role
(`main`, `embedding`, or `reranking`), and one locally installed matching model.
LiteRT variants are `cpu`, `gpu`, and `npu`. llama.cpp variants are shown as
`cpu`, `gpu`, `openvino`, `cuda13`, `cuda12`, and `sycl`, and are mapped to
installed folders under `native/llama-runtimes`. Pressing Enter or clicking
`START` creates and starts a runner. New runner tabs are inserted after the
Launch Wizard and are named by runtime and role, such as `LR-M-1`, `LM-E-1`,
or `LM-R-1`; numbering is per role.

Runner tabs show a single basic status/control panel with runtime, role,
backend, model, upstream, PID, and `s`/`x`/`r` start/stop/restart actions.

In headless mode, the sidecar still:

- searches for `litert-lm` on `PATH` or beside the sidecar binary;
- searches for `models/litert/main/gemma-4-E2B-it.litertlm`;
- imports the model as `gemma4-e2b` when it is missing from the LiteRT-LM
  registry;
- starts `litert-lm serve --host 127.0.0.1 --port 9381`.

In a fresh clone, provide selected models from the external model hosts before
starting the sidecar. The default LiteRT main path is
`models/litert/main/gemma-4-E2B-it.litertlm`; llama.cpp main, embedding, and
reranking models live under `models/llamacpp/`. Model binaries are ignored by
Git.
The sidecar also exposes a model catalog at `/sidecar/v1/models` and supports
authenticated Hugging Face downloads through `/sidecar/v1/models/download`.
The supervisor can also start a `llama-server` main runner against a GGUF model
and route OpenAI-compatible chat requests to that runner. llama.cpp embedding
runners use `--embedding`; reranking runners use `--embedding --pooling rank
--reranking` and route `/v1/rerank` when healthy.

Useful flags:

```text
-runtime-exe /path/to/litert-lm
-model-file /path/to/gemma-4-E2B-it.litertlm
-model-id gemma4-e2b
-launch-runtime=false
-import-model=false
-runtime-verbose
```

Use `-launch-runtime=false` when a separate `litert-lm serve` process is
already running.

## Browser Control

After the sidecar process is started manually, the web UI controls the managed
runtime through:

```text
ws://127.0.0.1:9379/sidecar/v1/ws
```

Supported client messages:

```json
{ "type": "status.get" }
{ "type": "logs.subscribe" }
{ "type": "runtime.start", "mode": "release", "config": { "runtimeExe": "/path/to/litert-lm" } }
{ "type": "runtime.restart", "mode": "debug", "config": { "runtimePort": 9481 } }
{ "type": "runtime.stop" }
```

The sidecar pushes `status`, `log`, and `error` messages over the same socket.
Runtime status changes are emitted when the managed process starts, stops,
exits, or is reconfigured. Text generation still uses the OpenAI-compatible
HTTP `/v1/chat/completions` endpoint; native multimodal prompts use
`/sidecar/v1/multimodal`.

Runner management is also exposed over HTTP:

```text
GET  /sidecar/v1/runners
POST /sidecar/v1/runners
PATCH /sidecar/v1/runners/{id}
POST /sidecar/v1/runners/{id}/start
POST /sidecar/v1/runners/{id}/restart
POST /sidecar/v1/runners/{id}/stop
```

`POST /sidecar/v1/runners` accepts runner fields such as `id`, `runtime`,
`role`, `backend`, `executable`, `modelPath`, `modelId`, `host`, `port`,
`launch`, and `upstream`. `PATCH /sidecar/v1/runners/{id}` accepts the same
fields as a partial update for a runner that is not currently starting or
running. The same routes are available through WebSocket `api.request` frames.
Long-lived start and restart operations are detached from the request context,
while stop operations still honor the caller timeout.

`/sidecar/v1/status` probes the upstream `/v1/models` endpoint when the runtime
is available. The base `gemma4-e2b` model means the default CPU/base path is
available; `gemma4-e2b,gpu` and `gemma4-e2b,npu` advertise concrete GPU and NPU
paths. CUDA is reported as probe-only because LiteRT-LM uses the `gpu` backend
selection rather than a CUDA model suffix. If the model probe fails, status
falls back to runtime-state evidence.

## Native Multimodal

The web chat stays text-only, but the sidecar exposes a native attachment
endpoint backed by `litert-lm run`:

```bash
curl http://127.0.0.1:9379/sidecar/v1/multimodal \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Describe this image.",
    "backend": "gpu",
    "visionBackend": "gpu",
    "attachments": [
      {
        "name": "sample.png",
        "mimeType": "image/png",
        "dataBase64": "..."
      }
    ]
  }'
```

The endpoint writes attachments to a temporary private directory and calls
`litert-lm run <model-id> --attachment=<file> --vision-backend=<backend>` or
`--audio-backend=<backend>`.

The web UI enables its image/audio attachment picker only when this sidecar
capability is advertised as available. Text-only executable chat continues to
flow through `/v1/chat/completions`.

## Release Builds

macOS/Linux shell:

```bash
scripts/build-release.sh
```

Windows PowerShell:

```powershell
.\scripts\build-release.ps1
```

Both scripts build:

- `darwin/arm64`
- `darwin/amd64`
- `windows/amd64`
- `windows/arm64`

When you pass an output directory, the scripts write release artifacts there.
With no output argument, they place binaries under
`../../native/sidecar-artifacts/` so the native runner lives beside the web UI
that controls it without colliding with this source tree. They do not copy the
large `.litertlm` model file. Put the external native model under
`models/litert/main/gemma-4-E2B-it.litertlm`, or run with `-model-file`.

`go test ./...` runs a host-side release artifact test for the shell build
script. It cross-compiles the four targets into a temporary directory, checks
each package includes `README.md`, and verifies Windows binaries have PE `MZ`
headers while macOS binaries have Mach-O headers. This proves artifact shape,
not real Windows runtime execution.

## Real Runtime Smoke

Run the opt-in smoke when a real `litert-lm` binary and model file are
available:

```bash
LITERT_LM_BIN=/path/to/litert-lm scripts/real-runtime-smoke.sh
```

The script builds a temporary sidecar binary, launches it on local random
ports, uses an isolated LiteRT-LM `HOME`, asserts `/v1/models` advertises
`gemma4-e2b`, sends one non-streaming chat completion, and sends a tiny PNG
attachment through `/sidecar/v1/multimodal`. Set `LITERT_HOME` to reuse an
isolated imported-model registry across runs, and set
`MULTIMODAL_TIMEOUT_SECONDS` for slower attachment generation.
