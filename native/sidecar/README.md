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

By default it:

- searches for `litert-lm` on `PATH` or beside the sidecar binary;
- searches for `models/gemma-4-E2B-it.litertlm`;
- imports the model as `gemma4-e2b` when it is missing from the LiteRT-LM
  registry;
- starts `litert-lm serve --host 127.0.0.1 --port 9381`.

In a fresh clone, provide the native model from the external model host before
starting the sidecar. The default local path is
`models/gemma-4-E2B-it.litertlm`; model binaries are ignored by Git.

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
`models/gemma-4-E2B-it.litertlm`, or run with `-model-file`.

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
