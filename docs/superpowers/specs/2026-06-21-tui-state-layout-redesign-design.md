# TUI State, Layout, And Action Redesign

## Goal

Make G0LiteLLaMa's TUI robust, responsive, mouse-first, replayable, and
extendable by moving from one giant Bubble Tea model to a store-driven
component architecture. The same command/action layer must serve the TUI,
native HTTP API, internal tasks, and future API clients.

## Current Problems

- `G0LiteLLaMa/internal/tui/model.go` mixes state, actions, task effects,
  layout, rendering, hit testing, popovers, streaming, and API interaction in
  one file.
- UI controls are located by searching rendered text, so responsive layout and
  hit testing drift apart.
- Popovers overlay rendered strings without hard clipping, so long CLI or
  system prompt values corrupt neighboring panels.
- Chat scrolling uses text markers instead of a real scrollbar and does not
  provide reliable mouse-first behavior.
- TUI state changes cannot be replayed as a clean action log.
- Native API and TUI behavior can drift because they are not clients of the
  same command bus.

## Architecture Decisions

- Use Bubble Tea as the runtime and Elm-style update loop.
- Add Redux-style typed action envelopes, reducers, substores, selectors, and
  effects where they help.
- Treat the TUI as an in-process client of the same command bus used by the
  native HTTP API and internal task runner.
- Keep `/v1/*` as a transparent OpenAI-compatible proxy. It may observe and
  mirror traffic into the store, but it must preserve OpenAI request/response
  behavior.
- Treat `/g0litellama/*` as an action-oriented internal API. It does not need
  to imitate REST. RESTful shims can be added later on top of the store.
- Use SQLite as the default local persistence backend for all app persistence.
  It must live in the user app data directory, not the repo. Add an override
  path for tests and portable/debug runs.
- No compatibility or migration is required for current WIP local UI state.
  Model binaries, runtime installs, build outputs, and large artifacts remain
  external files and must not be stored in SQLite.
- Harmonica is part of the target stack for animation readiness. The first
  implementation should not depend on animation for correctness. Verify current
  package/version before adding it.

## State Model

The app owns one central store with domain and UI substores.

```go
type AppState struct {
	Revision StateRevision
	Viewport ViewportState
	Runners  RunnersState
	Models   ModelsState
	Runtime  RuntimeState
	Chat     ChatState
	Wizard   WizardState
	Tasks    TaskState
	UI       UIState
}
```

State that affects behavior or replay belongs in the store. That includes
active tab, focus, popovers, scroll offsets, drafts, selected rows, active
session, prompt settings, and active tasks.

Pure render cache may remain transient. Examples: latest measured component
bounds, cached rendered strings, and best-effort hover highlights.

## Action Envelope

Every command or event uses a durable envelope.

```go
type ActionEnvelope struct {
	ID            ActionID
	Type          ActionType
	Source        ActionSource
	CorrelationID ActionID
	ParentID      ActionID
	Time          time.Time
	Payload       json.RawMessage
}
```

Sources include `tui`, `api`, `task`, `openai-proxy`, and `system`.

The mental model is:

```text
ActionEnvelope
  -> reducer validates against AppState
  -> atomic commit: new StateRevision + TaskSpec tree
  -> task runner executes TaskSpec
  -> task emits progress/result ActionEnvelope values
  -> reducers produce new AppState revisions
  -> TUI/API observe state changes and events
```

## Reducers And Effects

Reducers are pure and deterministic. They validate actions against current
state, return the next state, and may return task/effect specifications. They
do not call HTTP, filesystem, SQLite, subprocesses, clocks, or timers directly.

Effects own side effects. Effects dispatch follow-up action envelopes.

Reducers should be split by domain:

- `chat.Reduce`
- `wizard.Reduce`
- `tasks.Reduce`
- `runners.Reduce`
- `models.Reduce`
- `runtime.Reduce`
- `ui.Reduce`

The root reducer coordinates ordering and cross-domain validation, but it
should stay small.

## Task Model

Tasks are first-class state.

```go
type Task struct {
	ID        TaskID
	ParentID  TaskID
	Kind      TaskKind
	Status    TaskStatus
	StatePath StatePath
	Progress  TaskProgress
	Summary   string
	Error     string
	Events    []TaskEvent
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

Task trees should be logically aligned with state paths, not physically forced
to duplicate the state tree. A task can point to `chat.sessions.s1.stream`,
`runners.LR-M-1.start`, or `models.gemma4-litert.download`. State stores active
task IDs where useful.

Cancellation is action-based. A cancel action targets a task ID. The task
runner cancels the effect and dispatches task result actions.

## Persistence

SQLite is the default backend behind interfaces.

Interfaces:

```go
type EventLog interface {
	AppendAction(ActionEnvelope) error
	AppendEvents([]StoredEvent) error
	Since(StateRevision) ([]StoredEvent, error)
}

type SnapshotStore interface {
	LoadLatest() (AppState, StateRevision, error)
	Save(AppState) error
}
```

Initial tables:

```text
actions(id, type, source, correlation_id, parent_id, payload_json, created_at)
events(revision, action_id, type, payload_json, created_at)
tasks(id, parent_id, state_path, kind, status, progress_json, error, created_at, updated_at)
snapshots(revision, state_json, created_at)
settings(key, value_json, updated_at)
```

Persistence stores app state, action/event logs, task state, chat sessions,
prompt settings, runner registry/config, model metadata, download state, and
UI preferences worth preserving.

Do not store model binaries, runtime folders, release artifacts, or large logs
in SQLite unless a future spec explicitly asks for that.

## API Model

Native `/g0litellama/*` is action-oriented HTTP.

Core routes:

```text
POST /g0litellama/v1/actions
GET  /g0litellama/v1/state
GET  /g0litellama/v1/state/chat/sessions/{id}
GET  /g0litellama/v1/tasks/{id}
GET  /g0litellama/v1/events?after={revision}
WS   /g0litellama/v1/events
```

TUI dispatches to the same command bus in process. HTTP handlers dispatch to
the same bus over network transport. Internal task callbacks also dispatch to
the same bus.

`/v1/*` remains OpenAI-compatible. It resolves pinned runner slots for main,
embedding, and reranking. It proxies requests and responses as transparently as
possible. While proxying, it dispatches observed request, chunk, completion,
error, and task actions into the store. Observed `/v1/*` chat sessions are
auto-created and tagged `source=api`, but they do not replace the TUI's active
local chat session by default.

## Chat Model

Chat is session-based now, even if the first UI only shows one active session.

```go
type ChatState struct {
	ActiveSessionID ChatSessionID
	Sessions        map[ChatSessionID]ChatSession
}

type ChatSession struct {
	ID       ChatSessionID
	Source   ChatSessionSource
	Messages []Message
	Draft    string
	Settings PromptSettings
	Stream   TaskID
	Scroll   ScrollState
}
```

Future chat tabs can be built on top of this without changing the store model.

## Component Model

Extract focused components. Each component should have bounded state, layout,
view, hit testing, and update responsibilities.

Target components:

- `Shell`: header, tab bar, footer, global viewport resize.
- `ChatPage`: chat status, settings, message viewport, composer.
- `ScrollBox`: wheel, track, thumb, click, drag-ready scrollbar.
- `Popover`: anchored dropdown/editor, clipping, focus trap, outside click.
- `TextAreaField`: bounded multiline input for system prompt and long CLI
  options.
- `WizardPage`: runtime, model, option rows, command preview.
- `TaskList`: task progress, cancellation, logs.

Components use explicit rectangles. Do not locate interactive controls by
searching rendered text except as a temporary bridge in migration slices.

```go
type Rect struct {
	X, Y, W, H int
}

type Component interface {
	Layout(Rect, AppState) LayoutResult
	View(AppState) string
	HitTest(Point, MouseEventKind) (ActionEnvelope, bool)
}
```

## Layout Rules

- Resize recomputes component bounds.
- Chat composer is sticky at the bottom and always visible.
- Chat messages consume remaining height.
- Scrollable regions render a right-side scrollbar track and thumb.
- Popovers clamp inside the viewport and never corrupt content behind them.
- Long text is wrapped or clipped inside component bounds.
- No fake `[up]` or `[down]` scroll controls.
- Mouse wheel targets the component under the pointer when possible, falling
  back to the active scroll area.
- Right-click opens a bounded context popover where supported.
- Hover is best-effort only. Core behavior must not depend on hover.

## Animation

The architecture must be animation-ready. Harmonica can be used for small,
non-critical UI polish such as scrollbar thumb easing or task progress easing.
Animation must never be required for correctness, tests, accessibility, or
state consistency.

## Verification Strategy

- Reducer tests: action in, state out, deterministic replay.
- Command bus tests: dispatch action, atomic revision/event append, effects
  scheduled.
- Task tests: task tree creation, progress, cancellation, result actions.
- Persistence tests: SQLite append/replay/snapshot restore.
- Native API tests: action dispatch, state projections, events.
- OpenAI proxy tests: `/v1/*` compatibility plus observed event capture.
- Component tests: layout bounds, hit testing, clipping, scroll math.
- Rendered TUI E2E: Bun `@microsoft/tui-test` for visible mouse, wheel,
  keyboard, resize, chat composer, popovers, and wizard editor behavior.
- Screenshot evidence for visible TUI slices using
  `skills/tui-model-screenshot/SKILL.md`.

## Implementation Slices

1. `tui-store-command-bus-foundation`
2. `tui-sqlite-event-log-persistence`
3. `native-action-api-and-projections`
4. `openai-proxy-observation`
5. `tui-layout-popover-scrollbox-primitives`
6. `chat-session-scroll-composer-redesign`
7. `wizard-cli-editor-redesign`

Each slice must leave the app runnable and verified. Visible TUI slices must
include rendered E2E coverage and screenshot evidence.
