# llm-agent-memory-gateway Module Design

> Date: 2026-05-26
> Scope: define the role and boundary of the `llm-agent-memory-gateway` module before any HTTP code is added

## Goal

Define the future M6 gateway module as the service-composition layer that exposes
durable memory over HTTP while depending on:

- SDK-owned abstractions from `llm-agent-memory`
- concrete durable backend implementations such as `llm-agent-memory-postgres`

This design intentionally does **not** implement HTTP handlers yet.

## Role

`llm-agent-memory-gateway` is the first module that is allowed to own:

- HTTP routing
- auth and tenant binding
- request validation and error mapping
- runtime configuration and process startup
- backend composition and dependency injection

It is not allowed to own:

- SDK-core abstractions
- backend-specific SQL schema or storage logic
- duplicate durable-memory contracts

## Dependency Model

Allowed dependencies:

- `llm-agent-memory-gateway` -> `llm-agent-memory`
- `llm-agent-memory-gateway` -> `llm-agent-memory-postgres`

Expected composition:

- SDK provides neutral durable-memory interfaces and domain models
- Postgres module provides one concrete implementation
- Gateway wires auth + transport + backend together

## Future M6 Responsibilities

The module will later own:

- `cmd/memory-gateway`
- HTTP error model matching `docs/memory-gateway-api-contract.zh-CN.md`
- scope extraction from auth context
- request/response schemas for the first-batch endpoints
- consistency-level handling
- request ID propagation and response headers
- structured decision-trace emission

## First-Batch Endpoints

Planned M6 endpoint set:

- `POST /memory/recall/unified`
- `POST /memory/write`
- `PATCH /memory/items/{memory_id}`
- `POST /memory/items/{memory_id}/pin`
- `POST /memory/items/{memory_id}/disable`
- `DELETE /memory/items/{memory_id}`
- `POST /memory/sessions/{session_id}/close`

## Boundary Rules

1. Gateway must use service/auth-derived tenant scope, never client-claimed
   scope as authoritative input.
2. Gateway may translate HTTP requests into SDK/backend inputs, but it must not
   reimplement Postgres write semantics in handler code.
3. Gateway must not bypass SDK abstractions when a backend-neutral contract
   exists.
4. Gateway is the first place where env vars, config structs, and process-level
   lifecycle are allowed.

## Suggested Package Shape

When implementation begins, a likely shape is:

- `cmd/memory-gateway/`
- `internal/config/`
- `internal/httpapi/`
- `internal/authz/`
- `internal/transport/`
- `internal/service/`

No package layout is committed yet beyond the module boundary itself.

## Non-Goals For This Step

This design step does not:

- choose the final router implementation
- implement handlers
- define production deployment topology
- add background worker processes

## Acceptance Criteria

The module boundary is correct when:

- all service/process concerns are routed here rather than into the SDK or backend module
- backend selection can change without changing HTTP contract ownership
- auth and tenant binding remain gateway-owned concerns
