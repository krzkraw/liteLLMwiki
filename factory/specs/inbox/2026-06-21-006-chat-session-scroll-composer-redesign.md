---
id: chat-session-scroll-composer-redesign
title: Chat sessions, real scrollbar, and sticky composer
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
  - test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
  - git diff --check
---

# Chat Sessions, Real Scrollbar, And Sticky Composer

## Goal

Rewrite the Chat page on top of the store and layout primitives so chat behaves
like a modern app: sessions, sticky composer, real scrollbar, streaming
updates, mouse-first controls, and robust long-text handling.

## Scope

This slice owns Chat page behavior and visuals. It depends on the store/bus and
layout primitive slices.

## Acceptance Criteria

- Chat state is session-based with `ActiveSessionID` and a sessions map.
- `/new` creates or switches to a fresh session-ready state.
- Future session tabs are supported by state shape, even if only one session is
  visible in this slice.
- The chat composer is always visible and sticky at the bottom of the Chat tab.
- The message area consumes remaining height above the composer.
- Long streaming responses do not push the composer off-screen.
- The message area has a real right-side scrollbar with track and thumb.
- Mouse wheel scrolls the message area.
- Scrollbar track/thumb clicks change scroll position.
- Auto-scroll remains pinned to bottom while the user is at bottom.
- Streaming does not yank the viewport when the user manually scrolls away.
- Chat settings values are wrapped/clipped and cannot corrupt the message area.
- System prompt editing uses bounded multiline `TextAreaField` or `Popover`.
- Right-click context menu exists for chat messages with at least:
  - inspect task
  - retry
  - copy if clipboard support is already cheap; otherwise omit copy.
- Hover is best-effort only; tests do not depend on hover.
- Rendered TUI E2E covers wheel or equivalent scroll behavior, sticky composer,
  long response visibility, and bounded settings editor.
- Capture a fresh screenshot and delete temporary screenshot tests before
  finishing.

## Notes

Do not route `/v1/*` through native chat commands. Chat sessions can observe
API traffic, but the OpenAI proxy remains transparent.
