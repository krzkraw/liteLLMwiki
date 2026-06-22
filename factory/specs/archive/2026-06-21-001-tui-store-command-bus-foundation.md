---
id: tui-store-command-bus-foundation
title: TUI store and command bus foundation
status: active
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - git diff --check
---

# TUI Store And Command Bus Foundation

## Goal

Introduce the store/action/task foundation described in
`docs/superpowers/specs/2026-06-21-tui-state-layout-redesign-design.md`
without changing visible TUI behavior.

## Scope

Create internal state/action/task primitives and route one small existing TUI
path through the new command bus. This is a foundation slice only.

## Acceptance Criteria

- Add focused files under `G0LiteLLaMa/internal/tui/store/` or an equivalent
  package-local path for:
  - `AppState`
  - `StateRevision`
  - `ActionEnvelope`
  - `ActionID`
  - `ActionType`
  - `ActionSource`
  - `CommandBus`
  - reducer/effect interfaces
  - in-memory event log
  - task state types
- The command bus appends every accepted action and increments state revision
  atomically.
- Reducers are deterministic and side-effect-free.
- Effects are represented as scheduled work, not run inside reducers.
- Add action replay tests proving that the same ordered action envelopes rebuild
  the same logical state.
- Route one low-risk TUI interaction through the bus. Prefer tab selection or
  focus selection; do not rewrite chat or wizard in this slice.
- Existing visible TUI behavior remains unchanged.
- No SQLite dependency is added in this slice.
- No Harmonica dependency is added in this slice.
- Charm v2 imports remain in use.

## Grill Gate

Decisions recorded before dispatch:

- **Tab state ownership**: `AppState.UI.ActiveTab` is the source of truth.
  `setActiveTab` dispatches a `SelectTab` action through the CommandBus.
  The Model reads `AppState.UI.ActiveTab` instead of `m.active` for
  rendering.
- **AppState shape**: Define all substate types from the design doc
  (Viewport, Runners, Models, Runtime, Chat, Wizard, Tasks, UI) even if
  as empty structs. Reducers for unused domains return state unchanged.
  This keeps the state shape stable across slices.

## Notes

Keep this boring. This slice exists to make future slices easier, not to
redesign screens.
