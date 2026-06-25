---
id: chat-session-tabs
title: Chat session tabs
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
  - test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
  - git diff --check
---

# Chat Session Tabs

## Grill Gate

- Decision: chat sessions become visible tabs after the store/component
  foundation is stable.
- Decision: tabs are TUI controls over store-backed sessions, not independent
  local-only buffers.

## Goal

Add first-class chat session tabs so users can create, switch, rename, and
close chat sessions without losing the sticky composer and scroll behavior.

## Scope

This slice owns Chat tab session navigation and related state/actions. It does
not own the whole TUI component architecture unless that prior spec is not yet
implemented.

## Acceptance Criteria

- Chat state supports multiple sessions with stable IDs, labels, timestamps,
  active session ID, and messages.
- Add actions for new session, select session, rename session, and close
  session.
- The Chat screen renders session tabs or a compact tab strip that remains
  usable at narrow terminal widths.
- Mouse click selects a session tab.
- Keyboard navigation supports creating and switching sessions.
- Closing the active session selects a predictable neighbor or creates a blank
  session when none remain.
- Each session preserves its own scroll position and composer draft where
  appropriate.
- Streaming response state stays attached to the originating session.
- API-observed sessions do not steal focus from the active local session unless
  the user selects them.
- Rendered TUI E2E covers create, switch, close, and sticky composer behavior.
- Capture a fresh screenshot and delete temporary screenshot tests before
  finishing.
- Update `API.md` if chat action/state schemas change.

## Out Of Scope

- Cloud sync.
- Search across sessions.
- Rich transcript export.
