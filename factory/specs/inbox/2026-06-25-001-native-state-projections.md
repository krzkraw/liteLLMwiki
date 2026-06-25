---
id: native-state-projections
title: Native state projections for runners, models, runtime, and chat
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh
  - git diff --check
---

# Native State Projections

## Grill Gate

- Decision: split remaining work into ordered Loop Factory specs, not one giant spec.
- Decision: native `/g0litellama/v1/*` remains action-oriented and store-backed.
- Decision: TUI remains an in-process bus client; it must not call HTTP for local state.

## Goal

Make `/g0litellama/v1/state` useful to API clients by replacing schema-only
state slices with committed projections for runners, models, runtime, chat, UI,
and wizard state.

## Scope

This slice owns read-model state shape and projection wiring. It does not add
task execution, settings mutation, REST shims, or OpenAPI generation.

## Acceptance Criteria

- `AppState.Runners` exposes runner snapshots and route mapping equivalent to
  the direct runner controller snapshot.
- `AppState.Models` exposes model catalog entries with current model state.
- `AppState.Runtime` exposes current runtime status when available.
- `/g0litellama/v1/state/runners` returns the real runner projection, not `{}`.
- `/g0litellama/v1/state/models` returns the real model projection, not `{}`.
- `/g0litellama/v1/state` includes the same real projections.
- Projection updates are dispatched through the command bus or a clearly
  documented projection refresh path; handlers must not return stale ad-hoc
  globals while the store says something else.
- API-observed chat sessions remain separate from active local TUI sessions.
- Add tests for state projection updates after runner/model/runtime changes.
- Update `API.md` for any state schema changes.

## Out Of Scope

- Task lifecycle and cancellation.
- Settings writes.
- RESTful shim endpoints.
- Swagger/OpenAPI generation.
