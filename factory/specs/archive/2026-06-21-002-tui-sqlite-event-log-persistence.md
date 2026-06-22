---
id: tui-sqlite-event-log-persistence
title: SQLite event log and app persistence
status: archived
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - git diff --check
---

# SQLite Event Log And App Persistence

## Goal

Add SQLite as the default local persistence backend for actions, events, tasks,
snapshots, and settings behind interfaces. The reducers must not know SQLite
exists.

## Scope

Implement persistence for the store foundation. Do not migrate old WIP local
state. Do not store model binaries, runtime folders, release artifacts, or big
logs in SQLite.

## Acceptance Criteria

- Verify current SQLite driver options before choosing one. Prefer a pure Go
  driver if it supports this app's release targets well enough.
- Add an app-data database path outside the repo:
  - macOS default: `~/Library/Application Support/G0LiteLLaMa/g0litellama.db`
  - include an env or flag override for tests and portable/debug runs.
- Add schema and tests for:
  - `actions`
  - `events`
  - `tasks`
  - `snapshots`
  - `settings`
- Add a SQLite implementation behind `EventLog` and `SnapshotStore`
  interfaces.
- Add replay tests:
  - load latest snapshot
  - apply events after snapshot revision
  - rebuild `AppState`
- Add tests that task state persists parent/child relationships, status,
  progress JSON, errors, and state paths.
- Reset/no-migration behavior is explicit in code or docs.
- Existing runtime/model artifact rules remain unchanged.
- No visible TUI behavior changes in this slice.

## Notes

Keep persistence schema small. Add tables only when this slice needs them.
