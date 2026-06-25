---
id: tui-component-state-architecture
title: TUI component tree and state architecture
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
  - test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
  - git diff --check
---

# TUI Component Tree And State Architecture

## Grill Gate

- Decision: keep Bubble Tea's Elm loop, but stop treating the whole TUI model
  as one giant component.
- Decision: global store remains the source of durable state; components keep
  only display-local state.
- Decision: mouse behavior is first-class for new visible controls.

## Goal

Introduce a minimal component tree pattern for the TUI so views, layout,
updates, and hit testing are easier to extend without destabilizing the whole
screen.

## Scope

This slice owns architecture extraction and one or two concrete component
migrations. It should not redesign every tab at once.

## Acceptance Criteria

- Define a small component contract for update, view, layout bounds, and hit
  testing using existing Bubble Tea/Bubbles/Lip Gloss patterns.
- Add selectors/helpers for reading global store slices without duplicating
  state logic in components.
- Components may keep local UI-only state, but durable data must come from the
  shared store.
- Migrate a narrow visible area to prove the pattern, preferably Chat shell or
  runner/detail panels.
- Mouse click and wheel routing goes through component hit testing where the
  migrated component owns the area.
- Resize handling recomputes component layout bounds and does not rely on stale
  cached terminal dimensions.
- Existing TUI behavior remains covered by Go tests and rendered E2E.
- Capture a fresh screenshot for visible changes and delete temporary screenshot
  tests before finishing.
- Add short code comments only where the component boundary would otherwise be
  unclear.

## Out Of Scope

- Full TUI rewrite.
- New dependencies.
- Animation framework adoption beyond what is already available.
