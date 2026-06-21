# Clickable Streaming Chat TUI Design

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
- `?`, `!`, `@`, `[`, `]`, and `\` are chat commands, which breaks natural
  text entry.
- System prompt editing is mixed into the bottom prompt composer.
- The transcript has a title and scroll header instead of being a direct chat
  timeline.
- The TUI sends a request and renders one final response instead of streaming
  visible assistant output.
- Mouse behavior exists for some old controls, but the chat surface is not
  click-first.

## Target Layout

The Chat tab uses three vertical zones.

### Plain Status Row

The first Chat-tab row is plain text, not a panel:

```text
/v1/chat/completions | gemma4-litert | idle | 0 tok/s
```

The row shows:

- Route: the effective OpenAI-compatible route for the selected target.
- Model: the selected runner model ID or `not configured`.
- Status: `idle`, `processing`, `responding`, or `error`.
- Throughput: live tokens per second during streaming, `0 tok/s` when idle.

Route and model are not clickable in this spec.

### Prompt Settings Box

The only box at the top is `Prompt settings`. It is a compact clickable
toolbar/form. The first row contains fields next to each other:

```text
Thinking: on  Target: main / LR-M-1  System: empty  Temp: default  Top P: default  Max: default  Stream: on
```

Every field is clickable and opens a dropdown or popup anchored under that
field, like an old Windows Start menu.

- `Thinking`: dropdown with `on`, `off`.
- `Target`: dropdown with the existing role choices, currently `main`,
  `embedding`, and `reranking`; keep this model even though chat-capable target
  rules may be refined later.
- `System`: opens a small multiline editor popup. It does not reuse the bottom
  chat input.
- `Temperature`: presets plus `custom...`.
- `Top P`: presets plus `custom...`.
- `Max Tokens`: presets plus `custom...`.
- `Stream`: dropdown with `on`, `off`; default is `on`.

Custom numeric editors validate input before applying changes. Invalid values
keep the popup open and show a compact inline error in the popup.

### Scrollable Chat Window

The middle zone fills remaining height with messages only.

- No `Transcript` title.
- No `Scroll 0/0` header.
- No per-message role headers.
- User and assistant messages are visually separated by color.
- Error messages use a distinct error color.
- The window scrolls with mouse wheel and keyboard navigation.
- New assistant chunks live-update in place while streaming.
- The view auto-scrolls while pinned to the bottom.
- If the user manually scrolls away from the bottom, streaming does not yank the
  viewport until the user scrolls back to bottom or sends a new prompt.

### Bottom Chat Input

The bottom zone is a real multiline text input with no header.

- Empty ready placeholder: `Ready. Input your prompt`.
- Busy placeholder: `Wait...`.
- `Enter` sends.
- `Shift+Enter` inserts a newline.
- Mouse click focuses the input and places the cursor as close as the terminal
  event model allows.
- The input scrolls when the draft exceeds visible rows.
- A small clickable send button sits at the right edge of the input box.
- The send button is disabled while waiting for a response.

System prompt editing is not available in the bottom input.

## Command Popup

Slash commands are allowed only through an explicit popup flow.

- Typing `/` at the start of an empty input opens a command popup anchored above
  the input.
- The popup filters as the user types.
- `/` anywhere else is normal text, including URLs and prose.
- `Enter` while a command is selected runs that command instead of sending the
  text to the model.
- `Esc` closes the popup and leaves typed text in the input.
- Commands are clickable.
- No single printable key outside this popup triggers chat actions.

Initial commands:

- `/clear`: clear chat messages.
- `/stop`: cancel the current streaming response.
- `/new`: clear draft and start a fresh chat context.
- `/settings`: focus prompt settings.

Do not add clipboard commands unless existing clipboard support already makes
that trivial.

## Input And Keyboard Rules

Chat input is text-first.

- Printable keys always insert text unless the slash command popup is active at
  the start of the input.
- `?`, `!`, `@`, `[`, `]`, and `\` must no longer be Chat-tab commands.
- `Enter` sends the prompt.
- `Shift+Enter` inserts a newline.
- `Esc` closes the active dropdown or command popup first.
- If no popup is open, `Esc` keeps existing app-level quit/back behavior.
- Mouse behavior must cover every visible chat control.

No extra keyboard shortcuts are required for this spec.

## Streaming State

Chat has four user-visible states.

- `idle`: input enabled, send clickable, `0 tok/s`.
- `processing`: request created, waiting for the first response chunk.
- `responding`: chunks are streaming into the active assistant message.
- `error`: the last request failed.

Streaming behavior:

- Create the assistant message as soon as the first assistant chunk arrives.
- Append chunks to the active assistant message in place.
- Switch from `processing` to `responding` on the first chunk.
- Compute tokens/sec from received text or chunks with the simplest reliable
  local estimate. This can be an approximation; it is a UI throughput signal,
  not billing data.
- Return to `idle` when the stream ends or is stopped.

## Error Behavior

- No selected runner: status becomes `error`, the draft remains in the input.
- HTTP error before response: status becomes `error`, and the draft remains in
  the input.
- Stream/decode error after partial response: keep partial assistant text and
  append a compact error marker to that assistant message.
- `/stop`: cancels the stream, keeps the partial assistant message, and returns
  to `idle`.

## Implementation Boundaries

- Keep the existing Bubble Tea and lipgloss stack.
- Keep Charm v2 imports.
- Prefer focused changes inside the existing TUI model before splitting files.
- Do not add dependencies.
- Do not change API route semantics beyond what streaming in the TUI requires.
- Preserve existing runner role selection for `main`, `embedding`, and
  `reranking`.

## Verification Requirements

Run from the repository root unless noted.

Required Go tests:

- Chat render has no `Transcript` title.
- Chat render has no boxed route/model panel.
- Prompt settings box exists and contains the clickable fields.
- Bottom input placeholder renders `Ready. Input your prompt` when idle.
- Busy input placeholder renders `Wait...`.
- `?`, `!`, `@`, `[`, `]`, and `\` insert text in chat input.
- Leading `/` opens the command popup.
- `/` in normal prose or URLs inserts text.
- `Enter` sends.
- `Shift+Enter` inserts a newline.
- Mouse click opens at least one prompt setting dropdown.
- Mouse click focuses the input.
- Mouse click on send sends the prompt.
- Chat window scroll is internal and does not move the global terminal scroll.

Required rendered TUI E2E:

- The Chat tab renders the new layout without the old transcript/header waste.
- At least one prompt settings dropdown can be opened with the mouse.
- A normal prompt can be typed with punctuation and sent.

Required final verification for implementation:

```bash
cd G0LiteLLaMa && go test ./...
bun run e2e:tui
```

Because this is visible TUI work, capture and show a fresh rendered terminal
screenshot before finishing implementation.

For this docs-only spec commit, `git diff --check` is sufficient.
