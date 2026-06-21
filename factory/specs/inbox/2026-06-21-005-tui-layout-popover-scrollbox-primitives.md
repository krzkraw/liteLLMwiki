---
id: tui-layout-popover-scrollbox-primitives
title: TUI layout, popover, and scrollbox primitives
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
  - test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
  - git diff --check
---

# TUI Layout, Popover, And ScrollBox Primitives

## Goal

Introduce reusable layout primitives that make terminal UI components bounded,
responsive, clipped, and mouse-aware.

## Scope

Add layout primitives and migrate one low-risk existing popover or scrollable
area as proof. Do not rewrite the Chat page or Wizard editor fully in this
slice.

## Acceptance Criteria

- Add explicit geometry types such as `Rect`, `Point`, and `LayoutResult`.
- Add component hit-test registration based on rectangles, not rendered text
  search.
- Add `Popover` primitive:
  - anchored to a source rect
  - clamped inside viewport
  - clipped/wrapped content
  - focus trap
  - outside click closes
  - right-click compatible for future context menus
- Add `ScrollBox` primitive:
  - right-side scrollbar track and thumb
  - mouse wheel support
  - clickable track/thumb behavior
  - no fake `[up]` or `[down]` controls
  - scroll math tests for pinned bottom and manual scroll-away
- Add bounded multiline `TextAreaField` primitive using existing Bubbles
  support where practical.
- Add tests for clipping long content so it cannot corrupt adjacent panels.
- Add rendered TUI E2E coverage for the migrated proof component.
- Capture a fresh screenshot for the visible primitive migration and delete
  temporary screenshot tests before finishing.
- Do not add Harmonica unless this slice explicitly verifies the current
  package/version and uses it only for non-critical animation readiness.

## Notes

This slice should delete ad-hoc string overlay assumptions where it touches
them. Keep the migration narrow.
