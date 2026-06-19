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

The TUI uses the same runtime and runner controller methods as the HTTP routes
and WebSocket `api.request` bridge. It opens with a colorized status header,
rounded panels, runtime/runner/route/log counters, and a status-rich tab bar.
The tab bar keeps number-key navigation stable while showing dashboard runner
counts, per-runner state glyphs, model readiness, log count, and Settings API
readiness. On wide terminals the dashboard pairs panels into two-column rows
for faster scanning.
Every tab ends with a context command rail that keeps global navigation,
tab-specific actions, and the matching controller or WebSocket/API path visible
without switching pages.
Its dashboard lists runtime specs, a visual topology graph, route authority,
runnable backend cards, runtime topology, route maps, recent activity, and a
signal board with readiness meters for runtime, runners, routes, required model
artifacts, and logs.
Each configured runner gets its own tab with health, a per-runner readiness
signal board, endpoint, control surface, operation flow, runtime command,
capability matrix, settings, process details, and recent log panels. The signal
board summarizes runner state, route, process, model, capabilities, log cache,
and the next useful action. On wide terminals runner tabs pair high-signal
panels into two-column rows so health, routes, controls, settings, details, and
logs stay visible without a long single-column scan. The operation flow shows
the runner state, model file, runtime/backend, upstream, role route, controller
methods, and matching WebSocket `api.request` paths. Runner controls use `s`
start, `x` stop, and `r` restart. Runner tabs include a settings matrix that
lists each edit key, current value, `PATCH` field, and
`RunnerController.UpdateRunner` method.
They edit settings through the same update method behind
`PATCH /sidecar/v1/runners/{id}`: `b` backend, `p` port, `h` host, `i` model ID,
`m` model path, `e` executable, `u` upstream, `f` Hugging Face token, `l`
launch mode, `v` verbose, `t` runtime, and `o` role. Typed edits show the
current value, accept a replacement value, save with Enter, and cancel with Esc;
HF token edits mask the typed value and only report `set` or `cleared`. The
Settings tab lists the matching WebSocket messages and sidecar API paths,
exposes default runtime controls with `s` start release, `d` start debug, `x`
stop, `r` restart release, and `g` restart debug, and includes a runtime config
editor for the same `runtime.start` and `runtime.restart` config fields used by
WebSocket clients. Its shared action map shows each TUI key beside the
controller method and matching WebSocket/API route it uses. Its live runner API
parity panel is generated from `RunnerController.Snapshot()` and lists each
runner's role, state, routed OpenAI path, `RunnerController` methods, and
concrete `api.request` PATCH/start/stop/restart paths. Its API parity panel
includes the model catalog, runner management, native multimodal, and `/v1/*`
upstream proxy paths supported by WebSocket `api.request`.
Settings keys edit `e` runtime executable, `h` runtime host, `p` runtime port,
`m` model file, `i` model ID, `u` upstream, and `f` Hugging Face token, plus
`l` launch runtime, `a` import model, and `v` runtime verbose toggles. The
Models tab can download the next missing required catalog artifact with `d` by
calling the same catalog download method behind
`POST /sidecar/v1/models/download`; it can also create catalog-backed llama.cpp
runners with `m` main, `e` embedding, and `r` rerank by calling the same runner
creation method behind `POST /sidecar/v1/runners`.

By default it:

- searches for `litert-lm` on `PATH` or beside the sidecar binary;
- searches for `models/litert/gemma-4-E2B-it.litertlm`;
- imports the model as `gemma4-e2b` when it is missing from the LiteRT-LM
  registry;
- starts `litert-lm serve --host 127.0.0.1 --port 9381`.

In a fresh clone, provide the native model from the external model host before
starting the sidecar. The default local path is
`models/litert/gemma-4-E2B-it.litertlm`; model binaries are ignored by Git.
The sidecar also exposes a model catalog at `/sidecar/v1/models` and supports
authenticated Hugging Face downloads through `/sidecar/v1/models/download`.
The supervisor can also start a `llama-server` main runner against a GGUF model
and route OpenAI-compatible chat requests to that runner. llama.cpp embedding
runners use `--embedding`; rerank probes use `--embedding --pooling rank
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
`models/litert/gemma-4-E2B-it.litertlm`, or run with `-model-file`.

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
