# Sidecar TUI Multi-Runner Design

Date: 2026-06-19
Status: Approved direction for autonomous implementation
Selected approach: Approach B, full multi-runner supervisor replacement

## Objective

Rebuild the Go sidecar around a first-class multi-runner supervisor and Bubble
Tea TUI. The TUI is the default launch mode. `--headless` preserves current
automation and browser behavior. The implementation must keep the existing
browser-facing HTTP and WebSocket contract compatible while replacing the
current single LiteRT manager as the internal runtime authority.

## Requirements

### R1: Launch Modes

WHEN `litert-sidecar` starts without `--headless`
THE SYSTEM SHALL start the local HTTP server and open an interactive Bubble Tea
TUI connected to the same supervisor state.

WHEN `litert-sidecar --headless` starts
THE SYSTEM SHALL expose the current sidecar HTTP/WebSocket behavior without an
interactive TUI.

### R2: Compatibility

WHEN existing React UI clients call `/sidecar/v1/status`,
`/sidecar/v1/ws`, `/sidecar/v1/multimodal`, or `/v1/*`
THE SYSTEM SHALL keep compatible request and response behavior for existing
status, runtime control, logs, API tunnel, chat completions, and multimodal
flows.

WHEN legacy WebSocket messages `runtime.start`, `runtime.stop`, and
`runtime.restart` are received
THE SYSTEM SHALL map them to the default LiteRT main runner.

### R3: Runner Supervisor

WHEN a runner is created
THE SYSTEM SHALL store runtime, role, backend, model artifact, port, command,
status, health, capabilities, and log metadata.

WHEN a runner is started, stopped, or restarted
THE SYSTEM SHALL supervise the child process, update state, stream redacted
logs, publish status events, and preserve clean shutdown behavior.

WHEN `/v1/chat/completions`, `/v1/embeddings`, or `/v1/rerank` is called
THE SYSTEM SHALL route the request to the selected healthy runner for the
matching role.

### R4: Runtimes

WHEN a LiteRT main runner is launched
THE SYSTEM SHALL support Gemma 4 E2B with CPU, GPU, and NPU backend selection,
including current model import behavior and existing multimodal run support.

WHEN a llama.cpp main runner is launched
THE SYSTEM SHALL run `llama-server` against the requested Gemma GGUF artifact
and proxy OpenAI-compatible chat completions to it.

WHEN a llama.cpp embedding runner is launched
THE SYSTEM SHALL run `llama-server --embedding` against the requested Qwen3
embedding GGUF artifact and expose `/v1/embeddings` when health checks pass.

WHEN reranking is requested
THE SYSTEM SHALL first test the requested Qwen3 embedding GGUF with the
configured llama.cpp rerank command and document a concrete unsupported-result
if `/v1/rerank` cannot be made healthy with that artifact.

### R5: Models And Downloads

WHEN a model artifact is missing
THE SYSTEM SHALL show catalog state in the TUI and support authenticated
downloads into ignored local model storage.

WHEN downloads use Hugging Face authentication
THE SYSTEM SHALL read tokens from `HF_TOKEN` or `HUGGING_FACE_HUB_TOKEN`, never
pass them as CLI arguments, never commit them, and redact them from all logs and
errors.

WHEN downloads are interrupted
THE SYSTEM SHALL use partial files outside tracked Git content and rename only
after successful completion.

### R6: TUI

WHEN the TUI opens
THE SYSTEM SHALL provide Dashboard, Runners, Launch Wizard, Chat, Models, Logs,
and Settings views.

WHEN the user operates the TUI
THE SYSTEM SHALL support dashboard probes, model detection/download, runner
create/start/stop/restart, chat with a main runner, log inspection, and route
status for embeddings and reranking.

### R7: Verification

WHEN implementation is complete
THE SYSTEM SHALL pass `npm test`, `npm run build`, `cd native/sidecar && go test
./...`, `npm run build:sidecar`, `npm run smoke:executable`, fake-runtime
sidecar/TUI smoke, and real TUI testing with downloaded artifacts or a concrete
external blocker.

## Architecture

The current `litert.Manager` stops being the central runtime owner. Its behavior
is folded into a LiteRT runner implementation under a new supervisor. The
supervisor is the single runtime authority for both TUI and headless mode.

```text
cmd/litert-sidecar
  flags/config
  app service
    supervisor
      runner registry
      process supervisor
      route selector
      model catalog/download manager
      system probe service
      redacted log broadcaster
    HTTP/WebSocket server
    Bubble Tea TUI
```

The server boundary remains compatible. Existing handlers call the supervisor
instead of the old single manager. New endpoints and WebSocket operations expose
runner and model catalog actions, but old messages continue to work.

## Components

### App Service

Owns process lifetime, signal handling, shared logs, status events, and startup
mode. It starts the HTTP server in both modes. In TUI mode it also starts the
Bubble Tea program and shuts down runners and HTTP server on exit.

### Supervisor

Owns runner definitions and process state. It provides methods for list, create,
start, stop, restart, active route selection, status snapshots, and shutdown.
It guarantees one state path for TUI and HTTP/WebSocket operations.

### Runner Implementations

LiteRT and llama.cpp implementations build commands, inject token environment
where needed, run health probes, and describe capabilities. They do not own
global routing. The supervisor decides which runner receives each role route.

### Model Catalog And Downloader

Stores catalog entries for the four required artifacts:

- `models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf`
- `models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf`
- `models/gemma-4-E2B-it.litertlm`
- `models/litert/embeddinggemma-300M_seq2048_mixed-precision.tflite`

Downloads use environment-only auth, resumable partial files, atomic rename,
progress state, and redaction.

### TUI

The TUI reads supervisor snapshots and dispatches commands through the app
service. It starts as a functional skeleton, then grows controls around the
same operations exposed through WebSocket. The TUI must never be the only way to
perform an operation needed by automated smoke tests.

### Server

The server preserves current routes and WebSocket frame formats. It adds
runner/model operations while keeping old status and control messages stable.
The `/v1/*` proxy becomes role-aware: chat, embeddings, and rerank are routed to
the selected runner for that role.

## Data Model

Runner records include:

- `id`
- `runtime`: `litert` or `llamacpp`
- `role`: `main`, `embedding`, or `reranking`
- `backend`: `cpu`, `gpu`, `npu`, `metal`, `vulkan`, `cuda`, `openvino`, or
  `sycl`
- `modelPath`
- `modelID`
- `host`
- `port`
- `state`
- `pid`
- `upstream`
- `capabilities`
- `lastError`
- `logSequence`

Catalog entries include:

- `id`
- `repo`
- `filename`
- `targetPath`
- `runtime`
- `role`
- `required`
- `state`
- `bytesDownloaded`
- `sizeBytes`
- `lastError`

## Data Flow

Starting a runner:

1. TUI, WebSocket, or compatibility runtime control sends a start request.
2. Supervisor validates runner config and model artifact state.
3. Runtime implementation builds the command.
4. Process supervisor starts the child with redacted stdout/stderr writers.
5. Health probe checks the upstream route.
6. Supervisor marks the runner healthy and updates route selection.
7. Server status events notify TUI and WebSocket clients.

Downloading a model:

1. TUI or API asks the catalog to download an artifact.
2. Downloader reads token from environment only.
3. Downloader writes to a partial file and publishes progress.
4. On success it renames to the ignored target path.
5. Catalog state updates and the TUI runner wizard can use the artifact.

Routing:

1. HTTP or WebSocket API tunnel receives `/v1/*`.
2. Server classifies the route by role.
3. Supervisor returns the active healthy runner upstream for that role.
4. Proxy forwards the request and records upstream errors.

## Error Handling

All child stdout/stderr and command errors pass through a central redactor
before storage, TUI display, WebSocket logs, or test output. Tokens are never
rendered.

Runner failures leave records in `exited` or `unavailable` state with a concise
redacted detail. Failed health probes do not silently select a route. Rerank
failure with the requested Qwen embedding artifact is an acceptable documented
external/model-capability blocker only after a real `/v1/rerank` health attempt.

## Testing

Unit tests cover command builders, token env injection, redaction, model
catalog paths, downloader partial/rename behavior, route selection, supervisor
state transitions, server compatibility, and TUI update behavior.

Fake-runtime tests provide local fake `litert-lm` and `llama-server` binaries
that expose enough OpenAI-compatible endpoints to verify start/stop/restart,
logs, health probes, route selection, embeddings, rerank behavior, and TUI
actions.

Final verification runs the existing web and sidecar checks, downloads the
exact requested artifacts, and uses a real TUI instance to exercise dashboard
probes, downloads, runner create/start/stop/restart, chat, logs, embeddings,
and rerank support or documented rerank blocker.

## Alternatives Considered

Approach A, an additive supervisor around the existing LiteRT manager, lowers
initial compatibility risk but keeps two runtime ownership models alive.

Approach C, a separate TUI controller process, preserves the sidecar internals
but fragments logs, lifecycle, and test coverage.

Approach B is selected because the requested end state is a sidecar
supervisor/TUI, not a LiteRT wrapper. Compatibility is maintained at the server
contract boundary while runtime ownership is made coherent internally.
