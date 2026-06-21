---
id: clickable-streaming-chat-tui
title: Clickable streaming Chat tab TUI
status: accepted
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
  - test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
  - git diff --check
---

# Clickable Streaming Chat TUI

## Goal

Redesign the Chat tab so it behaves like a clickable, streaming chat surface
instead of a keyboard-hint dashboard. The chat input must be safe for normal
text entry: printable characters are text, not hidden commands.

## Scope

This spec covers the Chat tab in `G0LiteLLaMa/internal/tui/model.go` and its
tests. It does not redesign Dashboard, Launch Wizard, Setup, runner tabs, API
routes, runtime startup, or model installation. The broader "whole TUI must be
clickable" direction remains valid, but this implementation slice makes the
Chat tab fully mouse-operable first.

## Grill Gate

- `Enter` sends the chat prompt.
- `Shift+Enter` inserts a newline in the chat input.
- Printable punctuation such as `?`, `!`, `@`, `[`, `]`, and `\` inserts text
  instead of triggering hidden commands.
- Slash commands are allowed only through a popup opened by leading `/` in an
  empty input.
- Keep the current target role choices `main`, `embedding`, and `reranking`;
  chat-capable target rules can be refined later.

## Current Problems

- Route, model, target, thinking, settings, transcript, and input are rendered
  as large panels that waste vertical space.
- The route and model are displayed as box content even though they are status,
  not controls.
- `?`, `!`, `@`, `[`, `]`, and `\` are chat commands, which breaks natural text
  entry.
- System prompt editing is mixed into the bottom prompt composer.
- The transcript has a title and scroll header instead of being a direct chat
  timeline.
- The TUI sends a request and renders one final response instead of streaming
  visible assistant output.
- Mouse behavior exists for some old controls, but the chat surface is not
  click-first.

## Acceptance Criteria

- The first Chat-tab row is plain text, not a panel, and shows route, model,
  status, and tokens/sec.
- Route and model are display-only in this implementation slice.
- The only top box is `Prompt settings`.
- `Prompt settings` has clickable fields for `Thinking`, `Target`, `System`,
  `Temperature`, `Top P`, `Max Tokens`, and `Stream`.
- Each prompt setting opens a dropdown or popup anchored under the clicked
  field.
- Prompt setting popups look like the existing global menu, open near the
  clicked field, keep keyboard focus until closed, and disappear on outside
  click.
- `System` opens a small multiline editor popup and does not reuse the bottom
  chat input.
- Custom numeric prompt values keep keyboard focus in the popup while typing and
  do not leak typed text into the chat input.
- The chat history is a boxed scrollable region with mouse wheel support and
  visible scroll buttons.
- The middle chat window has messages only: no `Transcript` title, no scroll
  header, and no per-message role headers.
- Assistant and error messages stick to the left side; user messages stick to
  the right side, messenger-style. Message types remain visually separated by
  color.
- The input is a boxed region stuck to the bottom of the Chat tab. It stays slim
  when empty and grows only when multiline input needs more rows.
- Assistant output streams into the active assistant message in place.
- The chat window auto-scrolls while pinned to bottom and does not yank the
  viewport when the user manually scrolls away.
- The bottom input is a real multiline text input with placeholder
  `Ready. Input your prompt` when idle.
- The bottom input shows `Wait...` while a request is busy.
- A clickable send button sends the prompt and is disabled while waiting.
- `Enter` sends; `Shift+Enter` inserts newline.
- Printable punctuation inserts text in chat input.
- Leading `/` in an empty input opens a clickable command popup.
- `/` elsewhere is normal text.
- Initial commands are `/clear`, `/stop`, `/new`, and `/settings`.
- No extra keyboard shortcuts are required.
- `/stop` cancels streaming, keeps partial assistant text, and returns to
  `idle`.
- No selected runner or HTTP error leaves the draft in the input and sets status
  to `error`.
- Stream/decode error after partial response keeps partial assistant text and
  appends a compact error marker.
- No dependencies are added.
- Charm v2 imports remain in use.

## Verification Requirements

Run the frontmatter verification commands after implementation. For visible TUI
changes, use `skills/tui-model-screenshot/SKILL.md` to capture a fresh rendered
model screenshot, and keep `bun run e2e:tui` as behavior verification.
