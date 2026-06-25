---
id: native-api-contract-polish
title: Native API contract polish before client expansion
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh
  - git diff --check
---

# Native API Contract Polish

## Grill Gate

- Decision: fix rough native API contract edges before generating Swagger.
- Decision: keep `/g0litellama/v1/*` action-oriented; do not pretend it is
  REST in this slice.

## Goal

Make the native API stable enough for generated docs and external clients by
cleaning up method handling, CORS, JSON shapes, and WebSocket route coverage.

## Scope

This slice owns native API contract consistency. It should prefer small handler
fixes over new frameworks or codegen.

## Acceptance Criteria

- All native read handlers reject unsupported methods with documented errors.
- CORS advertises every supported native method, including `PATCH`.
- Task and event response structs use stable JSON tags instead of exported Go
  field names.
- Structured action API errors are used consistently for action/state/event/
  task/settings routes.
- Direct runner/model/multimodal text errors are either documented as legacy or
  migrated to a consistent JSON error envelope.
- WebSocket `api.request` either supports all native HTTP endpoints or `API.md`
  explicitly documents unsupported native paths with tests.
- Add tests for method rejection, CORS methods, JSON key casing, and WebSocket
  native route coverage.
- Update `API.md` with the final error and WebSocket support contract.

## Out Of Scope

- OpenAI `/v1/*` compatibility expansion.
- OpenAPI generation.
- RESTful shim routes.
