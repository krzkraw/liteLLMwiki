# AGENTS.md

This file is the operating contract for autonomous agents working in
`liteLLMwiki`. Follow the nearest nested `AGENTS.md` when working inside a
subdirectory that has one.

## Project Description

`liteLLMwiki` now ships only the native Go runner, renamed to `G0LiteLLaMa`.
The React/Rspack web UI and old app scripts were removed. The repo keeps a
small Bun package only for rendered TUI E2E. The Go process serves
OpenAI-compatible `/v1/*` routes and native `/g0litellama/v1/*` control routes.

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

# Rendered TUI E2E
bun run e2e:tui

# Release artifacts
cd G0LiteLLaMa && scripts/build-release.sh dist
```

For docs-only changes, at minimum run `git diff --check`.

Real backend checks read `G0LiteLLaMa/runtime-config/backends.json` or
`RUNTIME_BACKEND_CONFIG` and skip missing models/runtimes with clear reasons
unless `G0LITELLAMA_E2E_REAL=1` is set. Real LiteRT smoke requires
`LITERT_LM_BIN=/path/to/litert-lm`.

## Rendered TUI E2E

The project is Go-first, but rendered terminal E2E uses Bun plus
`@microsoft/tui-test` on macOS/Linux development machines.

For any change affecting:
- Bubble Tea `Update` or `View`;
- TUI layout;
- tab navigation;
- Launch Wizard behavior;
- option modals;
- keyboard handling;
- mouse handling;
- bottom action bar;
- runner tab rendering;

run:

```bash
cd G0LiteLLaMa && go test ./...
bun run e2e:tui
```

For any visible TUI edit, capture and show a fresh screenshot of the rendered
terminal before finishing, and commit the intentional change at the end after
verification passes.

Direct `Model.Update` tests are necessary but not sufficient for TUI behavior.
Rendered terminal behavior must be verified through `@microsoft/tui-test`
unless the change is provably unrelated to visible or interactive TUI behavior.
Every visible TUI action added or changed must be mouse-compatible as well as
keyboard-accessible. Cover mouse hit behavior with focused Go tests and rendered
`@microsoft/tui-test` coverage when the visible terminal behavior changes.

Do not claim TUI behavior is verified from:

- raw process exit code alone;
- terminal scrollback;
- `tmux`;
- Ghostty screenshots alone;
- direct `Model.Update` tests alone;
- manual inspection alone.

Windows usage/builds remain supported, but Windows development of Bun-based TUI
E2E is not required.

## Required First Steps

1. Confirm repository state with `git rev-parse --is-inside-work-tree`, then
   inspect the current git status with `git status --short`.
2. Read the relevant README and `G0LiteLLaMa/README.md` before edits.
3. Preserve user changes. Do not reset, clean, stash, or discard work unless the
   user explicitly asks.
4. Keep model files, generated release binaries, dependency folders, build
   outputs, caches, and local logs out of Git.

## Model And Artifact Rules

- Never add Git LFS to this repository.
- Never commit `.litertlm` files, `.litertlm.parts/`, partial downloads, GGUF
  model files, or model caches.
- Keep Hugging Face auth env-only (`HF_TOKEN` or `HUGGING_FACE_HUB_TOKEN`).
  Never commit tokens, print tokens, or place them in repo-local config.
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
- If an active plan exists for the requested work, read it first, follow its
  scope, and do not add unrelated improvements without asking.
- For large numbered implementation plans, use sequential subagents only when
  delegating, review each result in the parent thread, and make one commit per
  completed goal when the user asks for committed goal slices.
- Do not create worktrees unless the user explicitly asks.
- Before editing, identify whether a file is source, generated output,
  dependency material, model artifact, cache, reference material, or user-owned
  scratch.
- Do not modify dependencies, generated files, build outputs, caches, vendored
  code, or reference snapshots unless the user explicitly asks or the generation
  step is part of the task.
- Do not add new dependencies until existing project options have been checked
  and the new dependency is justified.
- Do not leave placeholder implementations, fake data, `TODO` code, stubs, or
  unimplemented branches unless the user explicitly asks.
- Use real failure paths for filesystem access, network calls, parsing, browser
  automation, process management, and external tools.
- Use Bun for the JS/TUI harness in this repo; do not switch to npm, yarn, or
  pnpm unless the user explicitly asks.
- Update `README.md` and this file when setup, commands, structure, or model
  policy changes.
- Treat renames and path moves as whole-repo migrations: update docs, scripts,
  tests, ignore rules, and run a repo-wide `rg` sweep for stale paths.
- When changing runtime downloads, package versions, or hard-coded checksums,
  verify current upstream versions from the package manager, registry, release
  page, or official docs before choosing values.
- Keep the Charm TUI stack on v2 imports:
  `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, and
  `charm.land/lipgloss/v2`. Do not reintroduce v1 imports.
- For configure/E2E runtime probes, keep llama.cpp reasoning off unless the
  task is explicitly testing reasoning. Report slow behavior instead of hiding
  it with arbitrary tiny token caps.
- Use exact commands for verification evidence.
- If verification cannot run, state exactly what was skipped and why.
- Clean temporary debug artifacts, screenshots, logs, traces, and scratch
  outputs before committing unless they are deliberate deliverables or required
  test fixtures.

## Entrypoint And Instruction Precedence

- Primary entrypoint: `AGENTS.md`; no tracked mirrors.
- Subtree overrides: none currently. Add a closer `AGENTS.md` only when a
  subtree needs materially different rules.
- If another agent instruction file is added, make it a symlink, generated
  mirror, or short pointer to this file, and document the drift check.
- Use this priority order when instructions conflict:

  1. The user's explicit prompt.
  2. The nearest applicable `AGENTS.md`.
  3. Project docs such as `README.md` and `G0LiteLLaMa/README.md`.
  4. Installed skills, tools, and tool-specific workflows.
  5. Generic model defaults.

Use installed skills and tools only when they materially help the current task
or are explicitly requested. Do not let broad process skills expand a small task
into unrelated plans, worktrees, subagents, or commits.

When subagents are used, the main agent owns scope, review, verification, and
commits. Subagents should not bootstrap the workspace, manage plans, dispatch
other subagents, or commit unless their prompt explicitly asks.

## Git Rules

- Commit only intentional source/doc changes.
- Do not stage or commit unrelated user changes.
- Do not commit local models or generated artifacts.
- Use `clean.sh` or `clean.ps1` for generated-file cleanup; they preserve
  `models/`.
- Do not force-push, rewrite history, run `git clean`, or delete untracked user
  files unless the user explicitly asks.
- If this workspace is ever not inside a git repository, ask before running
  `git init`, creating a remote, or pushing anything.

## Pending Plans

- None.
