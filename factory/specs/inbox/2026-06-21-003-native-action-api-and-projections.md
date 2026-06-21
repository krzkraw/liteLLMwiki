---
id: native-action-api-and-projections
title: Native action API and state projections
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh
  - git diff --check
---

# Native Action API And State Projections

## Goal

Expose the store command bus over native `/g0litellama/*` HTTP routes. Native
HTTP is action-oriented transport, not REST.

## Scope

Add native action dispatch and read-only projections. Native route compatibility
can break. OpenAI `/v1/*` compatibility is not changed in this slice.

## Acceptance Criteria

- Add or replace native routes as needed:
  - `POST /g0litellama/v1/actions`
  - `GET /g0litellama/v1/state`
  - `GET /g0litellama/v1/state/chat/sessions/{id}`
  - `GET /g0litellama/v1/tasks/{id}`
  - `GET /g0litellama/v1/events?after={revision}`
  - `WS /g0litellama/v1/events` or retain an equivalent native event stream.
- HTTP handlers dispatch `ActionEnvelope` values through the same `CommandBus`
  used by the TUI.
- Command responses include action ID and state revision. Async commands also
  include task ID.
- State projection responses are generated from committed store state.
- Event streaming emits committed event/revision data, not ad-hoc task output.
- Add tests for dispatch, projection reads, event reads, and invalid action
  validation.
- TUI code must not call HTTP just to use this bus; it remains an in-process
  client.
- Existing `/v1/*` OpenAI-compatible behavior remains covered by existing
  tests.

## Notes

Do not pretend this is REST. RESTful shims can be added later on top of this
action layer.
