---
id: openapi-swagger-source
title: OpenAPI and Swagger source generation
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - git diff --check
---

# OpenAPI And Swagger Source Generation

## Grill Gate

- Decision: generate OpenAPI after API contract polish, not before.
- Decision: `API.md` remains the human-readable contract; generated OpenAPI
  must not silently drift from code or docs.

## Goal

Create a maintainable OpenAPI/Swagger artifact for native `/g0litellama/v1/*`
routes and the documented `/v1/*` proxy surface.

## Scope

This slice owns the OpenAPI source artifact and a cheap drift check. It should
not add a new server framework.

## Acceptance Criteria

- Add an OpenAPI 3.x artifact under a stable path, for example
  `docs/openapi/g0litellama.openapi.yaml`.
- Cover native HTTP routes, action schemas, state schemas, task schemas,
  settings schemas, event stream notes, WebSocket message schemas, and `/v1/*`
  proxy notes.
- Include schemas for request and response bodies currently documented in
  `API.md`.
- Add a lightweight validation command using existing tooling if available; if
  no validator exists, add a minimal stdlib check for YAML/JSON shape only.
- Add a documented update workflow in `API.md` or a small docs section.
- `AGENTS.md` must mention updating generated OpenAPI when API surface changes,
  if generation is not fully automatic.
- Add tests or checks that fail when the OpenAPI artifact is missing after API
  docs change, if cheap.

## Out Of Scope

- Hosting Swagger UI.
- Replacing handlers with generated server code.
- Full automatic extraction from Go comments.
