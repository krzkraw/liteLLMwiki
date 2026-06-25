---
id: settings-api-persistence
title: Settings API and persisted app preferences
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - git diff --check
---

# Settings API And Persistence

## Grill Gate

- Decision: settings live in SQLite-backed app state, not ad-hoc config files.
- Decision: expose settings through the native action bus first.
- Decision: keep the first settings set small and real.

## Goal

Replace `/g0litellama/v1/settings` returning `{}` with real persisted settings
for API clients and the TUI.

## Scope

This slice owns settings schema, read projection, write actions, persistence,
and tests. It does not own runner/model state projections unless needed for
settings defaults.

## Acceptance Criteria

- Define a versioned settings state shape with stable JSON keys.
- Include at least these settings if already represented in the TUI:
  - chat defaults: target role, stream on/off, thinking on/off
  - wizard defaults: runtime, backend, role, option overrides
  - UI preferences that are genuinely persisted today or should survive restart
- Add settings read response for `GET /g0litellama/v1/settings`.
- Add native action type(s) for settings updates.
- Persist settings through the existing SQLite store.
- TUI reads initial settings from the shared store instead of local-only
  defaults where practical.
- Invalid settings updates return structured action API errors.
- Add tests for read, update, persistence/replay, and invalid payloads.
- Update `API.md` for settings schema and action types.

## Out Of Scope

- User profiles.
- Secrets storage; Hugging Face tokens must remain env/request-only unless a
  separate explicit spec changes that policy.
- Web UI settings screen.
