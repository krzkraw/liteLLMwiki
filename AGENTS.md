# AGENTS.md

This file is the operating contract for autonomous agents working in
`liteLLMwiki`.

## Project Description

`liteLLMwiki` now ships only the native Go runner, renamed to `G0LiteLLaMa`.
The React/Rspack web UI and Bun package scripts were removed. The Go process
serves OpenAI-compatible `/v1/*` routes and native `/g0litellama/v1/*` control
routes.

Model binaries are external artifacts. Keep them under `models/`; do not track
them in Git.

## Source Of Truth

- `README.md` - human setup, structure, model policy, and verification commands.
- `G0LiteLLaMa/README.md` - runtime contract and release commands.
- `G0LiteLLaMa/go.mod` - Go module definition.

## Project Map

- `G0LiteLLaMa/cmd/` - Go entry point.
- `G0LiteLLaMa/internal/` - runtime, proxy, server, supervisor, and TUI code.
- `G0LiteLLaMa/e2e/` - TUI and runtime/backend E2E tests.
- `G0LiteLLaMa/scripts/` - release and real-runtime smoke helpers.
- `G0LiteLLaMa/dist/` - ignored release artifacts; keep only `README.md`.
- `G0LiteLLaMa/runtime-config/` - ignored backend probe results.
- `G0LiteLLaMa/litert-runtimes/` - ignored local LiteRT-LM installs.
- `G0LiteLLaMa/llama-runtimes/` - ignored local llama.cpp installs.
- `models/` - ignored local model directory; keep only `README.md`.

## Verification Commands

Run from the repository root unless noted.

```bash
# Script syntax checks
bash -n configure.sh install.sh launch-g0litellama.sh clean.sh

# Go tests
cd G0LiteLLaMa && go test ./...

# E2E planner/TUI checks; real runtime combos skip unless configured
cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh

# Release artifacts
cd G0LiteLLaMa && scripts/build-release.sh dist
```

Real backend checks read `G0LiteLLaMa/runtime-config/backends.json` or
`RUNTIME_BACKEND_CONFIG` and skip missing models/runtimes with clear reasons
unless `G0LITELLAMA_E2E_REAL=1` is set. Real LiteRT smoke requires
`LITERT_LM_BIN=/path/to/litert-lm`.

## Required First Steps

1. Inspect the current git status with `git status --short`.
2. Read the relevant README and `G0LiteLLaMa/README.md` before edits.
3. Preserve user changes. Do not reset, clean, stash, or discard work unless the
   user explicitly asks.
4. Keep model files, generated release binaries, dependency folders, build
   outputs, caches, and local logs out of Git.

## Model And Artifact Rules

- Never add Git LFS to this repository.
- Never commit `.litertlm` files, `.litertlm.parts/`, partial downloads, GGUF
  model files, or model caches.
- Keep the model catalog and installer model selector aligned. The default
  installer model selection is `gemma4-litert`, `embeddinggemma-litert`, and
  `qwen3-reranker-q4km`.
- Never commit generated files under `G0LiteLLaMa/dist/` except
  `G0LiteLLaMa/dist/README.md`.
- Never commit downloaded LiteRT-LM uv tool folders under
  `G0LiteLLaMa/litert-runtimes/`.
- Never commit downloaded llama.cpp runtime folders or CUDA DLLs under
  `G0LiteLLaMa/llama-runtimes/`.
- Never commit generated `G0LiteLLaMa/runtime-config/backends.json`.

## Workflow Rules

- Prefer existing project patterns over new abstractions.
- Keep changes scoped to the user request.
- Update `README.md` and this file when setup, commands, structure, or model
  policy changes.
- Use exact commands for verification evidence.
- If verification cannot run, state exactly what was skipped and why.

## Git Rules

- Commit only intentional source/doc changes.
- Do not commit local models or generated artifacts.
- Do not force-push, rewrite history, run `git clean`, or delete untracked user
  files unless the user explicitly asks.

## Pending Plans

- None.
