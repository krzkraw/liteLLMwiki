---
id: task-engine-action-execution
title: Task engine and action execution lifecycle
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh
  - git diff --check
---

# Task Engine And Action Execution Lifecycle

## Grill Gate

- Decision: actions are the public command layer; tasks are execution records
  produced by accepted actions.
- Decision: tasks may form trees, but only where work actually has subtasks.
- Decision: task state must map cleanly to store paths for progress tracking.

## Goal

Turn the current task structs and `/tasks/{id}` projection into a real execution
model: action accepted, task created, progress recorded, cancellation supported,
state updated, and API clients able to observe the lifecycle.

## Scope

This slice owns task creation, task events, progress, cancellation, and action
to task wiring for native actions. It should implement the smallest useful task
set, then leave more task kinds for later specs.

## Acceptance Criteria

- Add task JSON tags so `/g0litellama/v1/tasks/{id}` has stable lower-camel
  wire keys.
- Add task IDs to async action responses when an action schedules work.
- Model task parent/child relationships for multi-step work.
- Task records include `statePath`, `kind`, `status`, progress, summary,
  error, events, `createdAt`, and `updatedAt`.
- Add cancellation support for running tasks exposed through the native action
  layer.
- Implement at least one real task kind end-to-end, preferably model download
  or runner start, with progress/events persisted through the store.
- `/g0litellama/v1/tasks/{id}` returns live task state and not-found errors.
- Events stream emits task progress changes as committed store events.
- Add tests for task create, progress, completion, failure, cancellation, and
  replay.
- Update `API.md` for task schema and action response changes.

## Out Of Scope

- Full workflow scheduler.
- Cross-process distributed workers.
- RESTful task endpoints beyond the action-oriented layer.
