# Sidecar TUI Basics Design

## Overview

The TUI is reduced to a native-runner control surface. It keeps the HTTP and
WebSocket server available, but the interactive terminal no longer starts a
default runner or centers the web UI workflow.

## Components

- `tui.Model`: owns tab selection, mouse handling, wizard selection state,
  dashboard model dropdown state, and the global menu state.
- `Supervisor`: keeps legacy default-runner behavior by default, with an opt-out
  flag for interactive TUI startup.
- `cmd/litert-sidecar`: chooses lazy startup for TUI mode and legacy autostart
  for headless mode.

## Rendering

The shell has three fixed regions: status header, tab bar, and bottom action
bar. The body shows one tab at a time and uses the shared responsive panel grid:
narrow terminals render one full-width stack, while wide terminals render two
masonry-balanced columns. Dashboard renders one status panel unless a model role
dropdown is open, in which case the dropdown becomes a second panel. Launch
Wizard renders choices and local models as two responsive sections. Runner tabs
render runner details and route/control actions as two responsive sections.

## Interaction

Bubble Tea runs with alternate screen and mouse cell-motion support. Mouse
clicks route to tabs, bottom F1 menu, dashboard model role dropdowns, wizard
runtime/variant/role/model controls, START, and runner tab bottom-bar actions.
Keyboard navigation keeps Tab, Shift+Tab, number keys, Enter, F1, and runner
start/stop/restart keys.

## Runner Creation

The wizard filters local catalog entries by runtime and role. llama.cpp variant
groups map to installed runtime folders under `native/llama-runtimes`. START
builds a `server.RunnerSpec`, creates the runner through `RunnerController`,
then starts it through the same controller. Runner IDs and tab labels use
`LR`/`LM` plus `M`/`E`/`R` and a role-scoped number.

## Out Of Scope

The web UI, chat tab, model download tab, logs tab, settings tab, and rich
diagnostic panels are intentionally out of scope for this simplification pass.
