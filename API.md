# G0LiteLLaMa API

This is the canonical API map for G0LiteLLaMa. Update this file in the same
change whenever HTTP routes, WebSocket messages, action types, proxy routing,
request bodies, response bodies, or API error behavior changes.

Default base URL: `http://127.0.0.1:9379`

Native API prefix: `/g0litellama/v1`

OpenAI-compatible proxy prefix: `/v1`

Primary implementation files:

- `G0LiteLLaMa/internal/server/server.go`
- `G0LiteLLaMa/internal/server/actions.go`
- `G0LiteLLaMa/internal/server/websocket.go`
- `G0LiteLLaMa/internal/server/observe.go`
- `G0LiteLLaMa/internal/tui/store/`

## Route Map

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/g0litellama/v1/status` | Process, runtime, backend, and capability status. |
| `GET` | `/g0litellama/v1/models` | Local model catalog. |
| `POST` | `/g0litellama/v1/models/download` | Download one catalog model. |
| `GET` | `/g0litellama/v1/runners` | List runners and active role routes. |
| `POST` | `/g0litellama/v1/runners` | Create a runner. |
| `PATCH` | `/g0litellama/v1/runners/{id}` | Update a runner. |
| `POST` | `/g0litellama/v1/runners/{id}/start` | Start a runner. |
| `POST` | `/g0litellama/v1/runners/{id}/restart` | Restart a runner. |
| `POST` | `/g0litellama/v1/runners/{id}/stop` | Stop a runner. |
| `POST` | `/g0litellama/v1/runners/{id}/close` | Stop and remove a runner. |
| `POST` | `/g0litellama/v1/runners/{id}/route` | Pin a runner to a role route. |
| `POST` | `/g0litellama/v1/multimodal` | Run a native multimodal prompt. |
| `POST` | `/g0litellama/v1/actions` | Dispatch one action to the shared command bus. |
| `GET` | `/g0litellama/v1/events` | Read persisted bus events after a revision. |
| `GET` | `/g0litellama/v1/events/stream` | Stream committed bus actions as SSE. |
| `GET` | `/g0litellama/v1/state` | Read full app state projection. |
| `GET` | `/g0litellama/v1/state/runners` | Read runner state slice. Currently schema-only. |
| `GET` | `/g0litellama/v1/state/models` | Read model state slice. Currently schema-only. |
| `GET` | `/g0litellama/v1/state/chat/sessions/{id}` | Read one chat session projection. |
| `GET` | `/g0litellama/v1/tasks/{id}` | Read one task projection. |
| `GET` | `/g0litellama/v1/settings` | Read settings. Currently returns `{}`. |
| `GET` | `/g0litellama/v1/ws` | WebSocket control and API tunnel. |
| `ANY` | `/v1` | OpenAI-compatible proxy. |
| `ANY` | `/v1/*` | OpenAI-compatible proxy. |

## Native HTTP API

### `GET /g0litellama/v1/status`

Returns:

```json
{
  "state": "available",
  "backends": [
    { "backend": "cpu", "state": "available", "detail": "..." }
  ],
  "detail": "...",
  "runtime": {
    "state": "running",
    "executable": "/path/to/runtime",
    "version": "...",
    "modelId": "qwen35-08b-gguf",
    "modelFile": "models/...",
    "upstream": "http://127.0.0.1:9482",
    "mode": "release",
    "logSequence": 42,
    "detail": "..."
  },
  "capabilities": {
    "multimodal": {
      "state": "available",
      "endpoint": "/g0litellama/v1/multimodal",
      "detail": "...",
      "imageBackends": ["cpu", "gpu"],
      "audioBackends": ["cpu", "gpu"]
    }
  }
}
```

`runtime` is omitted when no runtime status source is configured.

### `GET /g0litellama/v1/models`

Returns:

```json
{
  "models": [
    {
      "id": "qwen35-08b-gguf",
      "repo": "unsloth/Qwen3.5-0.8B-GGUF",
      "filename": "Qwen3.5-0.8B-UD-Q8_K_XL.gguf",
      "targetPath": "models/llamacpp/main/Qwen3.5-0.8B-UD-Q8_K_XL.gguf",
      "runtime": "llamacpp",
      "role": "main",
      "required": true,
      "state": "missing",
      "bytesDownloaded": 0,
      "sizeBytes": 0,
      "lastError": "..."
    }
  ]
}
```

Known model states: `missing`, `present`, `downloading`, `error`.

### `POST /g0litellama/v1/models/download`

Request:

```json
{ "id": "qwen35-08b-gguf" }
```

Returns:

```json
{ "model": { "id": "qwen35-08b-gguf", "state": "present" } }
```

The `model` object uses the same catalog entry shape as `GET /models`.

### Runner Endpoints

Runner roles: `main`, `embedding`, `reranking`.

Runner runtimes currently used by the supervisor: `litert`, `llamacpp`.

`GET /g0litellama/v1/runners` returns:

```json
{
  "runners": [
    {
      "id": "LM-M-1",
      "runtime": "llamacpp",
      "role": "main",
      "backend": "metal",
      "executable": "/path/to/llama-server",
      "version": "...",
      "modelPath": "models/...",
      "modelId": "qwen35-08b-gguf",
      "host": "127.0.0.1",
      "port": 9482,
      "launch": true,
      "verbose": false,
      "state": "running",
      "pid": 24037,
      "upstream": "http://127.0.0.1:9482",
      "command": ["llama-server", "..."],
      "capabilities": { "chat": "true" },
      "lastError": "...",
      "logSequence": 42,
      "detail": "..."
    }
  ],
  "routes": {
    "main": "LM-M-1",
    "embedding": "LM-E-1",
    "reranking": "LM-R-1"
  }
}
```

`POST /g0litellama/v1/runners` request:

```json
{
  "id": "LM-M-1",
  "runtime": "llamacpp",
  "role": "main",
  "backend": "metal",
  "executable": "/path/to/llama-server",
  "modelPath": "models/llamacpp/main/model.gguf",
  "modelId": "qwen35-08b-gguf",
  "host": "127.0.0.1",
  "port": 9482,
  "launch": true,
  "upstream": "http://127.0.0.1:9482",
  "command": ["llama-server", "--model", "..."],
  "commandLine": "llama-server --model ...",
  "huggingfaceToken": "hf_...",
  "verbose": false
}
```

Returns `201`:

```json
{ "runner": { "id": "LM-M-1", "state": "stopped" } }
```

The `runner` object uses the runner snapshot shape above.

`PATCH /g0litellama/v1/runners/{id}` accepts the same fields as runner create,
except `launch`, `commandLine`, `huggingfaceToken`, and `verbose` are nullable
patch fields. Returns:

```json
{ "runner": { "id": "LM-M-1" } }
```

Runner action endpoints:

```text
POST /g0litellama/v1/runners/{id}/start
POST /g0litellama/v1/runners/{id}/restart
POST /g0litellama/v1/runners/{id}/stop
POST /g0litellama/v1/runners/{id}/close
```

Each returns:

```json
{ "runner": { "id": "LM-M-1", "state": "running" } }
```

`POST /g0litellama/v1/runners/{id}/route` request:

```json
{ "role": "main" }
```

Returns the updated runner snapshot.

### `POST /g0litellama/v1/multimodal`

Request:

```json
{
  "prompt": "Describe this image.",
  "modelId": "gemma4-e2b",
  "backend": "gpu",
  "visionBackend": "gpu",
  "audioBackend": "cpu",
  "maxNumTokens": 256,
  "topK": 40,
  "topP": 0.95,
  "temperature": 0.7,
  "seed": 1,
  "preset": "default",
  "noTemplate": false,
  "filterChannelContentFromKvCache": false,
  "enableSpeculativeDecoding": "",
  "cache": "",
  "verbose": false,
  "fromHuggingFaceRepo": "",
  "huggingfaceToken": "hf_...",
  "attachments": [
    {
      "name": "image.png",
      "mimeType": "image/png",
      "dataBase64": "..."
    }
  ]
}
```

Returns:

```json
{
  "text": "A concise description.",
  "detail": "..."
}
```

Limits: request body is capped at 40 MiB; each decoded attachment is capped at
32 MiB.

## Action, State, Events, Tasks, Settings

These endpoints expose the shared command bus used by the TUI and API clients.

### `POST /g0litellama/v1/actions`

Request:

```json
{
  "id": "optional-action-id",
  "type": "ui:select-tab",
  "source": "api",
  "correlationId": "optional-correlation-id",
  "parentId": "optional-parent-id",
  "time": "2026-06-25T12:00:00Z",
  "payload": { "tab_id": "chat" }
}
```

If `id` is empty, the server generates one. If `source` is empty, the server
sets it to `api`. If `time` is empty, the command bus fills it.

Returns:

```json
{
  "actionId": "optional-action-id",
  "revision": 12
}
```

Known action sources:

```text
tui
api
task
openai-proxy
system
```

Known action types and payloads:

| Type | Payload |
| --- | --- |
| `chat:new-session` | `{ "label": "optional" }` |
| `ui:select-tab` | `{ "tab_id": "chat" }` |
| `wizard:state` | `{ "runtime": "...", "backend": "...", "role": "...", "optionOverrides": { "...": "..." } }` |
| `proxy:request-start` | `{ "method": "POST", "path": "/v1/chat/completions", "role": "main" }` |
| `proxy:response-chunk` | `{ "correlationId": "...", "data": "...", "index": 0 }` |
| `proxy:response-end` | `{ "correlationId": "...", "statusCode": 200, "contentType": "text/event-stream" }` |
| `proxy:response-error` | `{ "correlationId": "...", "error": "..." }` |

`proxy:*` actions are normally emitted by the observing proxy, not manually.

### `GET /g0litellama/v1/state`

Returns the current app state projection:

```json
{
  "revision": 12,
  "viewport": { "width": 247, "height": 68 },
  "runners": {},
  "models": {},
  "runtime": {},
  "chat": {
    "sessions": {
      "session-id": {
        "id": "session-id",
        "source": "tui",
        "messages": [
          { "role": "user", "content": "Hello" }
        ],
        "createdAt": "2026-06-25T12:00:00Z",
        "updatedAt": "2026-06-25T12:00:00Z"
      }
    },
    "activeSessionId": "session-id"
  },
  "wizard": {
    "runtime": "llamacpp",
    "backend": "metal",
    "role": "main",
    "optionOverrides": {}
  },
  "tasks": {
    "items": {}
  },
  "ui": {
    "activeTab": "chat"
  }
}
```

`runners`, `models`, and `runtime` state slices are currently schema-only.

### State Slice Endpoints

```text
GET /g0litellama/v1/state/runners
GET /g0litellama/v1/state/models
GET /g0litellama/v1/state/chat/sessions/{id}
```

Chat session not found returns structured `404`.

### `GET /g0litellama/v1/tasks/{id}`

Returns one task from `state.tasks.items`.

Task shape:

```json
{
  "ID": "task-id",
  "ParentID": "parent-task-id",
  "Kind": "download",
  "Status": "running",
  "StatePath": "models.items.model-id",
  "Progress": { "Percent": 50, "Message": "halfway" },
  "Summary": "...",
  "Error": "...",
  "Events": [
    { "At": "2026-06-25T12:00:00Z", "Message": "started" }
  ],
  "CreatedAt": "2026-06-25T12:00:00Z",
  "UpdatedAt": "2026-06-25T12:00:00Z"
}
```

Current Go task structs do not define JSON tags, so exported Go field names are
the wire keys.

Known task statuses: `pending`, `running`, `completed`, `failed`, `cancelled`.

### `GET /g0litellama/v1/settings`

Currently returns:

```json
{}
```

### `GET /g0litellama/v1/events`

Query parameters:

| Name | Required | Description |
| --- | --- | --- |
| `after` | No | Return events with revision greater than this unsigned integer. Defaults to `0`. |

Returns:

```json
[
  {
    "revision": 12,
    "actionId": "action-id",
    "type": "ui:select-tab",
    "payload": { "tab_id": "chat" },
    "createdAt": 1782398400000000000
  }
]
```

If no event log backend is configured, returns an empty array.

### `GET /g0litellama/v1/events/stream`

Server-Sent Events stream. Each event has one `data:` payload containing a
stored action:

```json
{
  "Action": {
    "id": "action-id",
    "type": "ui:select-tab",
    "source": "api",
    "payload": { "tab_id": "chat" }
  },
  "Revision": 12
}
```

Current Go stored-action structs do not define JSON tags, so exported Go field
names are the wire keys.

## OpenAI-Compatible Proxy

`/v1` and `/v1/*` are transparent proxy routes to the selected upstream runner,
except `GET /v1/models` is served locally when a runner controller is present.

Local `GET /v1/models` response:

```json
{
  "object": "list",
  "data": [
    {
      "id": "qwen35-08b-gguf",
      "object": "model",
      "created": 0,
      "owned_by": "g0litellama"
    }
  ]
}
```

Proxy role routing:

| Path | Runner role |
| --- | --- |
| `/v1/embeddings` and `/v1/embeddings/*` | `embedding` |
| `/v1/rerank` and `/v1/rerank/*` | `reranking` |
| Everything else under `/v1` | `main` |

The proxy observes `/v1/*` traffic and dispatches `proxy:*` actions to the
shared command bus when the bus is configured. Response body reads produce
`proxy:response-chunk`; response close produces `proxy:response-end`.

## WebSocket API

Endpoint: `GET /g0litellama/v1/ws`

The WebSocket accepts JSON text messages.

Client envelope:

```json
{
  "type": "api.request",
  "id": "request-1",
  "mode": "release",
  "config": {},
  "method": "POST",
  "path": "/v1/chat/completions",
  "headers": { "content-type": "application/json" },
  "bodyBase64": "..."
}
```

Client message types:

| Type | Fields | Description |
| --- | --- | --- |
| `status.get` | none | Send one current `status` message. |
| `runtime.start` | `mode`, `config` | Start runtime. Mode must be `release` or `debug`. |
| `runtime.stop` | none | Stop runtime. |
| `runtime.restart` | `mode`, `config` | Restart runtime. Mode must be `release` or `debug`. |
| `logs.subscribe` | none | Replay buffered logs, then stream new logs. |
| `api.request` | `id`, `method`, `path`, `headers`, `bodyBase64` | Run an HTTP-like request through the WebSocket tunnel. |
| `api.cancel` | `id` | Cancel an active `api.request`. |

`runtime.start` and `runtime.restart` config shape:

```json
{
  "upstream": "http://127.0.0.1:9381",
  "runtimeExe": "/path/to/litert-lm",
  "runtimeHost": "127.0.0.1",
  "runtimePort": 9481,
  "modelFile": "models/litert/main/gemma-4-E2B-it.litertlm",
  "modelId": "gemma4-e2b",
  "huggingfaceToken": "hf_...",
  "importModel": false,
  "launchRuntime": true,
  "runtimeVerbose": true
}
```

`api.request` accepts these native paths directly:

```text
/g0litellama/v1/status
/g0litellama/v1/models
/g0litellama/v1/models/download
/g0litellama/v1/runners
/g0litellama/v1/runners/*
/g0litellama/v1/multimodal
/v1
/v1/*
```

Other native action/state/event/settings endpoints are HTTP-only today.

Server message types:

`status`:

```json
{
  "type": "status",
  "status": {}
}
```

`log`:

```json
{
  "type": "log",
  "entry": {
    "seq": 1,
    "source": "runtime",
    "stream": "stdout",
    "line": "ready"
  }
}
```

`error`:

```json
{
  "type": "error",
  "message": "unknown message type"
}
```

`api.error`:

```json
{
  "type": "api.error",
  "id": "request-1",
  "message": "context canceled"
}
```

`api.response.start`:

```json
{
  "type": "api.response.start",
  "id": "request-1",
  "status": 200,
  "headers": { "content-type": "application/json" }
}
```

`api.response.chunk`:

```json
{
  "type": "api.response.chunk",
  "id": "request-1",
  "dataBase64": "..."
}
```

`api.response.end`:

```json
{
  "type": "api.response.end",
  "id": "request-1"
}
```

## Error Behavior

Native status, model, runner, multimodal, proxy, and WebSocket handshake
handlers mostly use `http.Error` or text responses for errors.

Runner API errors use `text/plain; charset=utf-8`:

```text
method not allowed
```

Action/state/event/task/settings API errors are structured JSON:

```json
{
  "error": "method not allowed",
  "code": "unsupported"
}
```

Known structured error codes:

```text
bad_request
not_found
conflict
internal_error
unsupported
```

Proxy route errors:

- `502` when the upstream proxy is not configured or unavailable.
- `501` when no routed runner exists for the requested `/v1/*` path.
- Otherwise proxied upstream status, headers, and body are preserved.

## CORS

Default allowed origins:

```text
http://127.0.0.1:5173
http://localhost:5173
```

When an allowed `Origin` is present, the server returns:

```text
Access-Control-Allow-Origin: <origin>
Vary: Origin
Access-Control-Allow-Headers: content-type
Access-Control-Allow-Methods: GET,POST,OPTIONS
```

Current implementation supports direct `PATCH` for runner updates, but CORS
does not advertise `PATCH`.

## Persistence

The command bus can persist actions, events, snapshots, tasks, and settings to
SQLite. This is not a raw SQL API surface, but it backs `/events`, `/state`,
`/tasks`, and future settings behavior.

Database path:

- `G0LITELLAMA_DB_PATH`, when set.
- macOS: `~/Library/Application Support/G0LiteLLaMa/g0litellama.db`
- Windows: `%APPDATA%/G0LiteLLaMa/g0litellama.db`
- Linux/other: `~/.config/g0litellama/g0litellama.db`

SQLite tables:

```text
actions
events
tasks
snapshots
settings
```
