# G0LiteLLaMa

Go TUI/API process for managing local LiteRT-LM and llama.cpp runners.

It listens on `127.0.0.1:9379` by default, exposes control routes under
`/g0litellama/v1/*`, and proxies OpenAI-compatible `/v1/*` requests to the
selected runner.

## Run Locally

```bash
go test ./...
go build -o g0litellama ./cmd/g0litellama
./g0litellama
```

Use `./g0litellama --headless` for scripts, CI, or any process without a TTY.
The interactive TUI starts lazy: it does not start a default runtime runner
until one is launched from the wizard. Headless mode preserves the legacy
default LiteRT runner startup for automation.

Useful flags:

```text
-addr 127.0.0.1:9379
-runtime-exe /path/to/litert-lm
-model-file /path/to/gemma-4-E2B-it.litertlm
-model-id gemma4-e2b
-launch-runtime=false
-import-model=false
-runtime-verbose
--headless
```

## API Reference

See `../API.md` for the canonical API map.

## Release Builds

macOS/Linux:

```bash
scripts/build-release.sh dist
```

Windows:

```powershell
.\scripts\build-release.ps1 -OutDir dist
```

Artifacts are written as:

```text
dist/g0litellama-darwin-arm64/g0litellama
dist/g0litellama-darwin-amd64/g0litellama
dist/g0litellama-windows-amd64/g0litellama.exe
dist/g0litellama-windows-arm64/g0litellama.exe
```

## E2E

```bash
scripts/runtime-backend-e2e.sh
```

Real backend checks read `G0LiteLLaMa/runtime-config/backends.json` or
`RUNTIME_BACKEND_CONFIG` and skip missing models/runtimes unless
`G0LITELLAMA_E2E_REAL=1` is set.

## TUI Testing

Go tests cover model logic and fake controller behavior:

```bash
go test ./...
```

Rendered terminal E2E is driven from the repository root with Bun:

```bash
bun run e2e:tui
```

The rendered tests use a fixture binary under
`cmd/g0litellama-tui-fixture` so they do not start real runtimes or bind the
API server port.
