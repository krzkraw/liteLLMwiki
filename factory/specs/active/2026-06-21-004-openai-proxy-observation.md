---
id: openai-proxy-observation
title: OpenAI proxy observation into action store
status: active
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh
  - git diff --check
---

# OpenAI Proxy Observation Into Action Store

## Goal

Keep `/v1/*` as a transparent OpenAI-compatible proxy while mirroring observed
traffic into the store, task log, and event stream for TUI/API debugging.

## Scope

Observe requests and responses for pinned runner slots: main, embedding, and
reranking. Secondary runners are not important in this slice.

## Acceptance Criteria

- `/v1/*` request and response semantics remain OpenAI-compatible.
- `/v1/chat/completions` streaming still returns OpenAI-style SSE chunks.
- The proxy dispatches observed actions for request start, response chunks,
  completion, and errors.
- Observed chat sessions are auto-created with `source=api`.
- Observed API sessions do not replace the TUI active local session by default.
- Observed events include correlation IDs tying request, task, stream chunks,
  and completion/error together.
- Store observation must not delay or mutate the proxied response beyond
  minimal bookkeeping.
- Add tests that compare proxied response compatibility and observed store
  events.
- Preserve the current pinned slot behavior for main, embedding, and reranking.

## Notes

The proxy response is the contract for `/v1/*`. Store observation is internal
debugging and state tracking.
