---
id: wizard-cli-editor-redesign
title: Wizard CLI option editor redesign
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
  - test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
  - git diff --check
---

# Wizard CLI Option Editor Redesign

## Goal

Replace the current fragile CLI option text popup with bounded, responsive,
component-based editors that cannot break layout when input exceeds one line.

## Scope

This slice owns Launch Wizard CLI option editing and command preview editing.
It depends on the layout primitive slice.

## Acceptance Criteria

- CLI option editor uses a bounded `Popover` and `TextAreaField` or suitable
  bounded single-line component.
- Long input is clipped or horizontally/vertically scrollable inside the editor.
- Long input never corrupts nearby wizard columns, chat area, footer, or
  command preview.
- Enum/sample buttons remain clickable.
- Save, reset, and close remain mouse-accessible.
- Right-click context menu exists for option rows with at least:
  - reset option
  - inspect generated argv
- Command preview editing uses bounded editor behavior too.
- Managed-screen resize recomputes editor bounds.
- Tests cover a long CLI option value that previously exceeded one terminal
  line.
- Rendered TUI E2E covers long option editing and command preview stability.
- Capture a fresh screenshot and delete temporary screenshot tests before
  finishing.

## Notes

Keep runner spec generation behavior aligned with command preview. Do not add
new runner options unless required for the editor architecture.
