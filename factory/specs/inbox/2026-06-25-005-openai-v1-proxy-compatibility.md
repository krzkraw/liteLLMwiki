---
id: openai-v1-proxy-compatibility
title: OpenAI v1 proxy compatibility coverage
status: inbox
agent: codex
verification:
  - cd G0LiteLLaMa && go test ./...
  - cd G0LiteLLaMa && scripts/runtime-backend-e2e.sh
  - git diff --check
---

# OpenAI V1 Proxy Compatibility Coverage

## Grill Gate

- Decision: `/v1/*` should be as OpenAI-compatible as possible.
- Decision: the proxy remains transparent; observation must not rewrite
  request or response bodies.
- Decision: internal behavior belongs under `/g0litellama/v1/*`, not `/v1/*`.

## Goal

Add explicit compatibility coverage for OpenAI-style `/v1/*` proxy behavior,
including streaming, headers, error preservation, model listing, and role-based
routing.

## Scope

This slice owns proxy behavior and tests. It should not implement native action
commands or RESTful shims.

## Acceptance Criteria

- Document the supported `/v1/*` compatibility matrix in `API.md`.
- Verify transparent proxying for representative OpenAI endpoints:
  - `/v1/chat/completions`
  - `/v1/completions`
  - `/v1/responses` if upstream supports it or proxy transparency can be tested
  - `/v1/embeddings`
  - `/v1/rerank`
  - `/v1/models`
- `/v1/models` local aggregation remains OpenAI-shaped and tested.
- Streaming responses preserve chunks, status, and relevant headers.
- Non-streaming JSON responses preserve status, headers, and body.
- Upstream errors preserve status/body when the upstream responds; connection
  errors remain gateway errors.
- Role routing remains:
  - embeddings path to `embedding`
  - rerank path to `reranking`
  - all other `/v1/*` paths to `main`
- Proxy observation dispatches `proxy:*` actions without mutating traffic.
- Add tests for role routing, streaming, non-streaming, upstream error, and
  unavailable route behavior.

## Out Of Scope

- Implementing missing model-provider-specific semantics locally.
- Translating internal actions into OpenAI responses.
